"""
Writes a completed video-processing run (tracks + detections, as returned
by VideoProcessingService.process_video_for_best_faces and already
enriched with storage keys by media_storage_service) to the database.

This is the tactical/operational layer only - see
app/models/video_processing.py's module docstring.
"""
import asyncio
import logging
import uuid
from typing import Dict, List, Optional

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

from app.core.config import settings
from app.models import FaceDetection, PersonTrack, Video

logger = logging.getLogger(__name__)


async def _persist(
    session: AsyncSession,
    *,
    video_id: uuid.UUID,
    job_id: str,
    original_filename: Optional[str],
    content_type: Optional[str],
    video_storage_key: Optional[str],
    params: Dict,
    expected_frames: int,
    frame_count: int,
    total_detections: int,
    tracks: List[Dict],
) -> None:
    session.add(
        Video(
            id=video_id,
            job_id=job_id,
            original_filename=original_filename,
            content_type=content_type,
            video_storage_key=video_storage_key,
            interval_seconds=params["interval_seconds"],
            embed_interval_seconds=params["embed_interval_seconds"],
            min_quality=params["min_quality"],
            min_blur=params["min_blur"],
            full_detection_every_frame=params["full_detection_every_frame"],
            iou_threshold=params["iou_threshold"],
            cluster_eps=params["cluster_eps"],
            cluster_min_samples=params["cluster_min_samples"],
            expected_frames=expected_frames,
            frame_count=frame_count,
            total_detections=total_detections,
            track_count=len(tracks),
        )
    )

    # Two-phase insert to satisfy the person_tracks <-> face_detections
    # circular FK: insert both sides with best_face_detection_id left NULL,
    # flush, then UPDATE it once the FaceDetection rows' (client-generated)
    # ids are known to be committed-insertable.
    person_tracks: List[PersonTrack] = []
    detections: List[FaceDetection] = []
    best_face_ids: Dict[uuid.UUID, uuid.UUID] = {}  # person_track.id -> best FaceDetection.id

    for track in tracks:
        pt_id = uuid.uuid4()
        best_face = track["best_face"]
        all_faces = track["all_faces"]

        pt = PersonTrack(
            id=pt_id,
            video_id=video_id,
            track_local_id=track["track_id"],
            face_count=len(all_faces),
            representative_embedding=best_face.get("embedding"),
        )
        person_tracks.append(pt)

        for det in all_faces:
            det_id = uuid.uuid4()
            # best_face is the same dict object as one entry of all_faces
            # (see process_video_for_best_faces) - identity comparison is
            # how we recover which FaceDetection row is the track's best.
            if det is best_face:
                best_face_ids[pt_id] = det_id

            detections.append(
                FaceDetection(
                    id=det_id,
                    video_id=video_id,
                    person_track_id=pt_id,
                    frame_number=det["frame_number"],
                    timestamp_seconds=det["timestamp_seconds"],
                    confidence=det["confidence"],
                    quality_score=det.get("quality_score"),
                    blur_score=det.get("blur_score"),
                    bbox=det.get("bbox"),
                    is_embedding=det["is_embedding"],
                    embedding=det.get("embedding"),
                    estimated_age=det.get("estimated_age"),
                    estimated_gender=det.get("estimated_gender"),
                    crop_storage_key=det.get("crop_storage_key"),
                )
            )

    session.add_all(person_tracks)
    session.add_all(detections)
    await session.flush()

    for pt in person_tracks:
        best_id = best_face_ids.get(pt.id)
        if best_id is not None:
            pt.best_face_detection_id = best_id

    await session.commit()


async def _persist_with_fresh_engine(**kwargs) -> None:
    """
    asyncpg connections are bound to the event loop that created them and
    cannot be reused from a different one. The shared app-wide engine's
    pool is bound to the main uvicorn event loop, so reusing it from a
    thread's freshly-created loop (see persist_video_result_sync) fails
    intermittently with "got Future ... attached to a different loop" -
    this creates and disposes its own engine entirely within the calling
    loop instead, never touching the shared pool.
    """
    engine = create_async_engine(settings.DATABASE_URL, pool_pre_ping=True)
    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    try:
        async with session_factory() as session:
            try:
                await _persist(session, **kwargs)
            except Exception:
                await session.rollback()
                raise
    finally:
        await engine.dispose()


def persist_video_result_sync(**kwargs) -> None:
    """
    Sync-thread bridge for callers running off the event loop (the video
    processing job runs in a plain threading.Thread). asyncio.run() spins
    up a fresh event loop for this one call.
    """
    asyncio.run(_persist_with_fresh_engine(**kwargs))
