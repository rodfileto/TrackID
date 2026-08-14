from app.dependencies.face_detection import get_face_detection_service
from app.services.face_detection_service import FaceDetectionService
from app.services.video_processing_service import VideoProcessingService


def get_video_processing_service() -> VideoProcessingService:
    face_service: FaceDetectionService = get_face_detection_service()
    return VideoProcessingService(face_service=face_service)
