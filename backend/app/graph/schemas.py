"""
Instance-layer ontology models: Target, Situation, Event, TargetEntity,
per docs/DTID_ARCHITECTURE.md's "Core Ontology" section.

Situation is deliberately narrative-first: its `description` is meant to
be a discursive account of the incident (an analyst's write-up), not a
short label - the structured, analyzable data comes from the Events
linked to it (HAS_EVENT) and the Entities participating in it
(PARTICIPATES_IN). A future step could use GraphRAG to suggest Events for
a Situation from that narrative text; nothing here builds toward that
yet, this is just why the ontology keeps the two separate rather than
folding Events into Situation-level fields.

Event carries no situation_id at all, in the model or the graph: an Event
can exist before anyone knows what Situation (if any) it belongs to - a
raw observation arrives, and a Situation may only get assembled later
from a collection of already-existing Events, not the other way around.
HAS_EVENT is the single source of truth for the relationship (created at
Event creation time if the Situation is already known, or afterward via
link_event_to_situation). Callers who need "what Situation is this Event
part of" query that edge directly (see get_situation, which returns its
Events) rather than reading a field that could duplicate/disagree with
the graph - there is deliberately no second place this fact lives.

PARTICIPATES_IN carries a `role` (e.g. "victim", "agent", "witness") and
can attach a TargetEntity to either a Situation directly (a situation-level
fact - "this person is a victim of this incident") or to a specific Event
(finer-grained - "this person appears in this observation"), matching the
architecture doc's "Target Entity→Event/Situation" edge definition. role
is free text, not a rigid enum, for the same "lightweight, unenforced"
reason property_hints on EntityTypeDefinition are unenforced.

Note: Target.target_type is a plain string matching a TargetTypeTemplate's
name (app/ontology/templates), not a graph edge to an actual template
node - the architecture doc's original design has TargetTypeTemplate
living in Memgraph too, but it was built as an in-memory Python store
before Memgraph existed in this codebase (see app/ontology/templates/
store.py's docstring). Migrating it into the graph is a follow-up, not
done here. TargetEntity.entity_type is the same situation relative to
EntityTypeDefinition.

Case is a second, independent owner a Situation can have alongside
Target: a Target is an ongoing subject of intelligence interest, a Case
is a formal forensic/legal procedure (a police case number) that does
NOT require a Target to exist first - real casework starts from evidence
with zero identified subjects, not from a pre-declared subject. A Case
may later be linked to one or more Targets via LINKED_TO (and a Target
may accumulate several Cases over time) through an explicit action, never
at Case-creation time. Because of this second owner, Situation.owner_id/
owner_type replace what used to be a single Situation.target_id - unlike
Event's situation_id (removed entirely, edge-derived only, because that
link is deferred/optional), a Situation's owner is required at creation
and set once, immutably, so keeping it as a stored field (rather than
deriving it from HAS_SITUATION at read time) avoids an extra traversal on
every read without reintroducing the kind of redundancy that removal was
about - there is still exactly one place this fact is set.

Identity is a document-backed biometric record (a name plus a reference
face template and/or fingerprint template), deliberately separate from
TargetEntity: a TargetEntity/"cluster" is a biometric grouping that forms
automatically the moment a face is seen (see
entity_resolution_service.py's resolve_face_observation - every observed
face always lands on some TargetEntity, identified or not), while an
Identity is the formal paperwork that may or may not ever get attached to
one. IDENTIFIES (Identity -> TargetEntity) is deliberately unconstrained
in both directions - in particular, more than one Identity pointing at
the same TargetEntity is not an error state to prevent, it's a fraud
signal worth surfacing (the same biometric person presenting under
multiple documents with different names).
"""
from datetime import datetime, timezone
from typing import Literal

from pydantic import BaseModel, Field, model_validator


def _utcnow() -> datetime:
    return datetime.now(timezone.utc)


class Target(BaseModel):
    id: str
    name: str
    description: str
    target_type: str
    created_at: datetime = Field(default_factory=_utcnow)


class Case(BaseModel):
    id: str
    name: str
    description: str
    case_number: str | None = None
    status: str = "open"
    created_at: datetime = Field(default_factory=_utcnow)


class Situation(BaseModel):
    id: str
    owner_id: str
    owner_type: Literal["Target", "Case"]
    name: str
    description: str
    created_at: datetime = Field(default_factory=_utcnow)


class Event(BaseModel):
    id: str
    description: str
    occurred_at: datetime | None = None
    created_at: datetime = Field(default_factory=_utcnow)
    # Pointer to Postgres resolution_embeddings.id - the artifact (a 512-d
    # face embedding) lives in Postgres, this is a UUID reference only, per
    # the two-tier storage split (Postgres = artifacts, Memgraph =
    # relationships). Set via set_event_embedding_pointer once a face
    # observation has been auto-committed, queued, or resolved into a new
    # cluster for this Event - every outcome sets it, nothing is discarded.
    embedding_id: str | None = None


class TargetEntity(BaseModel):
    id: str
    entity_type: str
    name: str
    properties: dict[str, str] = {}
    created_at: datetime = Field(default_factory=_utcnow)


class Identity(BaseModel):
    id: str
    full_name: str
    document_type: str | None = None
    document_number: str | None = None
    # Pointer to Postgres identity_face_templates.id - same two-tier
    # split as Event.embedding_id.
    face_template_id: str | None = None
    # Opaque placeholder - no matching/search logic implemented for this,
    # data-model support only, for future fingerprint-based resolution.
    fingerprint_template: str | None = None
    created_at: datetime = Field(default_factory=_utcnow)

    @model_validator(mode="after")
    def _require_a_biometric(self) -> "Identity":
        if self.face_template_id is None and self.fingerprint_template is None:
            raise ValueError(
                "Identity requires at least one of face_template_id or fingerprint_template"
            )
        return self


class EntityParticipation(BaseModel):
    entity: TargetEntity
    role: str | None = None
    confidence: float | None = None


class AnalystAction(BaseModel):
    id: str
    action_type: str
    actor: str
    confidence: float | None = None
    notes: str | None = None
    created_at: datetime = Field(default_factory=_utcnow)


class TargetDetail(Target):
    situations: list[Situation] = []


class CaseDetail(Case):
    situations: list[Situation] = []


class TargetEntityDetail(TargetEntity):
    identities: list[Identity] = []


class SituationDetail(Situation):
    events: list[Event] = []
    participants: list[EntityParticipation] = []
