"""
Video frame extraction, face detection, and person clustering.

Extracts frames from a video at a fixed interval, detects faces in each
frame, and clusters the collected face embeddings via DBSCAN to group
detections into unique people across the video.

Framework-agnostic like face_detection_service: takes a file path in,
returns plain dicts/lists out.
"""
import base64
import io
import logging
from typing import Callable, Dict, Generator, List, Optional, Tuple

import cv2
import numpy as np
from sklearn.cluster import DBSCAN

from app.services.face_detection_service import FaceDetectionService

logger = logging.getLogger(__name__)

MIN_CROP_DIMENSION = 224


def _crop_face_from_frame(frame: np.ndarray, bbox: list) -> Optional[np.ndarray]:
    x1, y1, x2, y2 = map(int, bbox)
    height, width = frame.shape[:2]
    face_width = x2 - x1
    face_height = y2 - y1

    padding_x = int(face_width * 1.0)
    padding_y = int(face_height * 1.0)

    x1_padded = max(0, x1 - padding_x)
    y1_padded = max(0, y1 - padding_y)
    x2_padded = min(width, x2 + padding_x)
    y2_padded = min(height, y2 + padding_y)

    if x2_padded <= x1_padded or y2_padded <= y1_padded:
        return None

    face_img = frame[y1_padded:y2_padded, x1_padded:x2_padded]

    crop_height, crop_width = face_img.shape[:2]
    if crop_height < MIN_CROP_DIMENSION or crop_width < MIN_CROP_DIMENSION:
        scale = MIN_CROP_DIMENSION / min(crop_height, crop_width)
        new_width = int(crop_width * scale)
        new_height = int(crop_height * scale)
        face_img = cv2.resize(face_img, (new_width, new_height), interpolation=cv2.INTER_CUBIC)

    return face_img


def extract_face_crop_bytes_from_capture(
    cap: cv2.VideoCapture, frame_number: int, bbox: list
) -> Optional[io.BytesIO]:
    """
    Extract a cropped face image as a BytesIO buffer, reusing an already-open
    VideoCapture. Use this when extracting many crops from the same video to
    avoid re-opening/seeking the whole file for each one.
    """
    cap.set(cv2.CAP_PROP_POS_FRAMES, frame_number)
    ret, frame = cap.read()
    if not ret:
        return None

    face_img = _crop_face_from_frame(frame, bbox)
    if face_img is None:
        return None

    success, buffer = cv2.imencode(".jpg", face_img, [cv2.IMWRITE_JPEG_QUALITY, 95])
    if not success:
        return None

    return io.BytesIO(buffer.tobytes())


def extract_face_crop_bytes(video_path: str, frame_number: int, bbox: list) -> Optional[io.BytesIO]:
    """Extract a cropped face image as a BytesIO buffer, opening the video fresh."""
    cap = cv2.VideoCapture(video_path)
    try:
        return extract_face_crop_bytes_from_capture(cap, frame_number, bbox)
    finally:
        cap.release()


def extract_face_crop_data_uri(video_path: str, frame_number: int, bbox: list) -> Optional[str]:
    """Same as extract_face_crop_bytes, but returns a base64 data: URI string."""
    buf = extract_face_crop_bytes(video_path, frame_number, bbox)
    if buf is None:
        return None
    b64 = base64.b64encode(buf.getvalue()).decode("utf-8")
    return f"data:image/jpeg;base64,{b64}"


def extract_frames_at_interval(
    video_path: str, interval_seconds: float = 1.0
) -> Generator[Tuple[int, float, np.ndarray], None, None]:
    cap = cv2.VideoCapture(video_path)

    if not cap.isOpened():
        raise ValueError(f"Could not open video file: {video_path}")

    try:
        fps = cap.get(cv2.CAP_PROP_FPS)
        if fps <= 0:
            fps = 30.0
            logger.warning("Could not detect FPS, using default: %s", fps)

        frame_interval = max(1, int(fps * interval_seconds))

        frame_number = 0
        frames_extracted = 0

        while True:
            ret, frame = cap.read()
            if not ret:
                break

            if frame_number % frame_interval == 0:
                timestamp = frame_number / fps
                yield (frame_number, timestamp, frame)
                frames_extracted += 1

            frame_number += 1

        logger.info("Extracted %d frames from %d total frames", frames_extracted, frame_number)

    finally:
        cap.release()


def _bbox_iou(bbox1: list, bbox2: list) -> float:
    x1 = max(bbox1[0], bbox2[0])
    y1 = max(bbox1[1], bbox2[1])
    x2 = min(bbox1[2], bbox2[2])
    y2 = min(bbox1[3], bbox2[3])
    intersection = max(0.0, x2 - x1) * max(0.0, y2 - y1)
    area1 = (bbox1[2] - bbox1[0]) * (bbox1[3] - bbox1[1])
    area2 = (bbox2[2] - bbox2[0]) * (bbox2[3] - bbox2[1])
    union = area1 + area2 - intersection
    return intersection / union if union > 0 else 0.0


