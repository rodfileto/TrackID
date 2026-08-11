from pydantic import BaseModel


class FaceDetectionResult(BaseModel):
    embedding: list[float]
    bbox: list[float]
    confidence: float
    estimated_age: int | None = None
    estimated_gender: str | None = None
    landmarks: list[list[float]] | None = None
    blur_score: float | None = None


class FaceDetectionResponse(BaseModel):
    faces: list[FaceDetectionResult]
    count: int
