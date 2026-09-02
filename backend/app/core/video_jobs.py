"""
In-memory job tracking for background video processing.

Video processing can take a while, so the API kicks it off in a
background thread and hands back a job_id immediately; clients poll for
progress. Nothing here is persisted - jobs live only for the process's
lifetime, which is fine for a single-instance testing/dev tool. If this
ever needs to survive restarts or run across multiple workers, swap this
for the taskiq queue already in requirements.txt instead of adding
persistence here.
"""
import threading
import time
import uuid
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class VideoJob:
    job_id: str
    status: str = "processing"  # "processing" | "completed" | "failed"
    frame_count: int = 0
    expected_frames: int = 0
    total_detections: int = 0
    started_at: float = field(default_factory=time.monotonic)
    finished_at: Optional[float] = None
    result: Optional[dict] = None
    error: Optional[str] = None
    # Phase 1: id of the corresponding job on the ML sidecar. The
    # orchestration layer polls the sidecar for progress and persists the
    # result once it completes.
    sidecar_job_id: Optional[str] = None


class VideoJobStore:
    def __init__(self):
        self._jobs: dict[str, VideoJob] = {}
        self._lock = threading.Lock()

    def create(self, expected_frames: int = 0) -> VideoJob:
        job = VideoJob(job_id=str(uuid.uuid4()), expected_frames=expected_frames)
        with self._lock:
            self._jobs[job.job_id] = job
        return job

    def get(self, job_id: str) -> Optional[VideoJob]:
        with self._lock:
            return self._jobs.get(job_id)

    def update_progress(
        self, job_id: str, frame_count: int, expected_frames: int, total_detections: int
    ) -> None:
        with self._lock:
            job = self._jobs.get(job_id)
            if job is None:
                return
            job.frame_count = frame_count
            # The service's own count can differ slightly from the upfront
            # estimate (metadata-based frame counts aren't always exact) -
            # trust whichever is larger so percent never exceeds 100.
            job.expected_frames = max(job.expected_frames, expected_frames)
            job.total_detections = total_detections

    def complete(self, job_id: str, result: dict) -> None:
        with self._lock:
            job = self._jobs.get(job_id)
            if job is None:
                return
            job.status = "completed"
            job.result = result
            job.finished_at = time.monotonic()

    def fail(self, job_id: str, error: str) -> None:
        with self._lock:
            job = self._jobs.get(job_id)
            if job is None:
                return
            job.status = "failed"
            job.error = error
            job.finished_at = time.monotonic()


_store = VideoJobStore()


def get_video_job_store() -> VideoJobStore:
    return _store