def cluster_embeddings(
    embeddings: List[List[float]],
    eps: float = 0.45,
    min_samples: int = 2,
) -> List[int]:
    """
    DBSCAN clustering on face embeddings using cosine distance.

    Args:
        embeddings: List of L2-normalised 512-d embedding vectors.
        eps: Maximum cosine distance between two samples for one to be
             considered in the neighbourhood of the other. Lower = more
             conservative (fewer merges). Default 0.45 works well for
             InsightFace buffalo_l.
        min_samples: Minimum samples in a neighbourhood to form a core point.

    Returns:
        List of cluster labels (-1 = outlier / not assigned).
    """
    X = np.array(embeddings)
    clustering = DBSCAN(eps=eps, min_samples=min_samples, metric="cosine")
    return clustering.fit_predict(X).tolist()


def _assign_lightweight_dets(
    lightweight_dets: List[Dict],
    embed_dets: List[Dict],
    iou_threshold: float = 0.15,
) -> None:
    """
    For each lightweight detection (no embedding), find the best IoU match
    among embedding-frame detections and copy its cluster_id. Mutates
    lightweight_dets in-place by adding 'cluster_id'.
    """
    for ldet in lightweight_dets:
        best_cluster = -1
        best_iou = iou_threshold
        for edet in embed_dets:
            if edet.get("cluster_id", -1) < 0:
                continue
            iou = _bbox_iou(ldet["bbox"], edet["bbox"])
            if iou > best_iou:
                best_iou = iou
                best_cluster = edet["cluster_id"]
        ldet["cluster_id"] = best_cluster


def count_expected_frames(video_path: str, interval_seconds: float) -> int:
    cap = cv2.VideoCapture(video_path)
    try:
        fps = cap.get(cv2.CAP_PROP_FPS)
        if fps <= 0:
            fps = 30.0
        total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
        if total_frames <= 0:
            return 0
        frame_interval = max(1, int(fps * interval_seconds))
        return total_frames // frame_interval + 1
    finally:
        cap.release()


def _make_det(face: dict, frame_number: int, timestamp: float, is_embedding: bool) -> dict:
    det = {
        "frame_number": frame_number,
        "timestamp_seconds": timestamp,
        "confidence": float(face.get("confidence", 0.0) or 0.0),
        "quality_score": float(face["quality_score"]) if face.get("quality_score") is not None else None,
        "blur_score": float(face["blur_score"]) if face.get("blur_score") is not None else None,
        "bbox": [float(x) for x in face["bbox"]] if face.get("bbox") else None,
        "is_embedding": is_embedding,
    }
    if is_embedding:
        det["embedding"] = face.get("embedding")
        det["estimated_age"] = face.get("estimated_age")
        det["estimated_gender"] = face.get("estimated_gender")
    return det


