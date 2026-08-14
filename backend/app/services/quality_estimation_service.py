"""
Face quality estimation using MagFace.

MagFace is a face recognition model that learns embeddings where the
magnitude (Euclidean norm) of the feature vector correlates with face
quality. High-quality faces (clear, well-lit, frontal) produce large
magnitudes, while low-quality faces (blurry, occluded, extreme pose)
produce small magnitudes.

This runs alongside the buffalo_l ArcFace model without replacing it or
affecting its embeddings - it's a separate ONNX model used only to score
quality.

Typical score ranges (tune to your data):
    < 20:   very poor quality
    20-25:  poor quality
    25-30:  acceptable quality
    > 30:   good quality
"""
import logging
import os

import numpy as np
import onnxruntime
from insightface.utils import face_align

logger = logging.getLogger(__name__)


class QualityEstimationService:
    """Wraps a loaded MagFace ONNX session."""

    def __init__(self, model_path: str, use_gpu: bool = False):
        if not os.path.exists(model_path):
            raise FileNotFoundError(f"MagFace model not found at {model_path}")

        providers = (
            ["CUDAExecutionProvider", "CPUExecutionProvider"]
            if use_gpu
            else ["CPUExecutionProvider"]
        )

        self.session = onnxruntime.InferenceSession(model_path, providers=providers)
        self.input_name = self.session.get_inputs()[0].name

    def _preprocess_aligned(self, aligned_bgr: np.ndarray) -> np.ndarray:
        """
        Normalize an already-aligned 112x112 face crop for the MagFace model.

        MagFace's reference training/inference code (dataloader.py,
        inference/gen_feat.py in github.com/IrvingMeng/MagFace) reads with
        cv2.imread (BGR) and only applies torchvision's ToTensor (scale
        [0,255] -> [0,1]) - no BGR->RGB conversion, no mean/std shift. Keep
        BGR order and matching [0,1] scaling here, unlike the ArcFace-style
        (x-127.5)/128 RGB normalization used for the buffalo_l embedding
        model.
        """
        aligned_t = np.transpose(aligned_bgr, (2, 0, 1))
        input_blob = np.expand_dims(aligned_t, axis=0).astype(np.float32)
        return input_blob / 255.0

    def get_quality_from_aligned(self, aligned_bgr: np.ndarray) -> float:
        """
        Quality score from a face crop that's already been aligned (e.g.
        reusing the alignment done for blur-score calculation).

        Returns the embedding magnitude - higher is better quality.
        """
        try:
            input_tensor = self._preprocess_aligned(aligned_bgr)
            embeddings = self.session.run(None, {self.input_name: input_tensor})[0]
            return float(np.linalg.norm(embeddings))
        except Exception:
            logger.exception("Failed to compute quality score")
            return 0.0

    def get_quality(self, image: np.ndarray, kps: np.ndarray | list) -> float:
        """
        Align a face using its 5-point landmarks and compute its quality
        score. Returns 0.0 if alignment/inference fails.
        """
        try:
            aligned = face_align.norm_crop(image, landmark=kps)
        except Exception:
            logger.exception("Failed to align face for quality estimation")
            return 0.0

        return self.get_quality_from_aligned(aligned)
