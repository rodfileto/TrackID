"""Process-global ontology taxonomy lifecycle.

Mirrors app/core/ml_models.py: the BFO+CCO taxonomy is parsed once from the
vendored Turtle files (app/ontology/importer.py) and cached in a module-level
global, then seeded into Memgraph once at startup (class nodes + SUBCLASS_OF
edges + relation nodes + uniqueness constraints). Idempotent: re-running
upserts via MERGE, and Memgraph treats duplicate constraint creation as a
no-op.
"""
from __future__ import annotations

from neo4j import AsyncSession

from app.ontology import importer, persistence
from app.ontology.models import Taxonomy

_taxonomy: Taxonomy | None = None


def load_taxonomy() -> Taxonomy:
    global _taxonomy
    if _taxonomy is None:
        _taxonomy = importer.load_taxonomy()
    return _taxonomy


def get_taxonomy() -> Taxonomy:
    if _taxonomy is None:
        raise RuntimeError(
            "Ontology taxonomy not loaded; ensure load_taxonomy() runs during startup."
        )
    return _taxonomy


async def ensure_constraints(session: AsyncSession) -> None:
    # IRI is the unique identity for both node kinds - this is what makes
    # write_taxonomy's MERGE genuinely unique rather than best-effort.
    await session.run("CREATE CONSTRAINT ON (n:OntologyClass) ASSERT n.iri IS UNIQUE")
    await session.run("CREATE CONSTRAINT ON (n:ObjectProperty) ASSERT n.iri IS UNIQUE")


async def seed_taxonomy(session: AsyncSession) -> dict[str, int]:
    taxonomy = load_taxonomy()
    await ensure_constraints(session)
    return await persistence.write_taxonomy(session, taxonomy)
