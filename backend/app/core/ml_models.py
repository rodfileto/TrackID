"""
ML model loading and lifecycle management.

Models are loaded once and reused across requests. Call `load_face_app()`
during app startup (see main.py lifespan) so the first HTTP request doesn't
pay the cold-start cost of downloading/initializing the model.
"""
import logging
from typing import Optional

from insightface.app import FaceAnalysis

from app.core.config import settings

logger = logging.getLogger(__name__)

_face_app: Optional[FaceAnalysis] = None


def load_face_app() -> FaceAnalysis:
    """Load (or return the already-loaded) FaceAnalysis instance."""
    global _face_app

    if _face_app is not None:
        return _face_app

    providers = (
        ["CUDAExecutionProvider", "CPUExecutionProvider"]
        if settings.FACE_USE_GPU
        else ["CPUExecutionProvider"]
    )

    logger.info(
        "Loading face model '%s' (providers=%s, det_size=%sx%s, det_thresh=%s)",
        settings.FACE_MODEL_NAME,
        providers,
        settings.FACE_DET_SIZE_WIDTH,
        settings.FACE_DET_SIZE_HEIGHT,
        settings.FACE_DET_THRESH,
    )

    app = FaceAnalysis(name=settings.FACE_MODEL_NAME, providers=providers)
    app.prepare(
        ctx_id=0,
        det_size=(settings.FACE_DET_SIZE_WIDTH, settings.FACE_DET_SIZE_HEIGHT),
        det_thresh=settings.FACE_DET_THRESH,
    )

    _face_app = app
    logger.info("Face model loaded successfully")
    return _face_app


def get_face_app() -> FaceAnalysis:
    """Dependency-friendly accessor. Raises if the model hasn't been loaded yet."""
    if _face_app is None:
        raise RuntimeError(
            "Face model not loaded. Ensure load_face_app() runs during app startup."
        )
    return _face_app