class VideoProcessingService:
    """
    Wraps a FaceDetectionService to run it over a video's frames and cluster
    the results into per-person tracks.
    """

    def __init__(self, face_service: FaceDetectionService):
        self.face_service = face_service

    def process_video_for_best_faces(
        self,
        video_path: str,
        interval_seconds: float = 0.1,
        min_quality: float = 0.0,
        min_blur: float = 50.0,
        embed_interval_seconds: float = 1.0,
        full_detection_every_frame: bool = False,
        iou_threshold: float = 0.15,
        cluster_eps: float = 0.45,
        cluster_min_samples: int = 2,
        progress_callback: Optional[Callable[[int, int, int, int], None]] = None,
        on_frame_batch: Optional[Callable[[List[Dict]], None]] = None,
        batch_size: int = 200,
    ) -> List[Dict]:
        """
        Process a video, detect faces, and group them into people via DBSCAN.

        Detection (SCRFD) runs on every sampled frame, but the expensive
        ArcFace embedding + MagFace quality runs only every
        ``embed_interval_seconds`` - unless ``full_detection_every_frame``
        is set, in which case every sampled frame gets the full pipeline
        (much slower, but avoids the IoU-matching step below entirely,
        which is the main source of dropped detections on noisy/low-quality
        footage).

        After all frames are processed, DBSCAN clusters the collected
        embeddings. Lightweight-frame detections (if any) are assigned to
        the nearest embedding-frame detection by IoU, using
        ``iou_threshold``.

        If ``on_frame_batch`` is provided, accumulated lightweight
        detections are flushed to the callback every ``batch_size`` frames
        so the caller can persist them without holding everything in
        memory. Embedding detections are NOT flushed - they must stay in
        memory for DBSCAN clustering.

        Returns:
            [
                {
                    "track_id": int,
                    "best_face": { ... quality-picked detection ... },
                    "all_faces": [ ... every detection for this person ... ],
                },
            ]
        """
        embed_dets: List[Dict] = []
        lightweight_dets: List[Dict] = []
        frame_count = 0
        total_detections = 0
        embed_counter = 0
        batch_accumulator: List[Dict] = []

        expected_frames = count_expected_frames(video_path, interval_seconds)

        logger.info("Processing video: %s", video_path)
        logger.info("Frame interval: %ss, Embed interval: %ss", interval_seconds, embed_interval_seconds)
        logger.info("Min quality: %s, Min blur: %s", min_quality, min_blur)
        logger.info("Expected frames: %s, Cluster eps: %s", expected_frames, cluster_eps)
        logger.info(
            "Full detection every frame: %s, IoU threshold: %s",
            full_detection_every_frame, iou_threshold,
        )

        def _flush_batch():
            nonlocal batch_accumulator
            if on_frame_batch and batch_accumulator:
                on_frame_batch(batch_accumulator)
                batch_accumulator.clear()

        try:
            for frame_number, timestamp, image in extract_frames_at_interval(video_path, interval_seconds):
                frame_count += 1
                embed_counter += 1
                is_embedding_frame = full_detection_every_frame or (
                    embed_counter % max(1, round(embed_interval_seconds / interval_seconds)) == 1
                )

                try:
                    if is_embedding_frame:
                        faces = self.face_service.detect_faces(image)
                    else:
                        faces = self.face_service.detect_faces_lightweight(image)
                except Exception:
                    logger.exception(
                        "Face detection failed on frame %d (%.2fs) - skipping frame",
                        frame_number, timestamp,
                    )
                    faces = []

                total_detections += len(faces)

                if progress_callback:
                    progress_callback(frame_count, expected_frames, 0, total_detections)

                if not faces:
                    continue

                logger.debug(
                    "Frame %d (%.2fs): %d face(s) [%s]",
                    frame_number, timestamp, len(faces),
                    "embedding" if is_embedding_frame else "lightweight",
                )

                for face in faces:
                    blur = face.get("blur_score", 0.0)
                    if blur is not None and blur < min_blur:
                        continue

                    if is_embedding_frame:
                        quality = face.get("quality_score", 0.0) or 0.0
                        if quality < min_quality:
                            continue

                    det = _make_det(face, frame_number, timestamp, is_embedding_frame)

                    if is_embedding_frame:
                        embed_dets.append(det)
                    else:
                        lightweight_dets.append(det)
                        batch_accumulator.append(det)

                        if len(batch_accumulator) >= batch_size:
                            _flush_batch()

            _flush_batch()

            logger.info(
                "Frame loop done: %d frames, %d embed dets, %d lightweight dets",
                frame_count, len(embed_dets), len(lightweight_dets),
            )

            if not embed_dets:
                logger.warning("No embedding detections found - returning empty results")
                return []

            embeddings = [d["embedding"] for d in embed_dets]
            labels = cluster_embeddings(embeddings, eps=cluster_eps, min_samples=cluster_min_samples)

            for i, det in enumerate(embed_dets):
                det["cluster_id"] = labels[i]

            if lightweight_dets:
                _assign_lightweight_dets(lightweight_dets, embed_dets, iou_threshold=iou_threshold)

            all_dets = embed_dets + lightweight_dets
            unique_clusters = sorted(set(l for l in labels if l >= 0))

            tracks = []
            for cluster_id in unique_clusters:
                cluster_faces = [d for d in all_dets if d.get("cluster_id") == cluster_id]
                if not cluster_faces:
                    continue

                embed_in_cluster = [
                    d for d in cluster_faces
                    if d.get("is_embedding") and d.get("quality_score") is not None
                ]
                if embed_in_cluster:
                    best = max(embed_in_cluster, key=lambda d: d["quality_score"])
                else:
                    best = max(cluster_faces, key=lambda d: d.get("blur_score", 0) or 0)

                tracks.append({
                    "track_id": len(tracks),
                    "best_face": best,
                    "all_faces": sorted(
                        cluster_faces,
                        key=lambda d: d.get("quality_score", 0) or 0,
                        reverse=True,
                    ),
                })

            logger.info(
                "Clustering complete: %d people, %d assigned detections",
                len(unique_clusters),
                len([d for d in all_dets if d.get("cluster_id", -1) >= 0]),
            )

            return tracks

        except Exception:
            logger.exception("Error processing video %s", video_path)
            raise
