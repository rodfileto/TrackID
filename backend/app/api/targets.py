"""
Create/list/get Targets and their Situations - the instance layer of the
ontology (docs/DTID_ARCHITECTURE.md), stored in Memgraph. See
app/graph/service.py for the underlying Cypher.
"""
from fastapi import APIRouter, Depends, HTTPException
from neo4j import AsyncSession

from app.core.graph import get_graph_session
from app.graph.schemas import Situation, SituationDetail, Target, TargetDetail
from app.graph.service import (
    add_participation,
    create_situation,
    create_target,
    get_situation,
    get_target,
    list_targets,
)
from app.schemas.graph import (
    CreateParticipationRequest,
    CreateSituationRequest,
    CreateTargetRequest,
    TargetListResponse,
)

router = APIRouter(prefix="/api/v1/targets", tags=["targets"])


@router.get("", response_model=TargetListResponse)
async def list_targets_endpoint(
    session: AsyncSession = Depends(get_graph_session),
) -> TargetListResponse:
    return TargetListResponse(items=await list_targets(session))


@router.post("", response_model=Target, status_code=201)
async def create_target_endpoint(
    body: CreateTargetRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> Target:
    # target_type is currently a free string - the BFO domain registry
    # (app/ontology/domain) will re-add validation when it is wired in.
    return await create_target(session, body.name, body.description, body.target_type)


@router.get("/{target_id}", response_model=TargetDetail)
async def get_target_endpoint(
    target_id: str,
    session: AsyncSession = Depends(get_graph_session),
) -> TargetDetail:
    target = await get_target(session, target_id)
    if target is None:
        raise HTTPException(status_code=404, detail=f"Target {target_id!r} not found")
    return target


@router.post("/{target_id}/situations", response_model=Situation, status_code=201)
async def create_situation_endpoint(
    target_id: str,
    body: CreateSituationRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> Situation:
    situation = await create_situation(session, target_id, "Target", body.name, body.description)
    if situation is None:
        raise HTTPException(status_code=404, detail=f"Target {target_id!r} not found")
    return situation


@router.get("/{target_id}/situations/{situation_id}", response_model=SituationDetail)
async def get_situation_endpoint(
    target_id: str,
    situation_id: str,
    session: AsyncSession = Depends(get_graph_session),
) -> SituationDetail:
    situation = await get_situation(session, situation_id)
    if situation is None or situation.owner_id != target_id or situation.owner_type != "Target":
        raise HTTPException(status_code=404, detail=f"Situation {situation_id!r} not found")
    return situation


participations_router = APIRouter(prefix="/api/v1/participations", tags=["targets"])


@participations_router.post("", status_code=201)
async def create_participation_endpoint(
    body: CreateParticipationRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> dict[str, bool]:
    """
    Link a TargetEntity to a Situation or an Event, with an optional role
    (e.g. "victim", "agent", "witness") - see app/graph/schemas.py's
    module docstring for why participation can target either.
    """
    ok = await add_participation(session, body.entity_id, body.target_id, body.role)
    if not ok:
        raise HTTPException(
            status_code=404,
            detail=f"Entity {body.entity_id!r} or target {body.target_id!r} not found",
        )
    return {"ok": True}
