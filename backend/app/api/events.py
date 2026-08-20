"""
Create/list/get Events, and link an existing (possibly standalone) Event
to a Situation - see app/graph/schemas.py's module docstring on why Event
carries no situation_id field at all: a raw observation can exist before
anyone knows what Situation, if any, it belongs to, and the HAS_EVENT
edge is the only place that relationship is recorded.
"""
from fastapi import APIRouter, Depends, HTTPException, Query
from neo4j import AsyncSession

from app.core.graph import get_graph_session
from app.graph.schemas import Event
from app.graph.service import create_event, get_event, link_event_to_situation, list_events
from app.schemas.graph import CreateEventRequest, EventListResponse, LinkEventRequest

router = APIRouter(prefix="/api/v1/events", tags=["events"])


@router.get("", response_model=EventListResponse)
async def list_events_endpoint(
    unlinked: bool = Query(False, description="Only Events not yet linked to any Situation"),
    session: AsyncSession = Depends(get_graph_session),
) -> EventListResponse:
    return EventListResponse(items=await list_events(session, unlinked_only=unlinked))


@router.post("", response_model=Event, status_code=201)
async def create_event_endpoint(
    body: CreateEventRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> Event:
    event = await create_event(
        session, body.description, body.occurred_at, situation_id=body.situation_id
    )
    if event is None:
        raise HTTPException(
            status_code=404, detail=f"Situation {body.situation_id!r} not found"
        )
    return event


@router.get("/{event_id}", response_model=Event)
async def get_event_endpoint(
    event_id: str,
    session: AsyncSession = Depends(get_graph_session),
) -> Event:
    event = await get_event(session, event_id)
    if event is None:
        raise HTTPException(status_code=404, detail=f"Event {event_id!r} not found")
    return event


@router.post("/{event_id}/link-situation", response_model=Event)
async def link_event_endpoint(
    event_id: str,
    body: LinkEventRequest,
    session: AsyncSession = Depends(get_graph_session),
) -> Event:
    event = await link_event_to_situation(session, event_id, body.situation_id)
    if event is None:
        raise HTTPException(
            status_code=404,
            detail=(
                f"Event {event_id!r} or Situation {body.situation_id!r} not found, "
                "or the Event is already linked to a Situation"
            ),
        )
    return event
