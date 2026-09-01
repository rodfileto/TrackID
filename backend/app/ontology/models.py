"""In-memory representation of an imported ontology (BFO + CCO) taxonomy.

Plain dataclasses, framework-agnostic - the parse result of
app/ontology/importer.py, not yet wired to Memgraph. A ClassNode is what a
`(:Class)` node will materialize from once graph persistence is added (see
docs/ONTOLOGY_BFO.md).
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field


@dataclass
class ClassNode:
    iri: str
    label: str | None = None
    definition: str | None = None
    source: str | None = None
    parents: list[str] = field(default_factory=list)
    disjoint_with: list[str] = field(default_factory=list)


@dataclass
class Relation:
    iri: str
    label: str | None = None
    definition: str | None = None
    source: str | None = None
    domain: list[str] = field(default_factory=list)
    range: list[str] = field(default_factory=list)
    transitive: bool = False
    subproperty_of: list[str] = field(default_factory=list)
    inverse_of: list[str] = field(default_factory=list)


@dataclass
class Taxonomy:
    nodes: dict[str, ClassNode] = field(default_factory=dict)
    relations: dict[str, Relation] = field(default_factory=dict)

    def get(self, iri: str) -> ClassNode | None:
        return self.nodes.get(iri)

    def get_relation(self, iri: str) -> Relation | None:
        return self.relations.get(iri)

    def is_subclass_of(self, child_iri: str, parent_iri: str) -> bool:
        return child_iri == parent_iri or parent_iri in self.ancestors(child_iri)

    def ancestors(self, iri: str) -> set[str]:
        """Transitive closure of named superclasses (IRIs), including parents."""
        seen: set[str] = set()
        stack = list(self.nodes.get(iri, ClassNode(iri)).parents)
        while stack:
            parent = stack.pop()
            if parent in seen:
                continue
            seen.add(parent)
            node = self.nodes.get(parent)
            if node is not None:
                stack.extend(node.parents)
        return seen

    def label_chain(self, iri: str) -> list[str]:
        """Root-to-leaf label path (longest branch), for multi-label stamping."""
        node = self.nodes.get(iri)
        if node is None:
            return []
        seen: frozenset[str] = frozenset()

        def walk(current: str, visited: frozenset[str]) -> list[str]:
            n = self.nodes.get(current)
            if n is None:
                return []
            prefixes = [
                walk(p, visited | {current})
                for p in n.parents
                if p not in visited
            ]
            prefix = max(prefixes, key=len, default=[])
            return prefix + [n.label or current]

        return walk(iri, seen)

    def lineage_labels(self, iri: str) -> list[str]:
        """Sanitized, deduped root-to-leaf label set (Neo4j-label-safe)."""
        labels: list[str] = []
        for raw in self.label_chain(iri):
            clean = sanitize_label(raw)
            if clean not in labels:
                labels.append(clean)
        return labels


def sanitize_label(name: str | None) -> str:
    """Convert an rdfs:label into a valid graph label identifier.

    e.g. "material entity" -> "MaterialEntity", "generically dependent
    continuant" -> "GenericallyDependentContinuant". Collisions are possible
    (two classes with different IRIs can map to the same label); instance
    stamping should prefer the leaf type's own label and treat the ancestry as
    retrieval hints, not identity.
    """
    if not name:
        return "Class"
    words = re.findall(r"[A-Za-z0-9]+", name)
    label = "".join(w[:1].upper() + w[1:] for w in words)
    if not label:
        return "Class"
    if label[0].isdigit():
        label = "C_" + label
    return label
