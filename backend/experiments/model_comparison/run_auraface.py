"""
Same methodology as run_adaface.py: same SCRFD detector as the InsightFace
run (buffalo_l), only the embedding model swapped - this time for AuraFace's
glintr100.onnx (an Apache-2.0-licensed, InsightFace-format drop-in ArcFace
replacement, no use-based license restrictions).

Run from backend/ with the project venv active:
    python experiments/model_comparison/run_auraface.py <video_path> <output_json>
"""
import base64
import json
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

import cv2  # noqa: E402
import numpy as np  # noqa: E402
from insightface.app import FaceAnalysis  # noqa: E402
from insightface.model_zoo import get_model  # noqa: E402
from insightface.utils import face_align  # noqa: E402
from sklearn.cluster import DBSCAN  # noqa: E402

from app.services.face_detection_service import calculate_blur_score  # noqa: E402
from app.services.video_processing_service import MIN_CROP_DIMENSION, _crop_face_from_frame  # noqa: E402

AURAFACE_RECOGNITION_MODEL = str(
    Path.home() / ".insightface" / "models" / "auraface" / "glintr100.onnx"
)


def crop_to_data_uri(frame: np.ndarray, bbox: list) -> "str | None":
    face_img = _crop_face_from_frame(frame, bbox)
    if face_img is None:
        return None
    ok, buf = cv2.imencode(".jpg", face_img, [cv2.IMWRITE_JPEG_QUALITY, 95])
    if not ok:
        return None
    return f"data:image/jpeg;base64,{base64.b64encode(buf.tobytes()).decode('utf-8')}"


def main(video_path: str, output_path: str) -> None:
    print("Loading SCRFD detector (shared with InsightFace/AdaFace runs, CPU)...")
    face_app = FaceAnalysis(name="buffalo_l", providers=["CPUExecutionProvider"])
    face_app.prepare(ctx_id=0, det_size=(640, 640), det_thresh=0.5)
    detector = face_app.det_model

    print("Loading AuraFace recognition model (glintr100.onnx)...")
    auraface = get_model(AURAFACE_RECOGNITION_MODEL, providers=["CPUExecutionProvider"])
    auraface.prepare(ctx_id=0)

    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        raise ValueError(f"Could not open video: {video_path}")

    fps = cap.get(cv2.CAP_PROP_FPS) or 30.0
    interval_seconds = 0.5
    frame_interval = max(1, int(fps * interval_seconds))

    dets = []
    frame_number = 0
    start = time.time()

    while True:
        ret, frame = cap.read()
        if not ret:
            break

        if frame_number % frame_interval == 0:
            timestamp = frame_number / fps
            bboxes, kpss = detector.detect(frame, max_num=0, metric="default")

            for bbox, kps in zip(bboxes, kpss):
                try:
                    aligned = face_align.norm_crop(frame, landmark=kps)
                    blur = calculate_blur_score(aligned)
                    if blur < 30.0:
                        continue
                    embedding = auraface.get_feat(aligned).flatten()
                    norm = np.linalg.norm(embedding)
                    if norm > 0:
                        embedding = embedding / norm

                    dets.append(
                        {
                            "frame_number": frame_number,
                            "timestamp_seconds": timestamp,
                            "confidence": float(bbox[4]),
                            "quality_score": None,
                            "blur_score": blur,
                            "bbox": bbox[:4].tolist(),
                            "is_embedding": True,
                            "estimated_age": None,
                            "estimated_gender": None,
                            "embedding": embedding.tolist(),
                        }
                    )
                except Exception as exc:  # noqa: BLE001
                    print(f"  frame {frame_number}: skipped face ({exc})")

        frame_number += 1

    cap.release()
    elapsed = time.time() - start
    print(f"Detection+embedding done in {elapsed:.1f}s - {len(dets)} face detection(s)")

    if not dets:
        result = {"engine": "auraface-glintr100", "elapsed_seconds": elapsed, "tracks": [], "track_count": 0, "video_id": None}
        Path(output_path).write_text(json.dumps(result, indent=2))
        return

    embeddings = [d["embedding"] for d in dets]
    labels = DBSCAN(eps=0.55, min_samples=2, metric="cosine").fit_predict(np.array(embeddings)).tolist()
    for i, d in enumerate(dets):
        d["cluster_id"] = labels[i]

    cap = cv2.VideoCapture(video_path)
    unique_clusters = sorted(set(l for l in labels if l >= 0))
    tracks = []
    for cluster_id in unique_clusters:
        cluster_faces = [d for d in dets if d["cluster_id"] == cluster_id]
        best = max(cluster_faces, key=lambda d: d["blur_score"] or 0)
        for face in cluster_faces:
            cap.set(cv2.CAP_PROP_POS_FRAMES, face["frame_number"])
            ret, frame = cap.read()
            face["face_crop"] = crop_to_data_uri(frame, face["bbox"]) if ret else None
            face.pop("embedding", None)
        tracks.append(
            {
                "track_id": len(tracks),
                "best_face": best,
                "all_faces": sorted(cluster_faces, key=lambda d: d.get("blur_score") or 0, reverse=True),
            }
        )
    cap.release()

    print(f"Clustering complete: {len(tracks)} track(s)")

    result = {
        "engine": "auraface-glintr100",
        "elapsed_seconds": elapsed,
        "tracks": tracks,
        "track_count": len(tracks),
        "video_id": None,
    }
    Path(output_path).write_text(json.dumps(result, indent=2))
    print(f"Wrote {output_path} ({Path(output_path).stat().st_size / 1024:.0f} KB)")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
