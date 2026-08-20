"""
Evidence upload (Case evidence photo -> detected faces -> confidence-
gated resolution) and the analyst review queue - the HTTP surface over
app/services/entity_resolution_service.py. See that module's docstring
for the resolution pipeline itself.
"""
import uuid

import cv2
from fastapi import APIRouter, Depends, File, HTTPException, Query, UploadFile
from neo4j import AsyncSession as GraphSession
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession as PgSession
from starlette.concurrency import run_in_threadpool

from app.core.database import get_db
from app.core.graph import get_graph_session
from app.core.storage import get_media_storage
from app.dependencies.face_detection import get_face_detection_service
from app.graph.service import create_event, get_case
from app.models.resolution import ReviewQueueItem
from app.schemas.resolution import (
    ConfirmReviewItemAsIdentityRequest,
    ConfirmReviewItemAsNewRequest,
    ConfirmReviewItemRequest,
    EvidenceUploadResponse,
    RejectReviewItemRequest,
    ReviewQueueItemResponse,
    ReviewQueueListResponse,
)
from app.services.entity_resolution_service import (
    confirm_review_item,
    confirm_review_item_as_identity,
    confirm_review_item_as_new,
    reject_review_item,
    resolve_face_observation,
)
from app.services.face_detection_service import FaceDetectionService, load_image_from_bytes
from app.services.video_processing_service import crop_face_from_frame

router = APIRouter(prefix="/api/v1", tags=["resolution"])


@router.post("/cases/{case_id}/evidence", response_model=EvidenceUploadResponse)
async def upload_case_evidence(
    case_id: str,
    file: UploadFile = File(...),
    pg_session: PgSession = Depends(get_db),
    graph_session: GraphSession = Depends(get_graph_session),
    face_service: FaceDetectionService = Depends(get_face_detection_service),
) -> EvidenceUploadResponse:
    if await get_case(graph_session, case_id) is None:
        raise HTTPException(status_code=404, detail=f"Case {case_id!r} not found")

    if not file.content_type or not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="File must be an image")

    file_bytes = await file.read()
    try:
        image = load_image_from_bytes(file_bytes)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))

    faces = await run_in_threadpool(face_service.detect_faces, image)

    storage = get_media_storage()
    outcomes = []
    for face in faces:
        # Standalone Event, no situation yet - matches "raw evidence, zero
        # identified subjects" until an analyst assembles a narrative
        # around it (see app/graph/schemas.py's module docstring).
        event = await create_event(graph_session, "Face detected in case evidence upload")

        source_key = None
        crop = crop_face_from_frame(image, face["bbox"])
        if crop is not None:
            success, buffer = cv2.imencode(".jpg", crop, [cv2.IMWRITE_JPEG_QUALITY, 95])
            if success:
                key = f"cases/{case_id}/evidence/{event.id}/crop.jpg"
                storage.upload_bytes(buffer.tobytes(), key, content_type="image/jpeg")
                source_key = key

        outcome = await resolve_face_observation(
            pg_session,
            graph_session,
            event_id=event.id,
            embedding=face["embedding"],
            case_id=case_id,
            quality_score=face.get("quality_score"),
            detection_confidence=face.get("confidence"),
            source_image_storage_key=source_key,
            actor="system",
        )
        outcomes.append(outcome)

    return EvidenceUploadResponse(outcomes=outcomes)


@router.get("/review-queue", response_model=ReviewQueueListResponse)
async def list_review_queue(
    status: str | None = Query(default=None),
    case_id: str | None = Query(default=None),
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=20, ge=1, le=100),
    pg_session: PgSession = Depends(get_db),
) -> ReviewQueueListResponse:
    filters = []
    if status is not None:
        filters.append(ReviewQueueItem.status == status)
    if case_id is not None:
        filters.append(ReviewQueueItem.case_id == case_id)

    total = (
        await pg_session.execute(select(func.count()).select_from(ReviewQueueItem).filter(*filters))
    ).scalar_one()
    items = (
        await pg_session.execute(
            select(ReviewQueueItem)
            .filter(*filters)
            .order_by(ReviewQueueItem.created_at.desc())
            .offset((page - 1) * page_size)
            .limit(page_size)
        )
    ).scalars().all()
    return ReviewQueueListResponse(items=items, total=total)


@router.get("/review-queue/{item_id}", response_model=ReviewQueueItemResponse)
async def get_review_queue_item(
    item_id: uuid.UUID,
    pg_session: PgSession = Depends(get_db),
) -> ReviewQueueItem:
    item = await pg_session.get(ReviewQueueItem, item_id)
    if item is None:
        raise HTTPException(status_code=404, detail=f"Review queue item {item_id} not found")
    return item


@router.post("/review-queue/{item_id}/confirm", response_model=ReviewQueueItemResponse)
async def confirm_review_queue_item(
    item_id: uuid.UUID,
    body: ConfirmReviewItemRequest,
    pg_session: PgSession = Depends(get_db),
    graph_session: GraphSession = Depends(get_graph_session),
) -> ReviewQueueItem:
    item = await confirm_review_item(
        pg_session,
        graph_session,
        item_id=item_id,
        target_entity_id=body.target_entity_id,
        actor=body.reviewed_by,
    )
    if item is None:
        raise HTTPException(
            status_code=404,
            detail=(
                f"Review queue item {item_id} not found, already reviewed, "
                f"or target entity {body.target_entity_id!r} does not exist"
            ),
        )
    return item


@router.post("/review-queue/{item_id}/confirm-new", response_model=ReviewQueueItemResponse)
async def confirm_review_queue_item_as_new(
    item_id: uuid.UUID,
    body: ConfirmReviewItemAsNewRequest,
    pg_session: PgSession = Depends(get_db),
    graph_session: GraphSession = Depends(get_graph_session),
) -> ReviewQueueItem:
    item = await confirm_review_item_as_new(
        pg_session, graph_session, item_id=item_id, entity_name=body.entity_name, actor=body.reviewed_by
    )
    if item is None:
        raise HTTPException(status_code=404, detail=f"Review queue item {item_id} not found or already reviewed")
    return item


@router.post("/review-queue/{item_id}/confirm-identity", response_model=ReviewQueueItemResponse)
async def confirm_review_queue_item_as_identity(
    item_id: uuid.UUID,
    body: ConfirmReviewItemAsIdentityRequest,
    pg_session: PgSession = Depends(get_db),
    graph_session: GraphSession = Depends(get_graph_session),
) -> ReviewQueueItem:
    item = await confirm_review_item_as_identity(
        pg_session, graph_session, item_id=item_id, identity_id=body.identity_id, actor=body.reviewed_by
    )
    if item is None:
        raise HTTPException(
            status_code=404,
            detail=(
                f"Review queue item {item_id} not found, already reviewed, "
                f"or identity {body.identity_id!r} does not exist"
            ),
        )
    return item


@router.post("/review-queue/{item_id}/reject", response_model=ReviewQueueItemResponse)
async def reject_review_queue_item(
    item_id: uuid.UUID,
    body: RejectReviewItemRequest,
    pg_session: PgSession = Depends(get_db),
    graph_session: GraphSession = Depends(get_graph_session),
) -> ReviewQueueItem:
    item = await reject_review_item(
        pg_session, graph_session, item_id=item_id, actor=body.reviewed_by, notes=body.notes
    )
    if item is None:
        raise HTTPException(status_code=404, detail=f"Review queue item {item_id} not found or already reviewed")
    return item
