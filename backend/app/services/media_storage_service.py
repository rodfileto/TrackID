"""
Uploads a processed video's source file and per-detection face crops to
object storage, ahead of DB persistence (see video_persistence_service.py).

Framework-agnostic like the other services here: takes plain paths/dicts
in, returns plain values out.
"""
import logging
import os
import uuid
from typing import Dict, List, Optional, Tuple

import cv2

from app.core.storage import MediaStorage
from app.services.video_processing_service import extract_face_crop_bytes_from_capture

logger = logging.getLogger(__name__)


def upload_video_and_crops(
    storage: MediaStorage,
    video_id: uuid.UUID,
    video_path: str,
    content_type: Optional[str],
    tracks: List[Dict],
) -> Tuple[str, List[Dict]]:
    """
    Uploads the source video file, then walks every detection across every
    track (each track's all_faces already includes its best_face by object
    identity - see process_video_for_best_faces) and uploads a crop for
    each one that has a usable bbox, using a single VideoCapture pass to
    avoid reopening/seeking the file per crop.

    Mutates each detection dict in place, adding "crop_storage_key".
    Returns (video_storage_key, tracks).
    """
    ext = os.path.splitext(video_path)[1] or ".mp4"
    video_key = f"videos/{video_id}/original{ext}"
    storage.upload_file(video_path, video_key, content_type=content_type)

    cap = cv2.VideoCapture(video_path)
    try:
        for track in tracks:
            for det in track["all_faces"]:
                bbox = det.get("bbox")
                if not bbox:
                    continue

                buf = extract_face_crop_bytes_from_capture(cap, det["frame_number"], bbox)
                if buf is None:
                    continue

                crop_key = f"videos/{video_id}/crops/{uuid.uuid4()}.jpg"
                try:
                    storage.upload_bytes(buf.getvalue(), crop_key, content_type="image/jpeg")
                except Exception:
                    logger.exception(
                        "Failed to upload crop for frame %d - skipping this detection's crop",
                        det["frame_number"],
                    )
                    continue

                det["crop_storage_key"] = crop_key
    finally:
        cap.release()

    return video_key, tracks
