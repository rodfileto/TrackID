"""Persist the imported BFO+CCO taxonomy into Memgraph.

Class nodes (`:OntologyClass`) + `SUBCLASS_OF` edges, and relation nodes
(`:ObjectProperty`) + `DOMAIN`/`RANGE` edges. Identity is by IRI (a node
property); the human-readable name is kept as `name`. Multi-label stamping of
instance nodes is a separate concern (`Taxonomy.lineage_labels`) and is not
applied here, since there is no instance layer yet.

Idempotent via MERGE (re-running upserts rather than duplicating). `clear`
supports a clean drop-and-rebuild, per docs/ONTOLOGY_BFO.md.
"""
from __future__ import annotations

from neo4j import AsyncSession

from app.ontology.models import Taxonomy


async def clear_taxonomy(session: AsyncSession) -> None:
    await session.run(
        "MATCH (n) WHERE n:OntologyClass OR n:ObjectProperty DETACH DELETE n"
    )


async def write_taxonomy(session: AsyncSession, taxonomy: Taxonomy) -> dict[str, int]:
    class_rows = [
        {
            "iri": node.iri,
            "name": node.label,
            "definition": node.definition,
            "source": node.source,
        }
        for node in taxonomy.nodes.values()
    ]
    await session.run(
        """
        UNWIND $rows AS row
        MERGE (c:OntologyClass {iri: row.iri})
        SET c.name = row.name, c.definition = row.definition, c.source = row.source
        """,
        rows=class_rows,
    )

    subclass_rows = [
        {"child": node.iri, "parent": parent}
        for node in taxonomy.nodes.values()
        for parent in node.parents
        if parent in taxonomy.nodes
    ]
    await session.run(
        """
        UNWIND $rows AS row
        MATCH (c:OntologyClass {iri: row.child})
        MATCH (p:OntologyClass {iri: row.parent})
        MERGE (c)-[:SUBCLASS_OF]->(p)
        """,
        rows=subclass_rows,
    )

    relation_rows = [
        {
            "iri": relation.iri,
            "name": relation.label,
            "definition": relation.definition,
            "source": relation.source,
            "transitive": relation.transitive,
        }
        for relation in taxonomy.relations.values()
    ]
    await session.run(
        """
        UNWIND $rows AS row
        MERGE (r:ObjectProperty {iri: row.iri})
        SET r.name = row.name, r.definition = row.definition,
            r.transitive = row.transitive, r.source = row.source
        """,
        rows=relation_rows,
    )

    domain_rows = [
        {"relation": relation.iri, "class": cls}
        for relation in taxonomy.relations.values()
        for cls in relation.domain
        if cls in taxonomy.nodes
    ]
    await session.run(
        """
        UNWIND $rows AS row
        MATCH (r:ObjectProperty {iri: row.relation})
        MATCH (c:OntologyClass {iri: row.class})
        MERGE (r)-[:DOMAIN]->(c)
        """,
        rows=domain_rows,
    )

    range_rows = [
        {"relation": relation.iri, "class": cls}
        for relation in taxonomy.relations.values()
        for cls in relation.range
        if cls in taxonomy.nodes
    ]
    await session.run(
        """
        UNWIND $rows AS row
        MATCH (r:ObjectProperty {iri: row.relation})
        MATCH (c:OntologyClass {iri: row.class})
        MERGE (r)-[:RANGE]->(c)
        """,
        rows=range_rows,
    )

    return {
        "classes": len(taxonomy.nodes),
        "relations": len(taxonomy.relations),
        "subclass_edges": len(subclass_rows),
        "domain_edges": len(domain_rows),
        "range_edges": len(range_rows),
    }
