"""
Runs the same video through the same SCRFD face detector InsightFace uses
(so detection/localization is held constant), but swaps the embedding step
for AdaFace (IR-101, WebFace4M) instead of ArcFace - isolating the actual
variable being compared: embedding/clustering quality, not detector
differences.

Outputs a VideoProcessingResponse-shaped JSON identical in structure to
run_insightface.py's, so the frontend can render both the same way.

Run from backend/experiments/model_comparison/ with venv_adaface active:
    python run_adaface.py <video_path> <output_json>
"""
import base64
import io
import json
import os
import sys
import time
from pathlib import Path

import cv2
import numpy as np
import torch
import yaml
from insightface.app import FaceAnalysis
from insightface.utils import face_align
from omegaconf import OmegaConf
from sklearn.cluster import DBSCAN

ADAFACE_REPO = (
    "/home/rodrigo/.cache/huggingface/hub/"
    "models--minchul--cvlface_adaface_ir101_webface4m/snapshots/"
    "f2b38d9e24bfe301490d8dd081d8924b102333dd"
)

MIN_CROP_DIMENSION = 224


def load_adaface_model():
    cwd = os.getcwd()
    sys.path.insert(0, ADAFACE_REPO)
    os.chdir(ADAFACE_REPO)
    try:
        from models import get_model  # noqa: E402

        cfg = dict(yaml.safe_load(open("pretrained_model/model.yaml")))
        model_conf = OmegaConf.create(cfg)
        model = get_model(model_conf)
        model.load_state_dict_from_path("pretrained_model/model.pt")
        model.eval()
        return model
    finally:
        os.chdir(cwd)


def adaface_preprocess(aligned_bgr_112: np.ndarray) -> torch.Tensor:
    """AdaFace expects BGR, normalized to [-1, 1], CHW, batch dim."""
    img = aligned_bgr_112.astype(np.float32)
    img = (img - 127.5) / 127.5
    img = np.transpose(img, (2, 0, 1))
    return torch.from_numpy(img).unsqueeze(0).float()


def calculate_blur_score(image: np.ndarray) -> float:
    if image is None or image.size == 0:
        return 0.0
    gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)
    return float(cv2.Laplacian(gray, cv2.CV_64F).var())


def crop_face_from_frame(frame: np.ndarray, bbox: list) -> "np.ndarray | None":
    x1, y1, x2, y2 = map(int, bbox)
    height, width = frame.shape[:2]
    face_width, face_height = x2 - x1, y2 - y1
    padding_x, padding_y = int(face_width), int(face_height)
    x1p, y1p = max(0, x1 - padding_x), max(0, y1 - padding_y)
    x2p, y2p = min(width, x2 + padding_x), min(height, y2 + padding_y)
    if x2p <= x1p or y2p <= y1p:
        return None
    face_img = frame[y1p:y2p, x1p:x2p]
    h, w = face_img.shape[:2]
    if h < MIN_CROP_DIMENSION or w < MIN_CROP_DIMENSION:
        scale = MIN_CROP_DIMENSION / min(h, w)
        face_img = cv2.resize(
            face_img, (int(w * scale), int(h * scale)), interpolation=cv2.INTER_CUBIC
        )
    return face_img


def crop_to_data_uri(frame: np.ndarray, bbox: list) -> "str | None":
    face_img = crop_face_from_frame(frame, bbox)
    if face_img is None:
        return None
    ok, buf = cv2.imencode(".jpg", face_img, [cv2.IMWRITE_JPEG_QUALITY, 95])
    if not ok:
        return None
    b64 = base64.b64encode(buf.tobytes()).decode("utf-8")
    return f"data:image/jpeg;base64,{b64}"


def main(video_path: str, output_path: str) -> None:
    print("Loading SCRFD detector (shared with InsightFace, CPU)...")
    face_app = FaceAnalysis(name="buffalo_l", providers=["CPUExecutionProvider"])
    face_app.prepare(ctx_id=0, det_size=(640, 640), det_thresh=0.5)
    detector = face_app.det_model

    print("Loading AdaFace IR101 (WebFace4M)...")
    adaface = load_adaface_model()

    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        raise ValueError(f"Could not open video: {video_path}")

    fps = cap.get(cv2.CAP_PROP_FPS) or 30.0
    interval_seconds = 0.5
    frame_interval = max(1, int(fps * interval_seconds))

    dets = []  # each: frame_number, timestamp, bbox, confidence, blur_score, embedding
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
                    aligned = face_align.norm_crop(frame, landmark=kps)  # 112x112 BGR
                    blur = calculate_blur_score(aligned)
                    if blur < 30.0:
                        continue

                    with torch.no_grad():
                        embedding = adaface(adaface_preprocess(aligned)).squeeze(0)
                        embedding = torch.nn.functional.normalize(embedding, dim=0)
                        embedding = embedding.numpy().tolist()

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
                            "embedding": embedding,
                        }
                    )
                except Exception as exc:  # noqa: BLE001
                    print(f"  frame {frame_number}: skipped face ({exc})")

        frame_number += 1

    cap.release()
    elapsed = time.time() - start
    print(f"Detection+embedding done in {elapsed:.1f}s - {len(dets)} face detection(s)")

    if not dets:
        result = {"engine": "adaface-ir101-webface4m", "elapsed_seconds": elapsed, "tracks": [], "track_count": 0, "video_id": None}
        Path(output_path).write_text(json.dumps(result, indent=2))
        return

    embeddings = [d["embedding"] for d in dets]
    labels = DBSCAN(eps=0.625, min_samples=2, metric="cosine").fit_predict(np.array(embeddings)).tolist()
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
        "engine": "adaface-ir101-webface4m",
        "elapsed_seconds": elapsed,
        "tracks": tracks,
        "track_count": len(tracks),
        "video_id": None,
    }
    Path(output_path).write_text(json.dumps(result, indent=2))
    print(f"Wrote {output_path} ({Path(output_path).stat().st_size / 1024:.0f} KB)")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
