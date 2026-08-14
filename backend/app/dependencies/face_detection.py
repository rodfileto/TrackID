from app.core.ml_models import get_face_app, get_quality_service
from app.services.face_detection_service import FaceDetectionService


def get_face_detection_service() -> FaceDetectionService:
    return FaceDetectionService(
        face_app=get_face_app(), quality_service=get_quality_service()
    )
