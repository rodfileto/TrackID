"""
Confidence-gated face entity resolution - the platform's central control
loop per docs/DTID_ARCHITECTURE.md: a new face observation (an Event with
an embedding) gets matched against everyone already known to the system,
globally across Cases, and gated into one of three outcomes:

    similarity >= tau_high            -> auto-commit (link straight into
                                          the graph, no analyst step)
    tau_low <= similarity < tau_high  -> enqueue for analyst review
    similarity < tau_low (or no hit)  -> auto-create a brand-new,
                                          unidentified cluster (TargetEntity)
                                          and resolve into it

Every observation always lands on SOME TargetEntity cluster now - nothing
is discarded (this used to have a fourth "discarded" outcome that lost
the observation; that was the common case, not an edge case, since a
person's first-ever sighting always has zero candidates to match
against). "Whatever the app targets a face, it creates a cluster of
faces that means it is the same person" - clusters form bottom-up from
biometrics regardless of whether anyone is named yet.

Matching candidates come from two pools, searched together: existing
resolved clusters (ResolutionEmbedding rows), and enrolled Identity
face-templates (IdentityFaceTemplate rows, see app/graph/schemas.py's
Identity model) - so a person's first-ever observed sighting can
auto-resolve straight to a pre-enrolled, document-backed identity instead
of starting as an anonymous cluster.

This is the one piece of logic that legitimately needs both a Postgres
session (embeddings, review queue - see app/models/resolution.py) and a
Memgraph session (the ontology - see app/graph/service.py) at once, which
is why it doesn't live in either graph/service.py or
face_detection_service.py.

Cross-store atomicity is NOT solved here: writes go Postgres insert+commit
first (cheap, safely re-checkable by event_id), then Memgraph writes, then
a Postgres outcome update+commit. If the Memgraph step fails after the
Postgres insert commits, the embedding row is left permanently
"unresolved" until someone re-runs resolution for that event_id - no
automatic reconciliation job exists. Accepted as a known v1 gap rather
than building a saga/outbox pattern for a departmental-scale system.
"""
import uuid
from datetime import datetime, timezone
from typing import Literal

from neo4j import AsyncSession as GraphSession
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession as PgSession

from app.core.config import settings
from app.graph.service import (
    add_participation,
    attach_identity_to_entity,
    create_entity,
    create_identity,
    get_entity,
    get_entity_for_identity,
    get_identity,
    log_analyst_action,
    set_entity_properties,
    set_event_embedding_pointer,
)
from app.graph.schemas import Identity
from app.models.resolution import IdentityFaceTemplate, ResolutionEmbedding, ReviewQueueItem


class CandidateMatch(BaseModel):
    kind: Literal["cluster", "identity"]
    target_entity_id: str | None = None
    identity_id: str | None = None
    similarity: float
    embedding_id: str  # ResolutionEmbedding.id (cluster) or IdentityFaceTemplate.id (identity)


class ResolutionOutcome(BaseModel):
    outcome: Literal["auto_committed", "queued", "new_cluster"]
    target_entity_id: str | None = None
    review_item_id: uuid.UUID | None = None
    candidates: list[CandidateMatch] = []


async def _find_cluster_candidates(
    pg_session: PgSession, embedding: list[float], entity_type: str, pool_size: int
) -> list[CandidateMatch]:
    """
    ANN search (pgvector HNSW, cosine distance via <=>) over already-
    resolved rows (target_entity_id IS NOT NULL), scoped globally across
    all Cases - that cross-case-silo reach is the entire point (an
    unidentified face in Case A should be matchable against an identity
    resolved in Case B). Fetches `pool_size` nearest raw rows and de-dupes
    to the single best-matching observation per TargetEntity in Python
    (an entity may have many resolved observations; each row's distance
    stands alone). Plain fetch-then-group over a DISTINCT ON/window-function
    query, matching this codebase's existing preference for simple
    service-layer logic over clever SQL (see compare_embeddings() in
    face_detection_service.py).
    """
    distance = ResolutionEmbedding.embedding.cosine_distance(embedding)
    stmt = (
        select(ResolutionEmbedding.target_entity_id, ResolutionEmbedding.id, distance.label("distance"))
        .where(ResolutionEmbedding.entity_type == entity_type)
        .where(ResolutionEmbedding.target_entity_id.isnot(None))
        .order_by(distance)
        .limit(pool_size)
    )
    result = await pg_session.execute(stmt)

    best_per_entity: dict[str, CandidateMatch] = {}
    for target_entity_id, embedding_id, distance_value in result.all():
        similarity = 1.0 - float(distance_value)
        existing = best_per_entity.get(target_entity_id)
        if existing is None or similarity > existing.similarity:
            best_per_entity[target_entity_id] = CandidateMatch(
                kind="cluster",
                target_entity_id=target_entity_id,
                similarity=similarity,
                embedding_id=str(embedding_id),
            )
    return list(best_per_entity.values())


