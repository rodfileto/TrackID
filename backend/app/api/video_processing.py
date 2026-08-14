import logging
import os
import tempfile

from fastapi import APIRouter, Depends, File, HTTPException, Query, UploadFile
from starlette.concurrency import run_in_threadpool

from app.dependencies.video_processing import get_video_processing_service
from app.schemas.video_processing import VideoProcessingResponse
from app.services.video_processing_service import (
    VideoProcessingService,
    extract_face_crop_data_uri,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1", tags=["video-processing"])


@router.post("/process-video", response_model=VideoProcessingResponse)
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
) -> VideoProcessingResponse:
    if not file.content_type or not file.content_type.startswith("video/"):
        raise HTTPException(status_code=400, detail="File must be a video")

    suffix = os.path.splitext(file.filename or "")[1] or ".mp4"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp_path = tmp.name
        content = await file.read()
        tmp.write(content)

    try:
        tracks = await run_in_threadpool(
            service.process_video_for_best_faces,
            video_path=tmp_path,
            interval_seconds=interval_seconds,
            min_quality=min_quality,
            min_blur=min_blur,
            embed_interval_seconds=embed_interval_seconds,
            full_detection_every_frame=full_detection_every_frame,
            iou_threshold=iou_threshold,
            cluster_eps=cluster_eps,
            cluster_min_samples=cluster_min_samples,
        )

        if include_crops:
            for track in tracks:
                best = track["best_face"]
                if best.get("bbox") is not None:
                    best["face_crop"] = extract_face_crop_data_uri(
                        tmp_path, best["frame_number"], best["bbox"]
                    )
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    finally:
        os.unlink(tmp_path)

    return VideoProcessingResponse(tracks=tracks, track_count=len(tracks))
