"""
Framework-agnostic graph operations for Target/Case/Situation/Event/
TargetEntity/AnalystAction, matching the existing services/ convention
(face_detection_service.py etc.): takes a session in, returns plain
Pydantic models out, no HTTP concerns.
"""
import json
import uuid
from datetime import datetime
from typing import Literal

from neo4j import AsyncSession

from app.graph.schemas import (
    AnalystAction,
    Case,
    CaseDetail,
    EntityParticipation,
    Event,
    Identity,
    Situation,
    SituationDetail,
    Target,
    TargetDetail,
    TargetEntity,
    TargetEntityDetail,
)


def _target_from_record(record) -> Target:
    node = record["t"]
    return Target(
        id=node["id"],
        name=node["name"],
        description=node["description"],
        target_type=node["target_type"],
        created_at=datetime.fromisoformat(node["created_at"]),
    )


def _case_from_node(node) -> Case:
    return Case(
        id=node["id"],
        name=node["name"],
        description=node["description"],
        case_number=node.get("case_number"),
        status=node["status"],
        created_at=datetime.fromisoformat(node["created_at"]),
    )


def _situation_from_node(node) -> Situation:
    return Situation(
        id=node["id"],
        owner_id=node["owner_id"],
        owner_type=node["owner_type"],
        name=node["name"],
        description=node["description"],
        created_at=datetime.fromisoformat(node["created_at"]),
    )


def _event_from_node(node) -> Event:
    return Event(
        id=node["id"],
        description=node["description"],
        occurred_at=datetime.fromisoformat(node["occurred_at"]) if node.get("occurred_at") else None,
        created_at=datetime.fromisoformat(node["created_at"]),
        embedding_id=node.get("embedding_id"),
    )


def _analyst_action_from_node(node) -> AnalystAction:
    return AnalystAction(
        id=node["id"],
        action_type=node["action_type"],
        actor=node["actor"],
        confidence=node.get("confidence"),
        notes=node.get("notes"),
        created_at=datetime.fromisoformat(node["created_at"]),
    )


def _entity_from_node(node) -> TargetEntity:
    return TargetEntity(
        id=node["id"],
        entity_type=node["entity_type"],
        name=node["name"],
        properties=json.loads(node["properties_json"]) if node.get("properties_json") else {},
        created_at=datetime.fromisoformat(node["created_at"]),
    )


def _identity_from_node(node) -> Identity:
    return Identity(
        id=node["id"],
        full_name=node["full_name"],
        document_type=node.get("document_type"),
        document_number=node.get("document_number"),
        face_template_id=node.get("face_template_id"),
        fingerprint_template=node.get("fingerprint_template"),
        created_at=datetime.fromisoformat(node["created_at"]),
    )


async def create_target(
    session: AsyncSession, name: str, description: str, target_type: str
) -> Target:
    target = Target(
        id=str(uuid.uuid4()), name=name, description=description, target_type=target_type
    )
    await session.run(
        """
        CREATE (t:Target {
            id: $id, name: $name, description: $description,
            target_type: $target_type, created_at: $created_at
        })
        """,
        id=target.id,
        name=target.name,
        description=target.description,
        target_type=target.target_type,
        created_at=target.created_at.isoformat(),
    )
    return target


async def list_targets(session: AsyncSession) -> list[Target]:
    result = await session.run(
        "MATCH (t:Target) RETURN t ORDER BY t.created_at DESC"
    )
    return [_target_from_record(record) async for record in result]


async def get_target(session: AsyncSession, target_id: str) -> TargetDetail | None:
    result = await session.run(
        """
        MATCH (t:Target {id: $id})
        OPTIONAL MATCH (t)-[:HAS_SITUATION]->(s:Situation)
        RETURN t, collect(s) AS situations
        """,
        id=target_id,
    )
    record = await result.single()
    if record is None:
        return None

    target = _target_from_record(record)
    situations = [_situation_from_node(n) for n in record["situations"] if n is not None]
    return TargetDetail(**target.model_dump(), situations=situations)


