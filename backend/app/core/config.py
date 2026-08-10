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


settings = Settings()
