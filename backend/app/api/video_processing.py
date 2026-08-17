import logging
import os
import tempfile
import threading
import time
import uuid

from fastapi import APIRouter, Depends, File, HTTPException, Query, UploadFile

from app.core.storage import get_media_storage
from app.core.video_jobs import VideoJobStore, get_video_job_store
from app.dependencies.video_processing import get_video_processing_service
from app.schemas.video_processing import (
    VideoJobCreated,
    VideoJobStatus,
    VideoProcessingResponse,
)
from app.services.media_storage_service import upload_video_and_crops
from app.services.video_persistence_service import persist_video_result_sync
from app.services.video_processing_service import (
    VideoProcessingService,
    count_expected_frames,
    extract_face_crop_data_uri,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1", tags=["video-processing"])


def _run_job(
    job_id: str,
    job_store: VideoJobStore,
    service: VideoProcessingService,
    tmp_path: str,
    original_filename: str,
    content_type: str,
    interval_seconds: float,
    embed_interval_seconds: float,
    min_quality: float,
    min_blur: float,
    full_detection_every_frame: bool,
    iou_threshold: float,
    cluster_eps: float,
    cluster_min_samples: int,
    include_crops: bool,
) -> None:
    try:
        tracks = service.process_video_for_best_faces(
            video_path=tmp_path,
            interval_seconds=interval_seconds,
            min_quality=min_quality,
            min_blur=min_blur,
            embed_interval_seconds=embed_interval_seconds,
            full_detection_every_frame=full_detection_every_frame,
            iou_threshold=iou_threshold,
            cluster_eps=cluster_eps,
            cluster_min_samples=cluster_min_samples,
            progress_callback=lambda frame_count, expected_frames, _queued, total_detections: (
                job_store.update_progress(job_id, frame_count, expected_frames, total_detections)
            ),
        )
        frame_count, expected_frames, total_detections = _job_counts(job_store, job_id)

        if include_crops:
            for track in tracks:
                best = track["best_face"]
                if best.get("bbox") is not None:
                    best["face_crop"] = extract_face_crop_data_uri(
                        tmp_path, best["frame_number"], best["bbox"]
                    )

        video_id = uuid.uuid4()
        try:
            video_storage_key, tracks = upload_video_and_crops(
                get_media_storage(), video_id, tmp_path, content_type, tracks
            )
            persist_video_result_sync(
                video_id=video_id,
                job_id=job_id,
                original_filename=original_filename,
                content_type=content_type,
                video_storage_key=video_storage_key,
                params={
                    "interval_seconds": interval_seconds,
                    "embed_interval_seconds": embed_interval_seconds,
                    "min_quality": min_quality,
                    "min_blur": min_blur,
                    "full_detection_every_frame": full_detection_every_frame,
                    "iou_threshold": iou_threshold,
                    "cluster_eps": cluster_eps,
                    "cluster_min_samples": cluster_min_samples,
                },
                expected_frames=expected_frames,
                frame_count=frame_count,
                total_detections=total_detections,
                tracks=tracks,
            )
        except Exception:
            # Persistence is best-effort durability, not the user-facing
            # contract - the in-memory job result below already satisfies
            # the polling client regardless of whether this succeeded.
            logger.exception("Failed to persist video processing results for job %s", job_id)
            video_id = None

        job_store.complete(
            job_id,
            {
                "tracks": tracks,
                "track_count": len(tracks),
                "video_id": str(video_id) if video_id else None,
            },
        )
    except Exception as exc:
        logger.exception("Video processing job %s failed", job_id)
        job_store.fail(job_id, str(exc))
    finally:
        os.unlink(tmp_path)


def _job_counts(job_store: VideoJobStore, job_id: str) -> tuple[int, int, int]:
    job = job_store.get(job_id)
    if job is None:
        return 0, 0, 0
    return job.frame_count, job.expected_frames, job.total_detections


@router.post("/process-video", response_model=VideoJobCreated)
async def process_video(
    file: UploadFile = File(...),
    interval_seconds: float = Query(0.5, gt=0, description="Seconds between sampled frames"),
    embed_interval_seconds: float = Query(1.0, gt=0, description="Seconds between full (embedding) frames"),
    min_quality: float = Query(0.0, ge=0, description="Drop embedding-frame faces below this MagFace score"),
    min_blur: float = Query(50.0, ge=0, description="Drop faces below this Laplacian blur score"),
    full_detection_every_frame: bool = Query(
        False, description="Run the full pipeline on every sampled frame instead of just every embed_interval_seconds"
    ),
    iou_threshold: float = Query(0.15, ge=0, le=1, description="IoU cutoff for assigning lightweight detections to a cluster"),
    cluster_eps: float = Query(0.45, gt=0, description="DBSCAN max cosine distance within a cluster"),
    cluster_min_samples: int = Query(2, ge=1, description="DBSCAN minimum samples to form a cluster"),
    include_crops: bool = Query(True, description="Include a base64 JPEG crop of each track's best face"),
    service: VideoProcessingService = Depends(get_video_processing_service),
    job_store: VideoJobStore = Depends(get_video_job_store),
) -> VideoJobCreated:
    if not file.content_type or not file.content_type.startswith("video/"):
        raise HTTPException(status_code=400, detail="File must be a video")

    suffix = os.path.splitext(file.filename or "")[1] or ".mp4"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp_path = tmp.name
        content = await file.read()
        tmp.write(content)

    expected_frames = count_expected_frames(tmp_path, interval_seconds)
    if expected_frames <= 0:
        os.unlink(tmp_path)
        raise HTTPException(status_code=400, detail="Could not read this file as a video")

    job = job_store.create(expected_frames=expected_frames)

    thread = threading.Thread(
        target=_run_job,
        kwargs=dict(
            job_id=job.job_id,
            job_store=job_store,
            service=service,
            tmp_path=tmp_path,
            original_filename=file.filename,
            content_type=file.content_type,
            interval_seconds=interval_seconds,
            embed_interval_seconds=embed_interval_seconds,
            min_quality=min_quality,
            min_blur=min_blur,
            full_detection_every_frame=full_detection_every_frame,
            iou_threshold=iou_threshold,
            cluster_eps=cluster_eps,
            cluster_min_samples=cluster_min_samples,
            include_crops=include_crops,
        ),
        daemon=True,
    )
    thread.start()

    return VideoJobCreated(job_id=job.job_id)


@router.get("/process-video/{job_id}", response_model=VideoJobStatus)
async def get_video_processing_status(
    job_id: str,
    job_store: VideoJobStore = Depends(get_video_job_store),
) -> VideoJobStatus:
    job = job_store.get(job_id)
    if job is None:
        raise HTTPException(status_code=404, detail="Job not found")

    elapsed_seconds = (job.finished_at or time.monotonic()) - job.started_at

    percent = 0.0
    if job.expected_frames > 0:
        percent = min(100.0, job.frame_count / job.expected_frames * 100)

    eta_seconds = None
    if job.status == "processing" and job.frame_count > 0 and job.expected_frames > job.frame_count:
        rate = job.frame_count / elapsed_seconds if elapsed_seconds > 0 else 0
        if rate > 0:
            eta_seconds = (job.expected_frames - job.frame_count) / rate

    return VideoJobStatus(
        job_id=job.job_id,
        status=job.status,
        frame_count=job.frame_count,
        expected_frames=job.expected_frames,
        percent=percent,
        elapsed_seconds=elapsed_seconds,
        eta_seconds=eta_seconds,
        error=job.error,
        result=VideoProcessingResponse(**job.result) if job.result else None,
    )
