"""
Face detection and embedding extraction using InsightFace (ArcFace model).

Framework-agnostic: takes numpy arrays / bytes in, returns plain dicts/lists
out. No DB or HTTP concerns here — those live in their own services.
"""
import logging
from typing import List, Optional

import cv2
import numpy as np
from insightface.app import FaceAnalysis
from insightface.utils import face_align

logger = logging.getLogger(__name__)


def calculate_blur_score(image: np.ndarray) -> float:
    """Variance of Laplacian blur score. Higher = less blurry."""
    if image is None or image.size == 0:
        return 0.0

    gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)
    return float(cv2.Laplacian(gray, cv2.CV_64F).var())


def normalize_estimated_gender(face) -> Optional[str]:
    sex = getattr(face, "sex", None)
    if isinstance(sex, str):
        normalized = sex.strip().upper()
        if normalized in {"M", "F"}:
            return normalized

    gender = getattr(face, "gender", None)
    if gender is None:
        return None

    try:
        numeric_gender = int(gender)
    except (TypeError, ValueError):
        return None

    return "M" if numeric_gender == 1 else "F"


def load_image_from_file(file_path: str) -> np.ndarray:
    """Load an image from disk using OpenCV."""
    img = cv2.imread(file_path)
    if img is None:
        raise ValueError(f"Could not load image: {file_path}")
    return img


def load_image_from_bytes(file_bytes: bytes) -> np.ndarray:
    """Load an image from bytes using OpenCV."""
    nparr = np.frombuffer(file_bytes, np.uint8)
    img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
    if img is None:
        raise ValueError("Could not decode image from bytes")
    return img


def extract_images_from_pdf(pdf_path: str) -> List[np.ndarray]:
    """
    Extract images from each page of a PDF.
    Requires pdf2image (poppler-utils on Linux).
    """
    try:
        from pdf2image import convert_from_path
    except ImportError:
        raise ImportError(
            "pdf2image is required for PDF support. Install with: pip install pdf2image"
        )

    pil_images = convert_from_path(pdf_path, dpi=200)

    cv_images = []
    for pil_img in pil_images:
        np_img = np.array(pil_img)
        cv_img = cv2.cvtColor(np_img, cv2.COLOR_RGB2BGR)
        cv_images.append(cv_img)

    return cv_images


def compare_embeddings(embedding1: List[float], embedding2: List[float]) -> float:
    """
    Cosine similarity between two face embeddings.
    Returns a score between 0 and 1 (higher = more similar). Typical
    threshold for "same person" is > 0.4.
    """
    vec1 = np.array(embedding1)
    vec2 = np.array(embedding2)
    return float(np.dot(vec1, vec2) / (np.linalg.norm(vec1) * np.linalg.norm(vec2)))


class FaceDetectionService:
    """
    Wraps a loaded FaceAnalysis instance. The instance is injected rather
    than looked up globally, so this service is easy to test with a fake/mock
    model.
    """

    def __init__(self, face_app: FaceAnalysis):
        self.face_app = face_app

    def detect_faces(self, image: np.ndarray) -> List[dict]:
        """
        Detect faces in an image and return their embeddings and locations.

        Returns a list of dicts:
        [
            {
                "embedding": [float, ...],  # 512-d normalized vector
                "bbox": [x1, y1, x2, y2],
                "confidence": float,
                "estimated_age": int,        # optional
                "estimated_gender": str,     # optional, M/F
                "landmarks": [[x, y], ...],  # optional, 5 facial landmarks
                "blur_score": float,         # optional
            },
            ...
        ]

        Note: quality_score (MagFace) is intentionally omitted here — it
        will be added back once the quality estimation service is ported.
        """
        faces = self.face_app.get(image)

        results = []
        for face in faces:
            result = {
                "embedding": face.normed_embedding.tolist(),
                "bbox": face.bbox.tolist(),
                "confidence": float(face.det_score),
            }

            if hasattr(face, "age") and face.age is not None:
                result["estimated_age"] = int(face.age)

            estimated_gender = normalize_estimated_gender(face)
            if estimated_gender:
                result["estimated_gender"] = estimated_gender

            if hasattr(face, "kps") and face.kps is not None:
                result["landmarks"] = face.kps.tolist()
                try:
                    aligned_face = face_align.norm_crop(image, landmark=face.kps)
                    result["blur_score"] = calculate_blur_score(aligned_face)
                except Exception:
                    logger.exception("Failed to compute blur score for detected face")

            results.append(result)

        return results

    def detect_faces_lightweight(self, image: np.ndarray) -> List[dict]:
        """
        Detection-only – runs just the SCRFD detector. ~3-5x faster than
        detect_faces() since it skips ArcFace embedding, age/gender, and
        blur-on-aligned-crop.

        Returns a list of dicts:
        [
            {
                "bbox": [x1, y1, x2, y2],
                "confidence": float,
                "kps": [[x, y], ...],
                "blur_score": float | None,
            },
        ]
        """
        bboxes, kpss = self.face_app.det_model.detect(image, max_num=0, metric="default")

        results = []
        for bbox, kps in zip(bboxes, kpss):
            result = {
                "bbox": bbox[:4].tolist(),
                "confidence": float(bbox[4]),
                "kps": kps.tolist(),
            }
            try:
                aligned = face_align.norm_crop(image, landmark=kps)
                result["blur_score"] = calculate_blur_score(aligned)
            except Exception:
                result["blur_score"] = None
            results.append(result)

        return results

    def extract_face_embedding(self, image: np.ndarray, kps: np.ndarray | list) -> List[float]:
        """
        Align a face crop using the 5 landmarks and extract its ArcFace
        512-d embedding. Used after detect_faces_lightweight() when you
        only need the embedding for a specific face.
        """
        aligned = face_align.norm_crop(image, landmark=kps)
        embedding = self.face_app.models["recognition"].get_feat(aligned).flatten()
        norm = np.linalg.norm(embedding)
        if norm > 0:
            embedding = embedding / norm
        return embedding.tolist()
