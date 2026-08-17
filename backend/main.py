from app.core.gpu_env import ensure_cuda_libs_on_path

# Must run before onnxruntime is ever asked to create a CUDA session -
# see app/core/gpu_env.py for why.
ensure_cuda_libs_on_path()

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.face_detection import router as face_detection_router
from app.api.video_processing import router as video_processing_router
from app.api.videos import router as videos_router
from app.core.config import settings
from app.core.ml_models import load_face_app, load_quality_service
from app.core.storage import get_media_storage

logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Load ML models once at startup so the first request doesn't pay the
    # cold-start cost (model download + init).
    load_face_app()
    load_quality_service()
    try:
        get_media_storage().ensure_bucket()
    except Exception:
        # Non-fatal: MinIO isn't required for the app to boot (e.g. plain
        # `python main.py` without docker-compose's minio service) - only
        # video-processing persistence needs it, and that fails loudly on
        # its own when it actually runs.
        logger.warning("Could not reach object storage at startup - bucket not verified", exc_info=True)
    yield


app = FastAPI(
    title=settings.APP_NAME,
    debug=settings.DEBUG,
    lifespan=lifespan,
)

# CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.ALLOWED_ORIGINS,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.get("/health")
async def health_check():
    return {"status": "ok"}


@app.get("/api/v1/health")
async def api_health_check():
    return {"status": "ok", "version": "1.0"}


app.include_router(face_detection_router)
app.include_router(video_processing_router)
app.include_router(videos_router)


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=8000,
        reload=settings.DEBUG,
    )