async def _find_identity_candidates(
    pg_session: PgSession, embedding: list[float], pool_size: int
) -> list[CandidateMatch]:
    """
    Same ANN-then-group-in-Python shape as _find_cluster_candidates, over
    IdentityFaceTemplate instead - best-per-identity_id. identity_id IS
    NOT NULL guards against a row still mid-enrollment (see
    enroll_identity's ordering). No entity_type filter here: a face
    template is inherently person-scoped by construction, the same way
    Identity itself carries no entity_type field.
    """
    distance = IdentityFaceTemplate.embedding.cosine_distance(embedding)
    stmt = (
        select(IdentityFaceTemplate.identity_id, IdentityFaceTemplate.id, distance.label("distance"))
        .where(IdentityFaceTemplate.identity_id.isnot(None))
        .order_by(distance)
        .limit(pool_size)
    )
    result = await pg_session.execute(stmt)

    best_per_identity: dict[str, CandidateMatch] = {}
    for identity_id, embedding_id, distance_value in result.all():
        similarity = 1.0 - float(distance_value)
        existing = best_per_identity.get(identity_id)
        if existing is None or similarity > existing.similarity:
            best_per_identity[identity_id] = CandidateMatch(
                kind="identity",
                identity_id=identity_id,
                similarity=similarity,
                embedding_id=str(embedding_id),
            )
    return list(best_per_identity.values())


async def find_candidates(
    pg_session: PgSession,
    embedding: list[float],
    entity_type: str = "TargetPerson",
    pool_size: int = 200,
    top_k: int = 5,
) -> list[CandidateMatch]:
    """
    Merges both candidate pools (existing clusters + enrolled identity
    templates) and re-ranks together by similarity. Accepted rough edge:
    if a cluster already has both resolved observations and an attached
    identity's own template, both pools can surface a candidate for the
    same underlying entity in the merged top-K - not incorrect (either
    resolves to the same entity via the reuse-or-create path below), just
    occasionally redundant in the list.
    """
    cluster_candidates = await _find_cluster_candidates(pg_session, embedding, entity_type, pool_size)
    identity_candidates = await _find_identity_candidates(pg_session, embedding, pool_size)
    merged = sorted(cluster_candidates + identity_candidates, key=lambda c: c.similarity, reverse=True)
    return merged[:top_k]


async def _resolve_or_create_entity_for_identity(graph_session: GraphSession, identity_id: str) -> tuple[str, bool]:
    """
    Returns (target_entity_id, created). If identity_id already IDENTIFIES
    a TargetEntity, reuse it. Otherwise this is the identity's first
    matched sighting: create a new TargetEntity named after
    identity.full_name and IDENTIFIES-link it. Shared by the auto-commit
    identity path and confirm_review_item_as_identity so the "does this
    identity have a cluster yet" logic isn't duplicated.
    """
    existing = await get_entity_for_identity(graph_session, identity_id)
    if existing is not None:
        return existing.id, False

    identity = await get_identity(graph_session, identity_id)
    if identity is None:
        raise ValueError(f"Identity {identity_id!r} not found")

    entity = await create_entity(graph_session, "TargetPerson", identity.full_name, {})
    await attach_identity_to_entity(graph_session, identity_id, entity.id)
    return entity.id, True


async def _maybe_set_representative_embedding(
    graph_session: GraphSession, target_entity_id: str, embedding_id: uuid.UUID
) -> None:
    """
    Sets TargetEntity.properties["embedding_uuid"] the first time an
    entity gets resolved, implementing the pre-existing
    TARGET_PERSON.property_hints entry (app/ontology/entity_types/
    builtin.py). Display pointer only - never used for matching, which
    always searches the full resolved pool via find_candidates.
    """
    entity = await get_entity(graph_session, target_entity_id)
    if entity is not None and "embedding_uuid" not in entity.properties:
        await set_entity_properties(graph_session, target_entity_id, {"embedding_uuid": str(embedding_id)})