async def create_case(
    session: AsyncSession, name: str, description: str, case_number: str | None = None
) -> Case:
    case = Case(id=str(uuid.uuid4()), name=name, description=description, case_number=case_number)
    await session.run(
        """
        CREATE (c:Case {
            id: $id, name: $name, description: $description,
            case_number: $case_number, status: $status, created_at: $created_at
        })
        """,
        id=case.id,
        name=case.name,
        description=case.description,
        case_number=case.case_number,
        status=case.status,
        created_at=case.created_at.isoformat(),
    )
    return case


async def list_cases(session: AsyncSession) -> list[Case]:
    result = await session.run("MATCH (c:Case) RETURN c ORDER BY c.created_at DESC")
    return [_case_from_node(record["c"]) async for record in result]


async def get_case(session: AsyncSession, case_id: str) -> CaseDetail | None:
    result = await session.run(
        """
        MATCH (c:Case {id: $id})
        OPTIONAL MATCH (c)-[:HAS_SITUATION]->(s:Situation)
        RETURN c, collect(s) AS situations
        """,
        id=case_id,
    )
    record = await result.single()
    if record is None:
        return None

    case = _case_from_node(record["c"])
    situations = [_situation_from_node(n) for n in record["situations"] if n is not None]
    return CaseDetail(**case.model_dump(), situations=situations)


async def link_case_to_target(session: AsyncSession, case_id: str, target_id: str) -> bool:
    """
    Link a Case to a Target via LINKED_TO. Many-to-many, no uniqueness
    guard: a Case may end up implicating several Targets, and a Target may
    accumulate several formal Cases over time - repeated calls with new
    ids just add more edges, never overwrite. Returns False if either
    node doesn't exist.
    """
    result = await session.run(
        """
        MATCH (c:Case {id: $case_id})
        MATCH (t:Target {id: $target_id})
        CREATE (c)-[:LINKED_TO]->(t)
        RETURN c
        """,
        case_id=case_id,
        target_id=target_id,
    )
    record = await result.single()
    return record is not None


async def create_situation(
    session: AsyncSession,
    owner_id: str,
    owner_type: Literal["Target", "Case"],
    name: str,
    description: str,
) -> Situation | None:
    if owner_type not in ("Target", "Case"):
        raise ValueError(f"owner_type must be 'Target' or 'Case', got {owner_type!r}")

    situation = Situation(
        id=str(uuid.uuid4()), owner_id=owner_id, owner_type=owner_type, name=name, description=description
    )
    # owner_type is interpolated as a Cypher label, not a query parameter -
    # labels can't be parameterized in Cypher. Safe here because it's
    # constrained to the "Target"/"Case" literal check above, never passed
    # through from raw request input.
    result = await session.run(
        f"""
        MATCH (o:{owner_type} {{id: $owner_id}})
        CREATE (s:Situation {{
            id: $id, owner_id: $owner_id, owner_type: $owner_type, name: $name,
            description: $description, created_at: $created_at
        }})
        CREATE (o)-[:HAS_SITUATION]->(s)
        RETURN s
        """,
        owner_id=owner_id,
        owner_type=owner_type,
        id=situation.id,
        name=situation.name,
        description=situation.description,
        created_at=situation.created_at.isoformat(),
    )
    record = await result.single()
    if record is None:
        # MATCH found no owner with that id - CREATE never ran.
        return None
    return situation


async def get_situation(session: AsyncSession, situation_id: str) -> SituationDetail | None:
    result = await session.run(
        """
        MATCH (s:Situation {id: $id})
        OPTIONAL MATCH (s)-[:HAS_EVENT]->(e:Event)
        OPTIONAL MATCH (ent:TargetEntity)-[p:PARTICIPATES_IN]->(s)
        RETURN s, collect(DISTINCT e) AS events,
               collect(DISTINCT {entity: ent, role: p.role, confidence: p.confidence}) AS participants
        """,
        id=situation_id,
    )
    record = await result.single()
    if record is None:
        return None

    situation = _situation_from_node(record["s"])
    events = [_event_from_node(n) for n in record["events"] if n is not None]
    participants = [
        EntityParticipation(entity=_entity_from_node(p["entity"]), role=p["role"], confidence=p["confidence"])
        for p in record["participants"]
        if p["entity"] is not None
    ]
    return SituationDetail(**situation.model_dump(), events=events, participants=participants)


