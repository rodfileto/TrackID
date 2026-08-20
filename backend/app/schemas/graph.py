from datetime import datetime

from pydantic import BaseModel

from app.graph.schemas import Case, Event, Target, TargetEntity


class CreateTargetRequest(BaseModel):
    name: str
    description: str
    target_type: str


class CreateCaseRequest(BaseModel):
    name: str
    description: str
    case_number: str | None = None


class LinkCaseToTargetRequest(BaseModel):
    target_id: str


class CreateSituationRequest(BaseModel):
    name: str
    description: str


class CreateEventRequest(BaseModel):
    description: str
    occurred_at: datetime | None = None
    # Optional - omit to create a standalone Event with no Situation yet
    # (the common case: an observation arrives before anyone knows what
    # case it belongs to). Set it only if the Situation is already known.
    situation_id: str | None = None


class LinkEventRequest(BaseModel):
    situation_id: str


class CreateEntityRequest(BaseModel):
    entity_type: str
    name: str
    properties: dict[str, str] = {}


class CreateParticipationRequest(BaseModel):
    entity_id: str
    target_id: str  # a Situation id or an Event id
    role: str | None = None


class TargetListResponse(BaseModel):
    items: list[Target]


class CaseListResponse(BaseModel):
    items: list[Case]


class EntityListResponse(BaseModel):
    items: list[TargetEntity]


class EventListResponse(BaseModel):
    items: list[Event]
