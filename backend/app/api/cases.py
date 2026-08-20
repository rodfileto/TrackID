"""
Create/list/get Cases and their Situations - a Case is the formal
forensic/legal procedure (a police case number), deliberately NOT
requiring a Target to exist first: real casework starts from evidence
with zero identified subjects, not from a pre-declared subject of
interest. A Case may later be linked to one or more Targets via
link_case_to_target (and a Target may accumulate several Cases over
time) - see app/graph/schemas.py's module docstring for the full
reasoning. See app/api/resolution.py for evidence upload against a Case.
"""
from fastapi import APIRouter, Depends, HTTPException
from neo4j import AsyncSession

from app.core.graph import get_graph_session
from app.graph.schemas import Case, CaseDetail, Situation, SituationDetail
from app.graph.service import (
    create_case,
    create_situation,
    get_case,
    get_situation,
    link_case_to_target,
    list_cases,
)
from app.schemas.graph import (
    CaseListResponse,
    CreateCaseRequest,
    CreateSituationRequest,
    LinkCaseToTargetRequest,
)

router = APIRouter(prefix="/api/v1/cases", tags=["cases"])


@router.get("", response_model=CaseListResponse)
async def list_cases_endpoint(
    session: AsyncSession = Depends(get_graph_session),
) -> CaseListResponse:
    return CaseListResponse(items=await list_cases(session))


@router.post("", response_model=Case, status_code=201)
async def create_case_endpoint(
    body: CreateCaseRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> Case:
    return await create_case(session, body.name, body.description, body.case_number)


@router.get("/{case_id}", response_model=CaseDetail)
async def get_case_endpoint(
    case_id: str,
    session: AsyncSession = Depends(get_graph_session),
) -> CaseDetail:
    case = await get_case(session, case_id)
    if case is None:
        raise HTTPException(status_code=404, detail=f"Case {case_id!r} not found")
    return case


@router.post("/{case_id}/link-target", status_code=201)
async def link_case_to_target_endpoint(
    case_id: str,
    body: LinkCaseToTargetRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> dict[str, bool]:
    ok = await link_case_to_target(session, case_id, body.target_id)
    if not ok:
        raise HTTPException(
            status_code=404, detail=f"Case {case_id!r} or Target {body.target_id!r} not found"
        )
    return {"ok": True}


@router.post("/{case_id}/situations", response_model=Situation, status_code=201)
async def create_situation_endpoint(
    case_id: str,
    body: CreateSituationRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> Situation:
    situation = await create_situation(session, case_id, "Case", body.name, body.description)
    if situation is None:
        raise HTTPException(status_code=404, detail=f"Case {case_id!r} not found")
    return situation


@router.get("/{case_id}/situations/{situation_id}", response_model=SituationDetail)
async def get_situation_endpoint(
    case_id: str,
    situation_id: str,
    session: AsyncSession = Depends(get_graph_session),
) -> SituationDetail:
    situation = await get_situation(session, situation_id)
    if situation is None or situation.owner_id != case_id or situation.owner_type != "Case":
        raise HTTPException(status_code=404, detail=f"Situation {situation_id!r} not found")
    return situation
