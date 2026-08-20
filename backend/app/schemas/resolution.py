"""
Pydantic schemas for the evidence-upload/review-queue API - the
HTTP-facing shape of app/services/entity_resolution_service.py's
ResolutionOutcome and app/models/resolution.py's ReviewQueueItem ORM rows.
"""
from datetime import datetime
from uuid import UUID

from pydantic import BaseModel

from app.services.entity_resolution_service import ResolutionOutcome


class EvidenceUploadResponse(BaseModel):
    outcomes: list[ResolutionOutcome]


class ReviewQueueItemResponse(BaseModel):
    model_config = {"from_attributes": True}

    id: UUID
    embedding_id: UUID
    event_id: str
    case_id: str | None
    candidate_kind: str
    # Exactly one of these two is set, matching candidate_kind - an
    # identity-kind candidate may have no TargetEntity yet (this could be
    # that identity's first matched sighting).
    candidate_target_entity_id: str | None
    candidate_identity_id: str | None
    similarity: float
    top_candidates: list[dict]
    status: str
    reviewed_by: str | None
    reviewed_at: datetime | None
    created_at: datetime


class ReviewQueueListResponse(BaseModel):
    items: list[ReviewQueueItemResponse]
    total: int


class ConfirmReviewItemRequest(BaseModel):
    target_entity_id: str
    reviewed_by: str


class ConfirmReviewItemAsNewRequest(BaseModel):
    entity_name: str
    reviewed_by: str


class ConfirmReviewItemAsIdentityRequest(BaseModel):
    identity_id: str
    reviewed_by: str


class RejectReviewItemRequest(BaseModel):
    reviewed_by: str
    notes: str | None = None
