from pydantic_settings import BaseSettings
import os


class Settings(BaseSettings):
    # App
    APP_NAME: str = "TrackID"
    DEBUG: bool = os.getenv("DEBUG", "False").lower() == "true"

    # Database
    DATABASE_URL: str = os.getenv(
        "DATABASE_URL",
        "postgresql+asyncpg://postgres:password@localhost:5432/trackid"
    )

    # Redis (for task queue)
    REDIS_URL: str = os.getenv("REDIS_URL", "redis://localhost:6379")

    # Object storage (S3-compatible / MinIO locally, swappable to real AWS S3
    # in prod by changing endpoint/credentials only)
    # Host port shifted to 9010 for local (non-docker) dev - see
    # docker-compose.dev.yml's minio service for why.
    S3_ENDPOINT_URL: str = os.getenv("S3_ENDPOINT_URL", "http://localhost:9010")
    S3_ACCESS_KEY_ID: str = os.getenv("S3_ACCESS_KEY_ID", "minioadmin")
    S3_SECRET_ACCESS_KEY: str = os.getenv("S3_SECRET_ACCESS_KEY", "minioadmin")
    S3_BUCKET_NAME: str = os.getenv("S3_BUCKET_NAME", "trackid-media")
    S3_REGION_NAME: str = os.getenv("S3_REGION_NAME", "us-east-1")
    S3_USE_SSL: bool = os.getenv("S3_USE_SSL", "False").lower() == "true"
    # Endpoint used only when SIGNING presigned URLs handed to the browser.
    # Inside docker-compose, S3_ENDPOINT_URL is the container-network
    # hostname (http://minio:9000) which the browser can't resolve - this
    # must stay a host-reachable address.
    S3_PUBLIC_URL: str = os.getenv("S3_PUBLIC_URL", "http://localhost:9010")

    # CORS
    ALLOWED_ORIGINS: list = ["http://localhost:5173", "http://localhost:3000"]

    # API
    API_V1_PREFIX: str = "/api/v1"

    # Face recognition model
    FACE_MODEL_NAME: str = os.getenv("FACE_MODEL_NAME", "buffalo_l")
    FACE_USE_GPU: bool = os.getenv("FACE_USE_GPU", "False").lower() == "true"
    FACE_DET_SIZE_WIDTH: int = int(os.getenv("FACE_DET_SIZE_WIDTH", "640"))
    FACE_DET_SIZE_HEIGHT: int = int(os.getenv("FACE_DET_SIZE_HEIGHT", "640"))
    FACE_DET_THRESH: float = float(os.getenv("FACE_DET_THRESH", "0.5"))

    # Face quality estimation (MagFace). Optional - if the model file isn't
    # present, quality_score is simply omitted from detection results.
    FACE_QUALITY_MODEL_PATH: str = os.getenv(
        "FACE_QUALITY_MODEL_PATH",
        os.path.join(os.path.dirname(__file__), "..", "..", "models", "magface_iresnet50.onnx"),
    )
    FACE_QUALITY_USE_GPU: bool = os.getenv("FACE_QUALITY_USE_GPU", "False").lower() == "true"


settings = Settings()
