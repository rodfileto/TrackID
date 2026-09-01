"""Parse the vendored BFO + CCO Turtle files into an in-memory Taxonomy.

Structural import only: classes, their named rdfs:subClassOf parents, labels,
and definitions. No OWL reasoning - restriction/intersection class expressions
(blank nodes) are ignored, which is exactly the "bake semantics into structure"
approach the project uses (see docs/ONTOLOGY_BFO.md).
"""
from __future__ import annotations

from pathlib import Path

from rdflib import BNode, Graph, Literal, URIRef
from rdflib.collection import Collection
from rdflib.namespace import OWL, RDF, RDFS, SKOS

from app.ontology.models import ClassNode, Relation, Taxonomy

VENDOR_DIR = Path(__file__).resolve().parent / "vendor"

_BFO_IRI_PREFIX = "http://purl.obolibrary.org/obo/BFO_"
_CCO_IRI_PREFIX = "https://www.commoncoreontologies.org/"


def _source_for(iri: str) -> str:
    if iri.startswith(_BFO_IRI_PREFIX):
        return "bfo"
    if iri.startswith(_CCO_IRI_PREFIX):
        return "cco"
    return "other"


def _preferred_literal(graph: Graph, subject: URIRef, predicate) -> str | None:
    """Best-effort English (or lang-less) literal for a subject/predicate."""
    values = list(graph.objects(subject, predicate))
    if not values:
        return None
    for value in values:
        if isinstance(value, Literal) and value.language == "en":
            return str(value)
    for value in values:
        if isinstance(value, Literal) and value.language is None:
            return str(value)
    for value in values:
        if isinstance(value, Literal):
            return str(value)
    return None


def _definition_for(graph: Graph, subject: URIRef) -> str | None:
    return (
        _preferred_literal(graph, subject, SKOS.definition)
        or _preferred_literal(graph, subject, RDFS.comment)
    )


def ontology_files(vendor_dir: Path) -> list[Path]:
    """All vendored Turtle files, in import order: BFO, then CCO, then `tid:`.

    The `tid:` directory holds the (future) TrackID domain ontology; dropping a
    file there is picked up automatically by the importer, the persistence
    seed, and the reasoner.
    """
    bfo_files = sorted((vendor_dir / "bfo").glob("*.ttl"))
    cco_files = sorted((vendor_dir / "cco").rglob("*.ttl"))
    tid_files = sorted((vendor_dir / "tid").rglob("*.ttl"))
    return bfo_files + cco_files + tid_files


def _as_list(graph: Graph, node) -> list:
    """Iterate an RDF collection if `node` is one, else return it as a singleton."""
    if not isinstance(node, BNode):
        return [node]
    try:
        return list(Collection(graph, node))
    except Exception:
        return [node]


def _named_classes_in(graph: Graph, node) -> list[str]:
    """Flatten an OWL class expression into the named classes it *includes*.

    Approximate: union/intersection members and restriction fillers
    (some/allValuesFrom, onClass) are collected, but complementOf is skipped
    (those classes are excluded, not included). Good enough for recording
    domain/range; a validation guard must treat flattened complex domains as
    approximate.
    """
    found: set[str] = set()

    def visit(current) -> None:
        if isinstance(current, URIRef):
            found.add(str(current))
            return
        if not isinstance(current, BNode):
            return
        for pred in (OWL.unionOf, OWL.intersectionOf):
            for obj in graph.objects(current, pred):
                for item in _as_list(graph, obj):
                    visit(item)
        for pred in (OWL.someValuesFrom, OWL.allValuesFrom, OWL.onClass):
            for obj in graph.objects(current, pred):
                visit(obj)

    visit(node)
    return sorted(found)


def load_taxonomy(vendor_dir: Path | None = None) -> Taxonomy:
    vendor_dir = vendor_dir or VENDOR_DIR
    graph = Graph()
    for path in ontology_files(vendor_dir):
        graph.parse(path, format="turtle")

    class_iris = set(graph.subjects(RDF.type, OWL.Class))
    class_iris |= set(graph.subjects(RDF.type, RDFS.Class))

    nodes: dict[str, ClassNode] = {}
    for subject in class_iris:
        if not isinstance(subject, URIRef):
            continue
        iri = str(subject)
        parents = [
            str(parent)
            for parent in graph.objects(subject, RDFS.subClassOf)
            if isinstance(parent, URIRef)
        ]
        nodes[iri] = ClassNode(
            iri=iri,
            label=_preferred_literal(graph, subject, RDFS.label),
            definition=_definition_for(graph, subject),
            source=_source_for(iri),
            parents=parents,
            disjoint_with=[
                str(other)
                for other in graph.objects(subject, OWL.disjointWith)
                if isinstance(other, URIRef)
            ],
        )

    relations: dict[str, Relation] = {}
    for subject in graph.subjects(RDF.type, OWL.ObjectProperty):
        if not isinstance(subject, URIRef):
            continue
        iri = str(subject)
        domain = sorted(
            {
                cls
                for node in graph.objects(subject, RDFS.domain)
                for cls in _named_classes_in(graph, node)
            }
        )
        relation_range = sorted(
            {
                cls
                for node in graph.objects(subject, RDFS.range)
                for cls in _named_classes_in(graph, node)
            }
        )
        relations[iri] = Relation(
            iri=iri,
            label=_preferred_literal(graph, subject, RDFS.label),
            definition=_definition_for(graph, subject),
            source=_source_for(iri),
            domain=domain,
            range=relation_range,
            transitive=(subject, RDF.type, OWL.TransitiveProperty) in graph,
            subproperty_of=[
                str(obj)
                for obj in graph.objects(subject, RDFS.subPropertyOf)
                if isinstance(obj, URIRef)
            ],
            inverse_of=[
                str(obj)
                for obj in graph.objects(subject, OWL.inverseOf)
                if isinstance(obj, URIRef)
            ],
        )

    return Taxonomy(nodes=nodes, relations=relations)
