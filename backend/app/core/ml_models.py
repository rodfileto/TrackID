"""
ML model loading and lifecycle management.

Models are loaded once and reused across requests. Call `load_face_app()`
during app startup (see main.py lifespan) so the first HTTP request doesn't
pay the cold-start cost of downloading/initializing the model.
"""
import logging
import os
from typing import Optional

from insightface.app import FaceAnalysis

from app.core.config import settings
from app.services.quality_estimation_service import QualityEstimationService

logger = logging.getLogger(__name__)

_face_app: Optional[FaceAnalysis] = None
_quality_service: Optional[QualityEstimationService] = None
_quality_service_loaded = False


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


def load_quality_service() -> Optional[QualityEstimationService]:
    """
    Load (or return the already-loaded) quality estimator. Returns None if
    the MagFace model file isn't present - quality scoring is optional.
    """
    global _quality_service, _quality_service_loaded

    if _quality_service_loaded:
        return _quality_service

    _quality_service_loaded = True

    model_path = os.path.normpath(settings.FACE_QUALITY_MODEL_PATH)
    if not os.path.exists(model_path):
        logger.warning(
            "MagFace model not found at %s - quality_score will be omitted from results",
            model_path,
        )
        return None

    try:
        _quality_service = QualityEstimationService(
            model_path, use_gpu=settings.FACE_QUALITY_USE_GPU
        )
        logger.info("Loaded MagFace quality model from %s", model_path)
    except Exception:
        logger.exception("Failed to load MagFace quality model")
        _quality_service = None

    return _quality_service


def get_quality_service() -> Optional[QualityEstimationService]:
    """Dependency-friendly accessor. May return None if quality scoring is unavailable."""
    return _quality_service if _quality_service_loaded else load_quality_service()
