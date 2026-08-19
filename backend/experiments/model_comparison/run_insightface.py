"""
Runs the video through the *actual* production InsightFace pipeline
(FaceDetectionService + VideoProcessingService, unmodified) and dumps a
VideoProcessingResponse-shaped JSON, with every crop as a base64 data URI.

Run from backend/ with the project venv active:
    python experiments/model_comparison/run_insightface.py <video_path> <output_json>
"""
import json
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

from insightface.app import FaceAnalysis  # noqa: E402

from app.services.face_detection_service import FaceDetectionService  # noqa: E402
from app.services.video_processing_service import (  # noqa: E402
    VideoProcessingService,
    extract_face_crop_data_uri,
)


def main(video_path: str, output_path: str) -> None:
    print("Loading InsightFace (buffalo_l, CPU)...")
    face_app = FaceAnalysis(name="buffalo_l", providers=["CPUExecutionProvider"])
    face_app.prepare(ctx_id=0, det_size=(640, 640), det_thresh=0.5)

    face_service = FaceDetectionService(face_app=face_app, quality_service=None)
    video_service = VideoProcessingService(face_service=face_service)

    start = time.time()
    tracks = video_service.process_video_for_best_faces(
        video_path,
        interval_seconds=0.5,
        embed_interval_seconds=0.5,
        min_quality=0.0,
        min_blur=30.0,
        full_detection_every_frame=True,  # small local video - keep it simple, no IoU-matching step
        cluster_eps=0.57,
        cluster_min_samples=2,
    )
    elapsed = time.time() - start
    print(f"Done in {elapsed:.1f}s - {len(tracks)} track(s)")

    for track in tracks:
        for face in [track["best_face"], *track["all_faces"]]:
            face["face_crop"] = extract_face_crop_data_uri(
                video_path, face["frame_number"], face["bbox"]
            )
            face.pop("embedding", None)  # not needed client-side, keep the JSON small

    result = {
        "engine": "insightface-buffalo_l",
        "elapsed_seconds": elapsed,
        "tracks": tracks,
        "track_count": len(tracks),
        "video_id": None,
    }

    Path(output_path).write_text(json.dumps(result, indent=2))
    print(f"Wrote {output_path} ({Path(output_path).stat().st_size / 1024:.0f} KB)")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
