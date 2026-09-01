"""Pure-Python validation guard over the imported BFO+CCO taxonomy.

No graph/database dependency: given the in-memory Taxonomy, these checks decide
whether a proposed edge or class lineage is consistent with BFO/RO domain/range
and acyclic-hierarchy constraints, before anything is written to Memgraph (see
docs/ONTOLOGY_BFO.md "Ingest validation"). Invalid structure is blocked at the
door rather than discovered later.
"""
from __future__ import annotations

from dataclasses import dataclass, field

from app.ontology.models import Taxonomy

# Stable BFO 2.0 class IRIs (http://purl.obolibrary.org/obo/BFO_*).
BFO_ENTITY = "http://purl.obolibrary.org/obo/BFO_0000001"
BFO_CONTINUANT = "http://purl.obolibrary.org/obo/BFO_0000002"
BFO_OCCURRENT = "http://purl.obolibrary.org/obo/BFO_0000003"
BFO_INDEPENDENT_CONTINUANT = "http://purl.obolibrary.org/obo/BFO_0000004"
BFO_PROCESS = "http://purl.obolibrary.org/obo/BFO_0000015"
BFO_SITE = "http://purl.obolibrary.org/obo/BFO_0000029"
BFO_OBJECT = "http://purl.obolibrary.org/obo/BFO_0000030"
BFO_GENERICALLY_DEPENDENT_CONTINUANT = "http://purl.obolibrary.org/obo/BFO_0000031"
BFO_MATERIAL_ENTITY = "http://purl.obolibrary.org/obo/BFO_0000040"
BFO_IMMATERIAL_ENTITY = "http://purl.obolibrary.org/obo/BFO_0000141"


@dataclass
class ValidationResult:
    valid: bool
    errors: list[str] = field(default_factory=list)

    @classmethod
    def ok(cls) -> "ValidationResult":
        return cls(valid=True)


def label(taxonomy: Taxonomy, iri: str) -> str:
    node = taxonomy.get(iri)
    return node.label if node and node.label else iri


def find_class_by_label(taxonomy: Taxonomy, name: str) -> str | None:
    for iri, node in taxonomy.nodes.items():
        if node.label == name:
            return iri
    return None


def find_relation_by_label(taxonomy: Taxonomy, name: str) -> str | None:
    for iri, relation in taxonomy.relations.items():
        if relation.label == name:
            return iri
    return None


def _satisfies(taxonomy: Taxonomy, class_iri: str, constraints: list[str]) -> bool:
    if not constraints:
        return True
    return any(taxonomy.is_subclass_of(class_iri, c) for c in constraints)


def validate_edge(
    taxonomy: Taxonomy,
    subject_iri: str,
    relation_iri: str,
    object_iri: str,
) -> ValidationResult:
    """Check a proposed `subject -[relation]-> object` against domain/range.

    `subject_iri`/`object_iri` are class IRIs; the check accepts the edge if the
    subject class is a subclass of some domain class and the object class is a
    subclass of some range class. The same rule extends to instances once an
    instance layer exists (via each instance's type's lineage).
    """
    errors: list[str] = []
    relation = taxonomy.get_relation(relation_iri)
    if relation is None:
        return ValidationResult(False, [f"unknown relation {relation_iri}"])

    subject_known = subject_iri in taxonomy.nodes
    object_known = object_iri in taxonomy.nodes
    if not subject_known:
        errors.append(f"unknown subject class {subject_iri}")
    if not object_known:
        errors.append(f"unknown object class {object_iri}")

    if subject_known and not _satisfies(taxonomy, subject_iri, relation.domain):
        domain = [label(taxonomy, d) for d in relation.domain] or ["unconstrained"]
        errors.append(
            f"subject {label(taxonomy, subject_iri)} is not in the domain of "
            f"{label(taxonomy, relation_iri)} (domain: {domain})"
        )
    if object_known and not _satisfies(taxonomy, object_iri, relation.range):
        relation_range = [label(taxonomy, r) for r in relation.range] or ["unconstrained"]
        errors.append(
            f"object {label(taxonomy, object_iri)} is not in the range of "
            f"{label(taxonomy, relation_iri)} (range: {relation_range})"
        )

    return ValidationResult(not errors, errors)


