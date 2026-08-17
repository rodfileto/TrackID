"""
Read access to persisted video processing results (see
video_persistence_service.py for the write path).
"""
import uuid

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.core.database import get_db
from app.core.storage import MediaStorage, get_media_storage
from app.models import FaceDetection, Video
from app.schemas.video_processing import (
    VideoFaceDetection,
    VideoListResponse,
    VideoProcessingResponse,
    VideoSummary,
    VideoTrack,
)

router = APIRouter(prefix="/api/v1", tags=["videos"])


@router.get("/videos", response_model=VideoListResponse)
async def list_videos(
    page: int = Query(1, ge=1),
    page_size: int = Query(10, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
) -> VideoListResponse:
    total = (await db.execute(select(func.count()).select_from(Video))).scalar_one()

    result = await db.execute(
        select(Video)
        .order_by(Video.created_at.desc())
        .offset((page - 1) * page_size)
        .limit(page_size)
    )
    videos = result.scalars().all()

    return VideoListResponse(
        items=[
            VideoSummary(
                video_id=str(v.id),
                original_filename=v.original_filename,
                created_at=v.created_at,
                track_count=v.track_count,
                frame_count=v.frame_count,
                total_detections=v.total_detections,
            )
            for v in videos
        ],
        total=total,
        page=page,
        page_size=page_size,
    )


def _to_schema(det: FaceDetection, track_local_id: int, storage: MediaStorage) -> VideoFaceDetection:
    return VideoFaceDetection(
        frame_number=det.frame_number,
        timestamp_seconds=det.timestamp_seconds,
        confidence=det.confidence,
        quality_score=det.quality_score,
        blur_score=det.blur_score,
        bbox=list(det.bbox) if det.bbox is not None else None,
        is_embedding=det.is_embedding,
        estimated_age=det.estimated_age,
        estimated_gender=det.estimated_gender,
        cluster_id=track_local_id,
        face_crop=storage.generate_presigned_url(det.crop_storage_key)
        if det.crop_storage_key
        else None,
    )


@router.get("/videos/{video_id}", response_model=VideoProcessingResponse)
async def get_video(
    video_id: uuid.UUID,
    db: AsyncSession = Depends(get_db),
) -> VideoProcessingResponse:
    result = await db.execute(
        select(Video)
        .options(
            selectinload(Video.person_tracks),
            selectinload(Video.face_detections),
        )
        .where(Video.id == video_id)
    )
    video = result.scalar_one_or_none()
    if video is None:
        raise HTTPException(status_code=404, detail="Video not found")

    storage = get_media_storage()

    detections_by_track: dict[uuid.UUID, list[FaceDetection]] = {}
    for det in video.face_detections:
        if det.person_track_id is not None:
            detections_by_track.setdefault(det.person_track_id, []).append(det)

    tracks: list[VideoTrack] = []
    for pt in sorted(video.person_tracks, key=lambda t: t.track_local_id):
        dets = sorted(
            detections_by_track.get(pt.id, []),
            key=lambda d: d.quality_score or 0,
            reverse=True,
        )
        if not dets:
            continue

        best_det = next((d for d in dets if d.id == pt.best_face_detection_id), dets[0])

        tracks.append(
            VideoTrack(
                track_id=pt.track_local_id,
                best_face=_to_schema(best_det, pt.track_local_id, storage),
                all_faces=[_to_schema(d, pt.track_local_id, storage) for d in dets],
            )
        )

    return VideoProcessingResponse(
        tracks=tracks,
        track_count=len(tracks),
        video_id=str(video.id),
    )