async def create_event(
    session: AsyncSession,
    description: str,
    occurred_at: datetime | None = None,
    situation_id: str | None = None,
) -> Event | None:
    """
    Creates a standalone Event by default (situation_id=None) - a raw
    observation can arrive before anyone knows what Situation, if any, it
    belongs to. Pass situation_id only when the Situation is already
    known at creation time (creates the HAS_EVENT edge immediately);
    otherwise link it later with link_event_to_situation(). Returns None
    only if situation_id was given but doesn't match an existing
    Situation - a standalone creation (situation_id=None) always succeeds.
    """
    event = Event(id=str(uuid.uuid4()), description=description, occurred_at=occurred_at)
    params = dict(
        id=event.id,
        description=event.description,
        occurred_at=event.occurred_at.isoformat() if event.occurred_at else None,
        created_at=event.created_at.isoformat(),
    )

    if situation_id is None:
        await session.run(
            """
            CREATE (e:Event {
                id: $id, description: $description,
                occurred_at: $occurred_at, created_at: $created_at
            })
            """,
            **params,
        )
        return event

    result = await session.run(
        """
        MATCH (s:Situation {id: $situation_id})
        CREATE (e:Event {
            id: $id, description: $description,
            occurred_at: $occurred_at, created_at: $created_at
        })
        CREATE (s)-[:HAS_EVENT]->(e)
        RETURN e
        """,
        situation_id=situation_id,
        **params,
    )
    record = await result.single()
    return event if record is not None else None


async def list_events(session: AsyncSession, unlinked_only: bool = False) -> list[Event]:
    query = (
        "MATCH (e:Event) WHERE NOT EXISTS ((e)<-[:HAS_EVENT]-()) RETURN e ORDER BY e.created_at DESC"
        if unlinked_only
        else "MATCH (e:Event) RETURN e ORDER BY e.created_at DESC"
    )
    result = await session.run(query)
    return [_event_from_node(record["e"]) async for record in result]


async def get_event(session: AsyncSession, event_id: str) -> Event | None:
    result = await session.run("MATCH (e:Event {id: $id}) RETURN e", id=event_id)
    record = await result.single()
    return _event_from_node(record["e"]) if record else None


async def link_event_to_situation(
    session: AsyncSession, event_id: str, situation_id: str
) -> Event | None:
    """
    Attach an already-existing (possibly standalone) Event to a Situation
    - the "assemble a Situation from a collection of Events" workflow.
    Returns None if either node doesn't exist, or if the Event is already
    linked to a Situation (re-linking isn't supported here - would need an
    explicit unlink/correction step, not an implicit overwrite).
    """
    result = await session.run(
        """
        MATCH (e:Event {id: $event_id})
        WHERE NOT EXISTS ((e)<-[:HAS_EVENT]-())
        MATCH (s:Situation {id: $situation_id})
        CREATE (s)-[:HAS_EVENT]->(e)
        RETURN e
        """,
        event_id=event_id,
        situation_id=situation_id,
    )
    record = await result.single()
    return _event_from_node(record["e"]) if record is not None else None


async def set_event_embedding_pointer(
    session: AsyncSession, event_id: str, embedding_id: str
) -> Event | None:
    """
    Set Event.embedding_id to point at a Postgres resolution_embeddings
    row - the Memgraph-side half of the two-tier UUID-pointer wiring (the
    Postgres row itself stores event_id going the other way). Returns None
    if the Event doesn't exist.
    """
    result = await session.run(
        "MATCH (e:Event {id: $event_id}) SET e.embedding_id = $embedding_id RETURN e",
        event_id=event_id,
        embedding_id=embedding_id,
    )
    record = await result.single()
    return _event_from_node(record["e"]) if record is not None else None


async def create_entity(
    session: AsyncSession, entity_type: str, name: str, properties: dict[str, str]
) -> TargetEntity:
    entity = TargetEntity(
        id=str(uuid.uuid4()), entity_type=entity_type, name=name, properties=properties
    )
    await session.run(
        """
        CREATE (e:TargetEntity {
            id: $id, entity_type: $entity_type, name: $name,
            properties_json: $properties_json, created_at: $created_at
        })
        """,
        id=entity.id,
        entity_type=entity.entity_type,
        name=entity.name,
        properties_json=json.dumps(entity.properties),
        created_at=entity.created_at.isoformat(),
    )
    return entity


