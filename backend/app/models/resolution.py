"""
ORM models for the confidence-gated face resolution pipeline (see
docs/DTID_ARCHITECTURE.md's Confidence-Gated Entity Resolution section
and app/services/entity_resolution_service.py).

Not reusing Video/PersonTrack/FaceDetection (video_processing.py): those
are hard-scoped to the video-processing job pipeline (NOT NULL video_id/
frame_number/timestamp_seconds - meaningless for a single uploaded
case-evidence photo) and carry no pointer back to the Memgraph ontology.
ResolutionEmbedding is the Postgres-side half of that two-tier UUID-
pointer wiring instead: one row per resolved-or-unresolved observation
embedding, referenced from the Memgraph Event by Event.embedding_id.

ReviewQueueItem is durable, filterable analyst workflow state - Postgres,
not Memgraph (a pending review item represents no real-world ontology
fact, matching the "tactical/operational layer only" framing
video_processing.py already uses for similar reasons) and not Redis
(genuinely unused elsewhere in this app; would trade away ORM/admin
visibility for no benefit).

IdentityFaceTemplate is a separate table from ResolutionEmbedding, not a
discriminator column on it, for the same reason ResolutionEmbedding was
kept separate from FaceDetection/PersonTrack: different semantics
shouldn't be forced into a shared shape. An enrollment template has no
event_id/case_id/quality fields (it isn't a case observation) and is
never "pending" in the review-queue sense - it's written once at
Identity-enrollment time (see entity_resolution_service.py's
enroll_identity) and its identity_id is backfilled once the Memgraph
Identity node exists to point back at it.
"""
import uuid

from pgvector.sqlalchemy import Vector
from sqlalchemy import JSON, Column, DateTime, Float, ForeignKey, Index, String, func
from sqlalchemy.dialects.postgresql import UUID

from app.core.database import Base

EMBEDDING_DIM = 512


class ResolutionEmbedding(Base):
    __tablename__ = "resolution_embeddings"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    event_id = Column(String, nullable=False, index=True)
    # Denormalized from the owning Case for "evidence for Case X" listing
    # without a Memgraph round-trip - nullable since not every resolved
    # observation originates from case-evidence upload.
    case_id = Column(String, nullable=True, index=True)
    entity_type = Column(String, nullable=False, default="TargetPerson")
    # NULL only transiently, between insert and the gate decision -
    # resolve_face_observation always resolves to SOME TargetEntity
    # (existing cluster, existing identity's cluster, or a freshly
    # auto-created unidentified cluster), nothing is discarded.
    target_entity_id = Column(String, nullable=True, index=True)
    embedding = Column(Vector(EMBEDDING_DIM), nullable=False)
    quality_score = Column(Float, nullable=True)
    detection_confidence = Column(Float, nullable=True)
    source_image_storage_key = Column(String, nullable=True)

    created_at = Column(DateTime(timezone=True), server_default=func.now())


class ReviewQueueItem(Base):
    __tablename__ = "review_queue_items"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    embedding_id = Column(
        UUID(as_uuid=True),
        ForeignKey("resolution_embeddings.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    event_id = Column(String, nullable=False, index=True)
    case_id = Column(String, nullable=True, index=True)
    # Exactly one of candidate_target_entity_id/candidate_identity_id is
    # set, matching candidate_kind - an identity-kind candidate may not
    # have a TargetEntity yet (this could be that identity's first
    # matched sighting), so this can no longer be a required column.
    candidate_kind = Column(String, nullable=False, default="cluster")  # "cluster" | "identity"
    candidate_target_entity_id = Column(String, nullable=True)
    candidate_identity_id = Column(String, nullable=True)
    similarity = Column(Float, nullable=False)
    # Full top-K candidates ([{"kind":..., "target_entity_id":..., "identity_id":..., "similarity":...}, ...]),
    # not just the top-1 above - lets the analyst pick a different candidate.
    top_candidates = Column(JSON, nullable=False, default=list)
    # pending | confirmed | confirmed_new | confirmed_identity | rejected
    status = Column(String, nullable=False, default="pending")
    reviewed_by = Column(String, nullable=True)
    reviewed_at = Column(DateTime(timezone=True), nullable=True)

    created_at = Column(DateTime(timezone=True), server_default=func.now())


class IdentityFaceTemplate(Base):
    __tablename__ = "identity_face_templates"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    # Nullable, backfilled after the Memgraph Identity node is created -
    # the Postgres row must exist first (to hand its id to the node as
    # face_template_id), but the node's id isn't known until after it's
    # created. A row with identity_id still NULL is mid-enrollment and
    # excluded from candidate search (see find_candidates).
    identity_id = Column(String, nullable=True, index=True)
    embedding = Column(Vector(EMBEDDING_DIM), nullable=False)
    source_image_storage_key = Column(String, nullable=True)

    created_at = Column(DateTime(timezone=True), server_default=func.now())


Index(
    "ix_resolution_embeddings_embedding_hnsw",
    ResolutionEmbedding.embedding,
    postgresql_using="hnsw",
    postgresql_ops={"embedding": "vector_cosine_ops"},
)
Index(
    "ix_identity_face_templates_embedding_hnsw",
    IdentityFaceTemplate.embedding,
    postgresql_using="hnsw",
    postgresql_ops={"embedding": "vector_cosine_ops"},
)
