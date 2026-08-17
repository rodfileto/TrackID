from datetime import datetime

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
    video_id: str | None = None


class VideoSummary(BaseModel):
    video_id: str
    original_filename: str | None = None
    created_at: datetime
    track_count: int
    frame_count: int
    total_detections: int


class VideoListResponse(BaseModel):
    items: list[VideoSummary]
    total: int
    page: int
    page_size: int


class VideoJobCreated(BaseModel):
    job_id: str


class VideoJobStatus(BaseModel):
    job_id: str
    status: str  # "processing" | "completed" | "failed"
    frame_count: int
    expected_frames: int
    percent: float
    elapsed_seconds: float
    eta_seconds: float | None = None
    error: str | None = None
    result: VideoProcessingResponse | None = None
