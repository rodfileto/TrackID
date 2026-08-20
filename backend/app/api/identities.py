"""
Identity enrollment CRUD - document-backed biometric records, see
app/graph/schemas.py's Identity model and
app/services/entity_resolution_service.py's identity-kind candidate
matching / enroll_identity.
"""
import uuid

import cv2
from fastapi import APIRouter, Depends, File, Form, HTTPException, UploadFile
from neo4j import AsyncSession as GraphSession
from sqlalchemy.ext.asyncio import AsyncSession as PgSession
from starlette.concurrency import run_in_threadpool

from app.core.database import get_db
from app.core.graph import get_graph_session
from app.core.storage import get_media_storage
from app.dependencies.face_detection import get_face_detection_service
from app.graph.schemas import Identity
from app.graph.service import attach_identity_to_entity, get_identity
from app.schemas.identities import AttachIdentityRequest
from app.services.entity_resolution_service import enroll_identity
from app.services.face_detection_service import FaceDetectionService, load_image_from_bytes
from app.services.video_processing_service import crop_face_from_frame

router = APIRouter(prefix="/api/v1/identities", tags=["identities"])


@router.post("", response_model=Identity, status_code=201)
async def create_identity_endpoint(
    full_name: str = Form(...),
    document_type: str | None = Form(default=None),
    document_number: str | None = Form(default=None),
    fingerprint_template: str | None = Form(default=None),
    photo: UploadFile | None = File(default=None),
    pg_session: PgSession = Depends(get_db),
    graph_session: GraphSession = Depends(get_graph_session),
    face_service: FaceDetectionService = Depends(get_face_detection_service),
) -> Identity:
    """
    Mixes text fields and an optional file upload, so this uses Form(...)
    /File(...) params directly rather than a single Pydantic body - no
    existing precedent in this codebase for a Pydantic-model-as-
    multipart-form, and app/api/resolution.py's upload_case_evidence
    already established the bare-UploadFile pattern for the file-only
    case.

    An enrollment photo's ambiguity is a real error condition (unlike a
    case-evidence photo, where multiple faces are expected and fine) -
    this is meant to be one specific person's reference document photo.
    """
    face_embedding = None
    source_key = None

    if photo is not None:
        if not photo.content_type or not photo.content_type.startswith("image/"):
            raise HTTPException(status_code=400, detail="Photo must be an image")
        file_bytes = await photo.read()
        try:
            image = load_image_from_bytes(file_bytes)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc))

        faces = await run_in_threadpool(face_service.detect_faces, image)
        if len(faces) == 0:
            raise HTTPException(status_code=422, detail="No face detected in enrollment photo")
        if len(faces) > 1:
            raise HTTPException(
                status_code=422,
                detail=f"Enrollment photo must contain exactly one face, found {len(faces)}",
            )
        face = faces[0]
        face_embedding = face["embedding"]

        crop = crop_face_from_frame(image, face["bbox"])
        if crop is not None:
            success, buffer = cv2.imencode(".jpg", crop, [cv2.IMWRITE_JPEG_QUALITY, 95])
            if success:
                key = f"identities/enrollment/{uuid.uuid4()}/photo.jpg"
                get_media_storage().upload_bytes(buffer.tobytes(), key, content_type="image/jpeg")
                source_key = key

    if face_embedding is None and fingerprint_template is None:
        raise HTTPException(
            status_code=422, detail="Identity requires a reference photo or a fingerprint_template"
        )

    return await enroll_identity(
        pg_session,
        graph_session,
        full_name=full_name,
        document_type=document_type,
        document_number=document_number,
        fingerprint_template=fingerprint_template,
        face_embedding=face_embedding,
        source_image_storage_key=source_key,
    )


@router.get("/{identity_id}", response_model=Identity)
async def get_identity_endpoint(
    identity_id: str,
    session: GraphSession = Depends(get_graph_session),
) -> Identity:
    identity = await get_identity(session, identity_id)
    if identity is None:
        raise HTTPException(status_code=404, detail=f"Identity {identity_id!r} not found")
    return identity


@router.post("/{identity_id}/attach", status_code=201)
async def attach_identity_endpoint(
    identity_id: str,
    body: AttachIdentityRequest,
    session: GraphSession = Depends(get_graph_session),
) -> dict[str, bool]:
    ok = await attach_identity_to_entity(session, identity_id, body.target_entity_id)
    if not ok:
        raise HTTPException(
            status_code=404,
            detail=(
                f"Identity {identity_id!r} or TargetEntity {body.target_entity_id!r} "
                "(entity_type=TargetPerson) not found"
            ),
        )
    return {"ok": True}
