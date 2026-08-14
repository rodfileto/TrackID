from pydantic import BaseModel


class VideoFaceDetection(BaseModel):
    frame_number: int
    timestamp_seconds: float
    confidence: float
    quality_score: float | None = None
    blur_score: float | None = None
    bbox: list[float] | None = None
    is_embedding: bool
    estimated_age: int | None = None
    estimated_gender: str | None = None
    cluster_id: int | None = None
    face_crop: str | None = None


class VideoTrack(BaseModel):
    track_id: int
    best_face: VideoFaceDetection
    all_faces: list[VideoFaceDetection]


class VideoProcessingResponse(BaseModel):
    tracks: list[VideoTrack]
    track_count: int