async def list_entities(session: AsyncSession, identified: bool | None = None) -> list[TargetEntity]:
    if identified is None:
        query = "MATCH (e:TargetEntity) RETURN e ORDER BY e.created_at DESC"
    elif identified:
        query = (
            "MATCH (e:TargetEntity) WHERE EXISTS ((e)<-[:IDENTIFIES]-(:Identity)) "
            "RETURN e ORDER BY e.created_at DESC"
        )
    else:
        query = (
            "MATCH (e:TargetEntity) WHERE NOT EXISTS ((e)<-[:IDENTIFIES]-(:Identity)) "
            "RETURN e ORDER BY e.created_at DESC"
        )
    result = await session.run(query)
    return [_entity_from_node(record["e"]) async for record in result]


async def get_entity(session: AsyncSession, entity_id: str) -> TargetEntity | None:
    result = await session.run("MATCH (e:TargetEntity {id: $id}) RETURN e", id=entity_id)
    record = await result.single()
    return _entity_from_node(record["e"]) if record else None


async def get_entity_detail(session: AsyncSession, entity_id: str) -> TargetEntityDetail | None:
    result = await session.run(
        """
        MATCH (e:TargetEntity {id: $id})
        OPTIONAL MATCH (i:Identity)-[:IDENTIFIES]->(e)
        RETURN e, collect(i) AS identities
        """,
        id=entity_id,
    )
    record = await result.single()
    if record is None:
        return None

    entity = _entity_from_node(record["e"])
    identities = [_identity_from_node(n) for n in record["identities"] if n is not None]
    return TargetEntityDetail(**entity.model_dump(), identities=identities)


async def set_entity_properties(
    session: AsyncSession, entity_id: str, properties: dict[str, str]
) -> TargetEntity | None:
    """
    Merge `properties` into the entity's existing properties dict
    (overwrite on key collision). Plain, unopinionated primitive - callers
    (e.g. the resolver setting "embedding_uuid" only if absent) decide
    their own merge semantics before calling this. Returns None if the
    entity doesn't exist.
    """
    entity = await get_entity(session, entity_id)
    if entity is None:
        return None

    merged = {**entity.properties, **properties}
    await session.run(
        "MATCH (e:TargetEntity {id: $id}) SET e.properties_json = $properties_json",
        id=entity_id,
        properties_json=json.dumps(merged),
    )
    entity.properties = merged
    return entity


async def create_identity(
    session: AsyncSession,
    *,
    full_name: str,
    document_type: str | None = None,
    document_number: str | None = None,
    face_template_id: str | None = None,
    fingerprint_template: str | None = None,
) -> Identity:
    """
    Identity(...) below raises (via its model_validator) before any write
    happens if both face_template_id and fingerprint_template are None -
    callers at the API layer should catch and translate to 422.
    """
    identity = Identity(
        id=str(uuid.uuid4()),
        full_name=full_name,
        document_type=document_type,
        document_number=document_number,
        face_template_id=face_template_id,
        fingerprint_template=fingerprint_template,
    )
    await session.run(
        """
        CREATE (i:Identity {
            id: $id, full_name: $full_name, document_type: $document_type,
            document_number: $document_number, face_template_id: $face_template_id,
            fingerprint_template: $fingerprint_template, created_at: $created_at
        })
        """,
        id=identity.id,
        full_name=identity.full_name,
        document_type=identity.document_type,
        document_number=identity.document_number,
        face_template_id=identity.face_template_id,
        fingerprint_template=identity.fingerprint_template,
        created_at=identity.created_at.isoformat(),
    )
    return identity


async def get_identity(session: AsyncSession, identity_id: str) -> Identity | None:
    result = await session.run("MATCH (i:Identity {id: $id}) RETURN i", id=identity_id)
    record = await result.single()
    return _identity_from_node(record["i"]) if record else None