def validate_lineage(taxonomy: Taxonomy, class_iri: str) -> ValidationResult:
    """Check a class's position in the hierarchy is well-formed.

    Rejects unknown classes, references to unknown parents, and cycles (a class
    that is its own ancestor).
    """
    errors: list[str] = []
    node = taxonomy.get(class_iri)
    if node is None:
        return ValidationResult(False, [f"unknown class {class_iri}"])

    for parent in node.parents:
        if parent not in taxonomy.nodes:
            errors.append(f"class {label(taxonomy, class_iri)} has unknown parent {parent}")
    if class_iri in taxonomy.ancestors(class_iri):
        errors.append(f"class {label(taxonomy, class_iri)} is its own ancestor (cycle)")

    return ValidationResult(not errors, errors)


def is_occurrent(taxonomy: Taxonomy, class_iri: str) -> bool:
    return taxonomy.is_subclass_of(class_iri, BFO_OCCURRENT)


def is_continuant(taxonomy: Taxonomy, class_iri: str) -> bool:
    return taxonomy.is_subclass_of(class_iri, BFO_CONTINUANT)


def is_material_entity(taxonomy: Taxonomy, class_iri: str) -> bool:
    return taxonomy.is_subclass_of(class_iri, BFO_MATERIAL_ENTITY)


def is_information_content_entity(taxonomy: Taxonomy, class_iri: str) -> bool:
    return taxonomy.is_subclass_of(class_iri, BFO_GENERICALLY_DEPENDENT_CONTINUANT)


@dataclass
class ConformanceReport:
    valid: bool
    violations: list[str] = field(default_factory=list)


def check_conformance(taxonomy: Taxonomy, scope: str = "tid") -> ConformanceReport:
    """Verify that domain classes conform to the BFO/CCO backbone.

    The "lighter" counterpart to app/ontology/reasoner.py (which checks logical
    consistency via a DL reasoner). Pure-Python, no reasoner. `scope` is the
    set of classes to check; "tid" means classes whose source is neither BFO nor
    CCO (i.e. the domain classes we author). Three checks per class:

      - anchoring:     has at least one parent, all of which exist in the taxonomy
      - reachability:  its lineage reaches bfo:Entity
      - partition:     it is not (transitively) under two disjoint classes
    """
    violations: list[str] = []
    for iri, node in taxonomy.nodes.items():
        if scope == "tid" and node.source != "other":
            continue
        name = label(taxonomy, iri)

        if not node.parents:
            violations.append(f"{name}: no parent (must be anchored to a BFO/CCO class)")
        else:
            for parent in node.parents:
                if parent not in taxonomy.nodes:
                    violations.append(f"{name}: dangling parent {parent}")

        ancestors = taxonomy.ancestors(iri)
        if node.parents and BFO_ENTITY not in ancestors:
            violations.append(f"{name}: lineage does not reach bfo:Entity")

        for ancestor in ancestors:
            ancestor_node = taxonomy.get(ancestor)
            if ancestor_node is None:
                continue
            for other in ancestor_node.disjoint_with:
                if other in ancestors:
                    violations.append(
                        f"{name}: under disjoint classes "
                        f"{label(taxonomy, ancestor)} and {label(taxonomy, other)}"
                    )

    return ConformanceReport(valid=not violations, violations=violations)


def _main() -> int:
    import sys

    from app.ontology.importer import load_taxonomy

    report = check_conformance(load_taxonomy())
    print(f"conformant: {report.valid}")
    if report.violations:
        print(f"violations ({len(report.violations)}):")
        for violation in report.violations:
            print(f"  - {violation}")
    else:
        print("violations: none")
    return 0 if report.valid else 1


if __name__ == "__main__":
    raise SystemExit(_main())