async def resolve_face_observation(
    pg_session: PgSession,
    graph_session: GraphSession,
    *,
    event_id: str,
    embedding: list[float],
    case_id: str | None = None,
    quality_score: float | None = None,
    detection_confidence: float | None = None,
    source_image_storage_key: str | None = None,
    actor: str = "system",
    tau_high: float | None = None,
    tau_low: float | None = None,
) -> ResolutionOutcome:
    tau_high = settings.RESOLUTION_TAU_HIGH if tau_high is None else tau_high
    tau_low = settings.RESOLUTION_TAU_LOW if tau_low is None else tau_low

    row = ResolutionEmbedding(
        event_id=event_id,
        case_id=case_id,
        entity_type="TargetPerson",
        embedding=embedding,
        quality_score=quality_score,
        detection_confidence=detection_confidence,
        source_image_storage_key=source_image_storage_key,
    )
    pg_session.add(row)
    await pg_session.commit()
    await pg_session.refresh(row)

    await set_event_embedding_pointer(graph_session, event_id, str(row.id))

    candidates = await find_candidates(
        pg_session,
        embedding,
        pool_size=settings.RESOLUTION_CANDIDATE_POOL_SIZE,
        top_k=settings.RESOLUTION_TOP_K,
    )
    top = candidates[0] if candidates else None

    if top is not None and top.similarity >= tau_high:
        if top.kind == "identity":
            target_entity_id, _created = await _resolve_or_create_entity_for_identity(graph_session, top.identity_id)
        else:
            target_entity_id = top.target_entity_id

        await add_participation(
            graph_session, target_entity_id, event_id, role="auto-resolved", confidence=top.similarity
        )
        row.target_entity_id = target_entity_id
        await pg_session.commit()
        await _maybe_set_representative_embedding(graph_session, target_entity_id, row.id)
        await log_analyst_action(
            graph_session, "auto_match", actor, [event_id, target_entity_id], confidence=top.similarity
        )
        return ResolutionOutcome(outcome="auto_committed", target_entity_id=target_entity_id, candidates=candidates)

    if top is not None and top.similarity >= tau_low:
        item = ReviewQueueItem(
            embedding_id=row.id,
            event_id=event_id,
            case_id=case_id,
            candidate_kind=top.kind,
            candidate_target_entity_id=top.target_entity_id,
            candidate_identity_id=top.identity_id,
            similarity=top.similarity,
            top_candidates=[c.model_dump() for c in candidates],
        )
        pg_session.add(item)
        await pg_session.commit()
        await pg_session.refresh(item)
        await log_analyst_action(graph_session, "queued_for_review", actor, [event_id], confidence=top.similarity)
        return ResolutionOutcome(outcome="queued", review_item_id=item.id, candidates=candidates)

    # No candidate strong enough (or none at all) - every observation
    # always resolves to SOME TargetEntity now: a brand-new, unidentified
    # cluster, not a discard.
    entity = await create_entity(graph_session, "TargetPerson", f"Unidentified Person ({event_id[:8]})", {})
    await add_participation(
        graph_session, entity.id, event_id, role="auto-new-cluster", confidence=top.similarity if top else None
    )
    row.target_entity_id = entity.id
    await pg_session.commit()
    await _maybe_set_representative_embedding(graph_session, entity.id, row.id)
    await log_analyst_action(
        graph_session, "new_cluster", actor, [event_id, entity.id], confidence=top.similarity if top else None
    )
    return ResolutionOutcome(outcome="new_cluster", target_entity_id=entity.id, candidates=candidates)


async def confirm_review_item(
    pg_session: PgSession, graph_session: GraphSession, *, item_id: uuid.UUID, target_entity_id: str, actor: str
) -> ReviewQueueItem | None:
    """Analyst accepts a specific candidate (top-1 or another from top_candidates)."""
    item = await pg_session.get(ReviewQueueItem, item_id)
    if item is None or item.status != "pending":
        return None

    ok = await add_participation(
        graph_session, target_entity_id, item.event_id, role="analyst-confirmed", confidence=item.similarity
    )
    if not ok:
        return None

    embedding_row = await pg_session.get(ResolutionEmbedding, item.embedding_id)
    embedding_row.target_entity_id = target_entity_id
    item.status = "confirmed"
    item.reviewed_by = actor
    item.reviewed_at = datetime.now(timezone.utc)
    await pg_session.commit()
    await pg_session.refresh(item)

    await _maybe_set_representative_embedding(graph_session, target_entity_id, item.embedding_id)
    await log_analyst_action(
        graph_session, "analyst_confirm", actor, [item.event_id, target_entity_id], confidence=item.similarity
    )
    return item


async def confirm_review_item_as_new(
    pg_session: PgSession, graph_session: GraphSession, *, item_id: uuid.UUID, entity_name: str, actor: str
) -> ReviewQueueItem | None:
    """None of the candidates match - resolve this observation to a newly-created identity instead."""
    item = await pg_session.get(ReviewQueueItem, item_id)
    if item is None or item.status != "pending":
        return None

    entity = await create_entity(graph_session, "TargetPerson", entity_name, {})
    await add_participation(
        graph_session, entity.id, item.event_id, role="analyst-confirmed-new", confidence=item.similarity
    )

    embedding_row = await pg_session.get(ResolutionEmbedding, item.embedding_id)
    embedding_row.target_entity_id = entity.id
    item.status = "confirmed_new"
    item.reviewed_by = actor
    item.reviewed_at = datetime.now(timezone.utc)
    await pg_session.commit()
    await pg_session.refresh(item)

    await set_entity_properties(graph_session, entity.id, {"embedding_uuid": str(item.embedding_id)})
    await log_analyst_action(
        graph_session, "analyst_confirm_new", actor, [item.event_id, entity.id], confidence=item.similarity
    )
    return item


