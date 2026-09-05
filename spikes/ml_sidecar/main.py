"""Spike 3: mock ML sidecar exposing the §6 contract (docs/RUST_TRANSITION_PLAN.md).

Runs on :8001. No real InsightFace — returns deterministic fake detections so
the spike proves the Rust↔Python HTTP+JSON seam (multipart upload, JSON
round-trip, async job + polling), not the ML itself.
"""
import threading
import time
import uuid

from fastapi import FastAPI, File, HTTPException, Query, UploadFile
from pydantic import BaseModel

app = FastAPI(title="TrackID ML sidecar (spike)")


def _mock_embedding(seed: int) -> list[float]:
    return [(seed * 0.001 + i * 0.0001) % 1.0 for i in range(512)]


def _mock_faces(n: int, *, with_embedding: bool) -> list[dict]:
    faces = []
    for k in range(n):
        f = {
            "bbox": [10.0 + k * 20, 20.0 + k * 20, 110.0 + k * 20, 130.0 + k * 20],
            "confidence": 0.9 - 0.1 * k,
        }
        if with_embedding:
            f["embedding"] = _mock_embedding(k)
            f["estimated_age"] = 30 + k
            f["estimated_gender"] = "M"
            f["landmarks"] = [[10, 10], [20, 10], [15, 20], [12, 30], [18, 30]]
            f["blur_score"] = 120.0 - k * 10
            f["quality_score"] = 0.8 - 0.05 * k
        else:
            f["kps"] = [[10, 10], [20, 10], [15, 20], [12, 30], [18, 30]]
            f["blur_score"] = 120.0 - k * 10
        faces.append(f)
    return faces


@app.post("/ml/v1/detect")
async def detect(file: UploadFile = File(...)):
    await file.read()
    return {"faces": _mock_faces(2, with_embedding=True)}


@app.post("/ml/v1/detect-lightweight")
async def detect_lightweight(file: UploadFile = File(...)):
    await file.read()
    return {"faces": _mock_faces(2, with_embedding=False)}


class EmbedRequest(BaseModel):
    image_id: str
    kps: list[list[float]]


@app.post("/ml/v1/embed")
async def embed(req: EmbedRequest):
    return {"embedding": _mock_embedding(7)}


JOBS: dict[str, dict] = {}
JOBS_LOCK = threading.Lock()


def _run_job(job_id: str, expected_frames: int) -> None:
    job = JOBS[job_id]
    try:
        for i in range(expected_frames):
            time.sleep(0.15)
            job["frame_count"] = i + 1
            job["total_detections"] += 1
        job["status"] = "completed"
        job["result"] = {"tracks": [], "track_count": 0, "video_id": None}
    except Exception as exc:  # pragma: no cover - defensive
        job["status"] = "failed"
        job["error"] = str(exc)


@app.post("/ml/v1/video-jobs")
async def create_video_job(
    file: UploadFile = File(...),
    expected_frames: int = Query(10, ge=1),
):
    await file.read()
    job_id = uuid.uuid4().hex
    with JOBS_LOCK:
        JOBS[job_id] = {
            "job_id": job_id,
            "status": "processing",
            "frame_count": 0,
            "expected_frames": expected_frames,
            "total_detections": 0,
            "result": None,
            "error": None,
        }
    threading.Thread(target=_run_job, args=(job_id, expected_frames), daemon=True).start()
    return {"job_id": job_id}


@app.get("/ml/v1/video-jobs/{job_id}")
async def get_video_job(job_id: str):
    job = JOBS.get(job_id)
    if job is None:
        raise HTTPException(status_code=404, detail="Job not found")
    percent = (
        min(100.0, job["frame_count"] / job["expected_frames"] * 100)
        if job["expected_frames"]
        else 0.0
    )
    return {
        "job_id": job["job_id"],
        "status": job["status"],
        "frame_count": job["frame_count"],
        "expected_frames": job["expected_frames"],
        "total_detections": job["total_detections"],
        "percent": percent,
        "result": job["result"],
        "error": job["error"],
    }
