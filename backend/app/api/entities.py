"""
Create/list/get Target Entities - the instance-layer participants
(TargetPerson, Vehicle, etc.) per docs/DTID_ARCHITECTURE.md.
entity_type is validated against the EntityTypeDefinition registry
(app/ontology/entity_types), same pattern as Target.target_type against
TargetTypeTemplate.
"""
from fastapi import APIRouter, Depends, HTTPException, Query
from neo4j import AsyncSession

from app.core.graph import get_graph_session
from app.graph.schemas import TargetEntity, TargetEntityDetail
from app.graph.service import create_entity, get_entity_detail, list_entities
from app.ontology.entity_types.registry import get_builtin_entity_type
from app.schemas.graph import CreateEntityRequest, EntityListResponse

router = APIRouter(prefix="/api/v1/entities", tags=["entities"])


@router.get("", response_model=EntityListResponse)
async def list_entities_endpoint(
    identified: bool | None = Query(default=None),
    session: AsyncSession = Depends(get_graph_session),
) -> EntityListResponse:
    return EntityListResponse(items=await list_entities(session, identified=identified))


@router.post("", response_model=TargetEntity, status_code=201)
async def create_entity_endpoint(
    body: CreateEntityRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> TargetEntity:
    # Only built-ins exist right now - no user-defined EntityTypeDefinition
    # storage/API yet (same gap noted in app/ontology/entity_types/, this
    # just inherits it rather than papering over it).
    if get_builtin_entity_type(body.entity_type) is None:
        raise HTTPException(
            status_code=422,
            detail=f"entity_type {body.entity_type!r} does not match any known EntityTypeDefinition",
        )
    return await create_entity(session, body.entity_type, body.name, body.properties)


@router.get("/{entity_id}", response_model=TargetEntityDetail)
async def get_entity_endpoint(
    entity_id: str,
    session: AsyncSession = Depends(get_graph_session),
) -> TargetEntityDetail:
    entity = await get_entity_detail(session, entity_id)
    if entity is None:
        raise HTTPException(status_code=404, detail=f"Entity {entity_id!r} not found")
    return entity
