"""
ML sidecar — the Python process that owns all CV/ML work (InsightFace,
MagFace, video frame processing, DBSCAN clustering). Phase 1 of the Rust
transition (docs/RUST_TRANSITION_PLAN.md): the FastAPI orchestration layer
calls this over HTTP instead of running these models in-process.

Stateless w.r.t. Postgres/Memgraph/MinIO: takes bytes/paths in, returns
plain dicts out. Run from `backend/` with:

    uvicorn ml_sidecar.main:app --host 0.0.0.0 --port 8001
"""
from app.core.gpu_env import ensure_cuda_libs_on_path

# Must run before onnxruntime is ever asked to create a CUDA session —
# same reason as backend/main.py.
ensure_cuda_libs_on_path()

import base64
import logging
import os
import tempfile
import threading
import uuid
from contextlib import asynccontextmanager

import cv2
import numpy as np
from fastapi import FastAPI, File, HTTPException, Query, UploadFile
from pydantic import BaseModel
from starlette.concurrency import run_in_threadpool

from app.core.ml_models import get_face_app, get_quality_service, load_face_app, load_quality_service
from app.core.video_jobs import VideoJobStore
from app.services.face_detection_service import FaceDetectionService, load_image_from_bytes
from app.services.video_processing_service import (
    VideoProcessingService,
    count_expected_frames,
    crop_face_from_frame,
    extract_face_crop_bytes_from_capture,
)

logger = logging.getLogger(__name__)


def _crop_data_uri(image: np.ndarray, bbox: list) -> str | None:
    crop = crop_face_from_frame(image, bbox)
    if crop is None:
        return None
    ok, buf = cv2.imencode(".jpg", crop, [cv2.IMWRITE_JPEG_QUALITY, 95])
    if not ok:
        return None
    return "data:image/jpeg;base64," + base64.b64encode(buf.tobytes()).decode()


@asynccontextmanager
async def lifespan(app: FastAPI):
    load_face_app()
    load_quality_service()
    face_service = FaceDetectionService(
        face_app=get_face_app(), quality_service=get_quality_service()
    )
    app.state.face_service = face_service
    app.state.video_service = VideoProcessingService(face_service=face_service)
    yield


app = FastAPI(title="TrackID ML sidecar", lifespan=lifespan)


@app.get("/ml/v1/health")
async def health():
    return {"status": "ok"}


async def _read_image(file: UploadFile) -> np.ndarray:
    if not file.content_type or not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="File must be an image")
    file_bytes = await file.read()
    try:
        return load_image_from_bytes(file_bytes)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))


@app.post("/ml/v1/detect")
async def detect(file: UploadFile = File(...)):
    image = await _read_image(file)
    faces = await run_in_threadpool(app.state.face_service.detect_faces, image)
    for face in faces:
        if face.get("bbox"):
            crop = _crop_data_uri(image, face["bbox"])
            if crop is not None:
                face["face_crop"] = crop
    return {"faces": faces, "count": len(faces)}


@app.post("/ml/v1/detect-lightweight")
async def detect_lightweight(file: UploadFile = File(...)):
    image = await _read_image(file)
    faces = await run_in_threadpool(app.state.face_service.detect_faces_lightweight, image)
    return {"faces": faces, "count": len(faces)}


class EmbedRequest(BaseModel):
    image_b64: str
    kps: list[list[float]]


@app.post("/ml/v1/embed")
async def embed(req: EmbedRequest):
    image = load_image_from_bytes(base64.b64decode(req.image_b64))
    kps = np.array(req.kps, dtype=np.float32)
    embedding = await run_in_threadpool(
        app.state.face_service.extract_face_embedding, image, kps
    )
    return {"embedding": embedding}


_job_store = VideoJobStore()


def _run_video_job(job_id: str, tmp_path: str, **params) -> None:
    service: VideoProcessingService = params.pop("service")

    def progress(frame_count, expected_frames, _queued, total_detections):
        _job_store.update_progress(job_id, frame_count, expected_frames, total_detections)

    try:
        tracks = service.process_video_for_best_faces(
            video_path=tmp_path,
            progress_callback=progress,
            **params,
        )

        # Single-pass crop extraction: every detection gets a base64 JPEG
        # crop so the orchestration layer can persist it without cv2.
        cap = cv2.VideoCapture(tmp_path)
        try:
            for track in tracks:
                best_face = track["best_face"]
                for det in track["all_faces"]:
                    bbox = det.get("bbox")
                    if not bbox:
                        continue
                    if det is best_face:
                        det["is_best_face"] = True
                    buf = extract_face_crop_bytes_from_capture(cap, det["frame_number"], bbox)
                    if buf is not None:
                        det["face_crop"] = (
                            "data:image/jpeg;base64," + base64.b64encode(buf.getvalue()).decode()
                        )
        finally:
            cap.release()

        _job_store.complete(
            job_id,
            {"tracks": tracks, "track_count": len(tracks), "video_id": None},
        )
    except Exception as exc:
        logger.exception("ML video job %s failed", job_id)
        _job_store.fail(job_id, str(exc))
    finally:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass


@app.post("/ml/v1/video-jobs")
async def create_video_job(
    file: UploadFile = File(...),
    interval_seconds: float = Query(0.5, gt=0),
    embed_interval_seconds: float = Query(1.0, gt=0),
    min_quality: float = Query(0.0, ge=0),
    min_blur: float = Query(50.0, ge=0),
    full_detection_every_frame: bool = Query(False),
    iou_threshold: float = Query(0.15, ge=0, le=1),
    cluster_eps: float = Query(0.45, gt=0),
    cluster_min_samples: int = Query(2, ge=1),
):
    if not file.content_type or not file.content_type.startswith("video/"):
        raise HTTPException(status_code=400, detail="File must be a video")

    suffix = os.path.splitext(file.filename or "")[1] or ".mp4"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp_path = tmp.name
        tmp.write(await file.read())

    expected_frames = count_expected_frames(tmp_path, interval_seconds)
    if expected_frames <= 0:
        os.unlink(tmp_path)
        raise HTTPException(status_code=400, detail="Could not read this file as a video")

    job = _job_store.create(expected_frames=expected_frames)

    threading.Thread(
        target=_run_video_job,
        kwargs=dict(
            job_id=job.job_id,
            tmp_path=tmp_path,
            service=app.state.video_service,
            interval_seconds=interval_seconds,
            embed_interval_seconds=embed_interval_seconds,
            min_quality=min_quality,
            min_blur=min_blur,
            full_detection_every_frame=full_detection_every_frame,
            iou_threshold=iou_threshold,
            cluster_eps=cluster_eps,
            cluster_min_samples=cluster_min_samples,
        ),
        daemon=True,
    ).start()

    return {"job_id": job.job_id}


@app.get("/ml/v1/video-jobs/{job_id}")
async def get_video_job(job_id: str):
    import time

    job = _job_store.get(job_id)
    if job is None:
        raise HTTPException(status_code=404, detail="Job not found")

    elapsed = (job.finished_at or time.monotonic()) - job.started_at
    percent = 0.0
    if job.expected_frames > 0:
        percent = min(100.0, job.frame_count / job.expected_frames * 100)

    return {
        "job_id": job.job_id,
        "status": job.status,
        "frame_count": job.frame_count,
        "expected_frames": job.expected_frames,
        "total_detections": job.total_detections,
        "percent": percent,
        "elapsed_seconds": elapsed,
        "error": job.error,
        "result": job.result,
    }
