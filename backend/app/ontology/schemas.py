"""
Schema-layer ontology models: registrable types for Target Systems
(TargetTypeTemplate) and their internal concept networks (Concept), and
registrable types for Target Entities (EntityTypeDefinition), per
docs/DTID_ARCHITECTURE.md's "Core Ontology" section.

Framework-agnostic like the other services in this codebase - these are
plain Pydantic models, not tied to Memgraph yet. A template built here is
storage-ready: it serializes directly into the CREATE statements described
in the architecture doc (TargetTypeTemplate -[:HAS_CONCEPT]-> Concept,
Concept -[:RELATES_TO]-> Concept) once the graph driver exists.
"""
from pydantic import BaseModel, model_validator


class EntityTypeDefinition(BaseModel):
    """
    A registrable type for Target Entities - what makes TargetPerson,
    Vehicle, and PhoneNumber a starter template rather than a hardcoded,
    closed list. property_hints are typical/suggested properties, not
    enforced - matching how property graphs naturally work rather than
    adding a rigid validation layer. default_tau_high/default_tau_low are
    the per-type confidence-gate defaults referenced in the architecture
    doc's "Confidence-Gated Entity Resolution" section - what counts as a
    confident match differs by resolution modality (face similarity vs.
    an exact-match identifier like a phone number), so the default lives
    on the type, not as a single global constant.
    """

    name: str
    description: str
    property_hints: list[str] = []
    default_tau_high: float
    default_tau_low: float
    source: str = "builtin"  # "builtin" | "user-defined"
    created_by: str | None = None

    @model_validator(mode="after")
    def _thresholds_ordered_and_in_range(self) -> "EntityTypeDefinition":
        if not (0.0 <= self.default_tau_low <= self.default_tau_high <= 1.0):
            raise ValueError(
                "expected 0 <= default_tau_low <= default_tau_high <= 1, got "
                f"tau_low={self.default_tau_low}, tau_high={self.default_tau_high}"
            )
        return self


class ConceptDefinition(BaseModel):
    """One thematic/operational category within a TargetTypeTemplate."""

    name: str
    description: str


class ConceptRelation(BaseModel):
    """
    A RELATES_TO edge between two concepts in the same template, referenced
    by ConceptDefinition.name. Kept as a single generic relationship type
    for now, per the architecture doc - no PRECEDES/ENABLES taxonomy yet.
    """

    source: str
    target: str


class TargetTypeTemplate(BaseModel):
    """
    A registrable type for Target Systems (e.g. "Bank Robbery"), carrying
    its own network of interrelated Concepts. A Target instance references
    exactly one TargetTypeTemplate via an OF_TYPE edge - no composition or
    inheritance between templates yet.
    """

    name: str
    description: str
    source: str = "builtin"  # "builtin" | "user-defined"
    created_by: str | None = None
    concepts: list[ConceptDefinition]
    relations: list[ConceptRelation]

    @model_validator(mode="after")
    def _relations_reference_known_concepts(self) -> "TargetTypeTemplate":
        known = {c.name for c in self.concepts}
        for relation in self.relations:
            if relation.source not in known:
                raise ValueError(
                    f"relation source {relation.source!r} is not a concept in this template"
                )
            if relation.target not in known:
                raise ValueError(
                    f"relation target {relation.target!r} is not a concept in this template"
                )
        return self
