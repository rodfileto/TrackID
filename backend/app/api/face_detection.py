from fastapi import APIRouter, Depends, File, HTTPException, UploadFile
from starlette.concurrency import run_in_threadpool

from app.dependencies.face_detection import get_face_detection_service
from app.schemas.face_detection import FaceDetectionResponse
from app.services.face_detection_service import FaceDetectionService, load_image_from_bytes

router = APIRouter(prefix="/api/v1", tags=["face-detection"])


@router.post("/detect-faces", response_model=FaceDetectionResponse)
async def detect_faces(
    file: UploadFile = File(...),
    service: FaceDetectionService = Depends(get_face_detection_service),
) -> FaceDetectionResponse:
    if not file.content_type or not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="File must be an image")

    file_bytes = await file.read()

    try:
        image = load_image_from_bytes(file_bytes)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))

    faces = await run_in_threadpool(service.detect_faces, image)

    return FaceDetectionResponse(faces=faces, count=len(faces))
