"""
Runs detection+embedding once, then sweeps DBSCAN eps values to find a
threshold that isn't just re-using ArcFace's tuned value on a different
embedding space.
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

import cv2
import numpy as np
from insightface.app import FaceAnalysis
from insightface.model_zoo import get_model
from insightface.utils import face_align
from sklearn.cluster import DBSCAN

from app.services.face_detection_service import calculate_blur_score

AURAFACE_RECOGNITION_MODEL = str(Path.home() / ".insightface" / "models" / "auraface" / "glintr100.onnx")


def main(video_path: str):
    print("Loading models...")
    face_app = FaceAnalysis(name="buffalo_l", providers=["CPUExecutionProvider"])
    face_app.prepare(ctx_id=0, det_size=(640, 640), det_thresh=0.5)
    detector = face_app.det_model

    auraface = get_model(AURAFACE_RECOGNITION_MODEL, providers=["CPUExecutionProvider"])
    auraface.prepare(ctx_id=0)

    cap = cv2.VideoCapture(video_path)
    fps = cap.get(cv2.CAP_PROP_FPS) or 30.0
    frame_interval = max(1, int(fps * 0.5))

    embeddings = []
    frame_number = 0
    while True:
        ret, frame = cap.read()
        if not ret:
            break
        if frame_number % frame_interval == 0:
            bboxes, kpss = detector.detect(frame, max_num=0, metric="default")
            for bbox, kps in zip(bboxes, kpss):
                try:
                    aligned = face_align.norm_crop(frame, landmark=kps)
                    emb = auraface.get_feat(aligned).flatten()
                    norm = np.linalg.norm(emb)
                    if norm > 0:
                        emb = emb / norm
                    embeddings.append(emb)
                except Exception:
                    pass
        frame_number += 1
    cap.release()

    X = np.array(embeddings)
    print(f"\n{len(X)} face embeddings collected\n")

    # Pairwise cosine-distance distribution, to see where clusters vs.
    # outliers actually separate for THIS embedding space.
    from sklearn.metrics import pairwise_distances
    dmat = pairwise_distances(X, metric="cosine")
    iu = np.triu_indices_from(dmat, k=1)
    dists = dmat[iu]
    print("Pairwise cosine-distance percentiles (all face pairs, same+different identity mixed):")
    for p in [1, 5, 10, 25, 50]:
        print(f"  p{p:>2}: {np.percentile(dists, p):.3f}")

    print("\neps sweep (min_samples=1):")
    print(f"{'eps':>6} {'tracks':>8}")
    for eps in [0.35, 0.40, 0.45, 0.50, 0.55, 0.60, 0.65, 0.70, 0.75, 0.80]:
        labels = DBSCAN(eps=eps, min_samples=1, metric="cosine").fit_predict(X)
        n_tracks = len(set(labels[labels >= 0]))
        print(f"{eps:>6.2f} {n_tracks:>8}")


if __name__ == "__main__":
    main(sys.argv[1])