async def attach_identity_to_entity(session: AsyncSession, identity_id: str, target_entity_id: str) -> bool:
    """
    Create IDENTIFIES. Many-to-one is intentional and unrestricted - more
    than one Identity pointing at the same TargetEntity isn't an error
    state, it's a fraud signal (the same biometric person presenting
    under multiple documents with different names) - see
    app/graph/schemas.py's module docstring. No uniqueness guard, mirrors
    link_case_to_target. Restricted to TargetPerson entities only:
    Identity is unambiguously a person-scoped biometric record, so
    attaching one to e.g. a Vehicle TargetEntity is a real error, not a
    permissive edge case. Returns False if either node doesn't exist, or
    if target_entity_id isn't entity_type "TargetPerson".
    """
    result = await session.run(
        """
        MATCH (i:Identity {id: $identity_id})
        MATCH (e:TargetEntity {id: $target_entity_id, entity_type: "TargetPerson"})
        CREATE (i)-[:IDENTIFIES]->(e)
        RETURN i
        """,
        identity_id=identity_id,
        target_entity_id=target_entity_id,
    )
    record = await result.single()
    return record is not None


async def get_entity_for_identity(session: AsyncSession, identity_id: str) -> TargetEntity | None:
    """
    The TargetEntity this Identity already IDENTIFIES, if any - used by
    the resolution pipeline's reuse-or-create logic (see
    entity_resolution_service.py's _resolve_or_create_entity_for_identity).
    An Identity is expected to carry at most one such edge in practice;
    if more than one exists, the earliest-created one wins, deterministically.
    """
    result = await session.run(
        """
        MATCH (i:Identity {id: $identity_id})-[:IDENTIFIES]->(e:TargetEntity)
        RETURN e ORDER BY e.created_at ASC LIMIT 1
        """,
        identity_id=identity_id,
    )
    record = await result.single()
    return _entity_from_node(record["e"]) if record is not None else None


async def add_participation(
    session: AsyncSession,
    entity_id: str,
    target_id: str,
    role: str | None,
    confidence: float | None = None,
) -> bool:
    """
    Link a TargetEntity to a Situation or an Event (target_id may be
    either - both node types carry a unique `id`) via PARTICIPATES_IN,
    with an optional role (e.g. "victim", "agent", "witness") and an
    optional confidence (set by automated resolution - see
    entity_resolution_service.py; None for manually-created
    participations). Returns False if either node doesn't exist.
    """
    result = await session.run(
        """
        MATCH (ent:TargetEntity {id: $entity_id})
        MATCH (target {id: $target_id})
        WHERE target:Situation OR target:Event
        CREATE (ent)-[:PARTICIPATES_IN {role: $role, confidence: $confidence}]->(target)
        RETURN ent
        """,
        entity_id=entity_id,
        target_id=target_id,
        role=role,
        confidence=confidence,
    )
    record = await result.single()
    return record is not None


async def log_analyst_action(
    session: AsyncSession,
    action_type: str,
    actor: str,
    affects_ids: list[str],
    confidence: float | None = None,
    notes: str | None = None,
) -> AnalystAction:
    """
    Record an audit-log entry and link it to every node it concerns via
    AFFECTS (an Event, a TargetEntity, ...) - see
    docs/DTID_ARCHITECTURE.md's Confidence-Gated Entity Resolution
    section, which names this as the platform's audit trail for every
    gate decision (auto_match, queued_for_review, new_cluster,
    analyst_confirm, analyst_confirm_new, analyst_confirm_identity,
    analyst_reject).
    """
    action = AnalystAction(
        id=str(uuid.uuid4()), action_type=action_type, actor=actor, confidence=confidence, notes=notes
    )
    await session.run(
        """
        CREATE (a:AnalystAction {
            id: $id, action_type: $action_type, actor: $actor,
            confidence: $confidence, notes: $notes, created_at: $created_at
        })
        WITH a
        UNWIND $affects_ids AS affected_id
        MATCH (n {id: affected_id})
        CREATE (a)-[:AFFECTS]->(n)
        """,
        id=action.id,
        action_type=action.action_type,
        actor=action.actor,
        confidence=action.confidence,
        notes=action.notes,
        created_at=action.created_at.isoformat(),
        affects_ids=affects_ids,
    )
    return action


async def list_analyst_actions(session: AsyncSession, node_id: str) -> list[AnalystAction]:
    result = await session.run(
        """
        MATCH (a:AnalystAction)-[:AFFECTS]->(n {id: $node_id})
        RETURN a ORDER BY a.created_at DESC
        """,
        node_id=node_id,
    )
    return [_analyst_action_from_node(record["a"]) async for record in result]
