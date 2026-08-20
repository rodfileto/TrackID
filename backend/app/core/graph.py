"""
Memgraph connection lifecycle - the knowledge graph store for the
Target/Situation/Event/Target Entity ontology (docs/DTID_ARCHITECTURE.md).

Mirrors database.py's module-level-singleton pattern, but the neo4j driver
manages its own connection pool internally (there's no separate
engine/sessionmaker split the way SQLAlchemy has) - so there's just one
driver, created once at startup and closed once at shutdown (see main.py's
lifespan), with get_graph_session() as the per-request dependency.
"""
from typing import AsyncIterator

from neo4j import AsyncDriver, AsyncGraphDatabase, AsyncSession

from app.core.config import settings

_driver: AsyncDriver | None = None


def connect_graph() -> AsyncDriver:
    """Create (or return the already-created) driver. Call once at startup."""
    global _driver
    if _driver is None:
        # Memgraph Community has no auth by default - passing empty auth
        # rather than None keeps the driver from assuming an auth scheme
        # we're not using.
        _driver = AsyncGraphDatabase.driver(settings.MEMGRAPH_URI, auth=None)
    return _driver


async def close_graph() -> None:
    global _driver
    if _driver is not None:
        await _driver.close()
        _driver = None


def get_driver() -> AsyncDriver:
    if _driver is None:
        raise RuntimeError(
            "Graph driver not connected. Ensure connect_graph() runs during app startup."
        )
    return _driver


async def get_graph_session() -> AsyncIterator[AsyncSession]:
    driver = get_driver()
    async with driver.session() as session:
        yield session
