#!/usr/bin/env bash
# Downloads the face models the image expects at /models, checking each against its SHA-256:
# YuNet (detector, MIT) and AuraFace-v1 (recognizer, Apache-2.0), the defaults. With --scrfd it
# also fetches InsightFace's SCRFD detector, which is for NON-COMMERCIAL RESEARCH ONLY: read
# docs/MODELS.md before using it. The same files and checksums as trackid-vision's
# scripts/fetch-runtime-deps.sh.
#
#   ./fetch-models.sh [--scrfd] [directory]     # default directory: ./models
set -euo pipefail

SCRFD=false
if [[ "${1:-}" == "--scrfd" ]]; then
  SCRFD=true
  shift
fi
DIR="${1:-$(dirname "${BASH_SOURCE[0]}")/models}"
mkdir -p "$DIR"

fetch() {
  local name="$1" url="$2" sha="$3" dest="$DIR/$1"
  if [[ -f "$dest" ]] && echo "$sha  $dest" | sha256sum -c - >/dev/null 2>&1; then
    echo "already present: $name"
    return
  fi
  echo "downloading $name ..."
  curl -fsSL -o "$dest.part" "$url"
  echo "$sha  $dest.part" | sha256sum -c - >/dev/null || { rm -f "$dest.part"; echo "$name: checksum mismatch" >&2; exit 1; }
  mv "$dest.part" "$dest"
}

fetch face_detection_yunet_2026may.onnx \
  https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2026may.onnx \
  ebafce4e3c118d6554634be5c27ab333b4c047a9a8c3faf1d7cf93101c22f0f0
fetch glintr100.onnx \
  https://huggingface.co/fal/AuraFace-v1/resolve/main/glintr100.onnx \
  a7933ea5330113b01c9b60351d8f4c33003f145d8470ac5f0e52ee2effe25c60
if $SCRFD; then
  fetch scrfd_10g_bnkps.onnx \
    https://huggingface.co/fal/AuraFace-v1/resolve/main/scrfd_10g_bnkps.onnx \
    5838f7fe053675b1c7a08b633df49e7af5495cee0493c7dcf6697200b85b5b91
  echo "SCRFD is non-commercial research use only. To use it, set in .env:"
  echo "  VISION_DETECTOR=scrfd"
  echo "  VISION_DETECTOR_PATH=/models/scrfd_10g_bnkps.onnx"
fi
# The container runs as uid 10001: the files must be readable by it.
chmod a+r "$DIR"/*.onnx
echo "done: $DIR"