async def confirm_review_item_as_identity(
    pg_session: PgSession, graph_session: GraphSession, *, item_id: uuid.UUID, identity_id: str, actor: str
) -> ReviewQueueItem | None:
    """
    Analyst confirms this observation matches a specific pre-enrolled
    Identity (not necessarily the queued item's own top candidate - any
    known identity may be picked). Resolves via the same reuse-or-create
    logic the auto-commit identity path uses. Uses its own status value
    ("confirmed_identity", not "confirmed") so the four review outcomes
    stay independently queryable - confirming against an existing cluster
    and confirming against a pre-enrolled identity are different analyst
    decisions worth telling apart later.
    """
    item = await pg_session.get(ReviewQueueItem, item_id)
    if item is None or item.status != "pending":
        return None

    try:
        target_entity_id, _created = await _resolve_or_create_entity_for_identity(graph_session, identity_id)
    except ValueError:
        return None

    ok = await add_participation(
        graph_session, target_entity_id, item.event_id, role="analyst-confirmed-identity", confidence=item.similarity
    )
    if not ok:
        return None

    embedding_row = await pg_session.get(ResolutionEmbedding, item.embedding_id)
    embedding_row.target_entity_id = target_entity_id
    item.status = "confirmed_identity"
    item.reviewed_by = actor
    item.reviewed_at = datetime.now(timezone.utc)
    await pg_session.commit()
    await pg_session.refresh(item)

    await _maybe_set_representative_embedding(graph_session, target_entity_id, item.embedding_id)
    await log_analyst_action(
        graph_session,
        "analyst_confirm_identity",
        actor,
        [item.event_id, target_entity_id, identity_id],
        confidence=item.similarity,
    )
    return item


async def reject_review_item(
    pg_session: PgSession, graph_session: GraphSession, *, item_id: uuid.UUID, actor: str, notes: str | None = None
) -> ReviewQueueItem | None:
    """No graph edge created; the embedding stays unresolved/searchable for future matches."""
    item = await pg_session.get(ReviewQueueItem, item_id)
    if item is None or item.status != "pending":
        return None

    item.status = "rejected"
    item.reviewed_by = actor
    item.reviewed_at = datetime.now(timezone.utc)
    await pg_session.commit()
    await pg_session.refresh(item)

    await log_analyst_action(graph_session, "analyst_reject", actor, [item.event_id], notes=notes)
    return item


async def enroll_identity(
    pg_session: PgSession,
    graph_session: GraphSession,
    *,
    full_name: str,
    document_type: str | None,
    document_number: str | None,
    fingerprint_template: str | None,
    face_embedding: list[float] | None,
    source_image_storage_key: str | None = None,
) -> Identity:
    """
    Cross-store orchestration for POST /identities. No image processing
    here - the caller (API layer) already ran FaceDetectionService and
    passes the extracted embedding, mirroring how resolve_face_observation
    itself stays free of HTTP/upload concerns.

    Ordering is the reverse of the Event<->ResolutionEmbedding pattern:
    here the Postgres row must exist before the Memgraph node can
    reference its id (as face_template_id), but the node's id isn't known
    until after it's created - so the row's identity_id is backfilled as
    a second step, not set at insert time.

    Raises ValueError if both face_embedding and fingerprint_template are
    None - caller should translate to 422 (this mirrors, but pre-empts,
    the same check the Identity model's own model_validator would raise
    on construction, giving the API layer a chance to respond before any
    Postgres row is written).
    """
    if face_embedding is None and fingerprint_template is None:
        raise ValueError("Identity requires at least one of a reference photo or a fingerprint_template")

    template_row = None
    face_template_id = None
    if face_embedding is not None:
        template_row = IdentityFaceTemplate(embedding=face_embedding, source_image_storage_key=source_image_storage_key)
        pg_session.add(template_row)
        await pg_session.commit()
        await pg_session.refresh(template_row)
        face_template_id = str(template_row.id)

    identity = await create_identity(
        graph_session,
        full_name=full_name,
        document_type=document_type,
        document_number=document_number,
        face_template_id=face_template_id,
        fingerprint_template=fingerprint_template,
    )

    if template_row is not None:
        template_row.identity_id = identity.id
        await pg_session.commit()

    return identity
