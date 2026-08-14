# TrackID Backend

FastAPI backend for facial recognition and tracking system.

## Project Structure

```
backend/
├── app/
│   ├── api/          # Route handlers
│   ├── core/         # Config, database setup
│   ├── models/       # SQLAlchemy ORM models
│   ├── schemas/      # Pydantic request/response schemas
│   ├── services/     # Business logic layer
│   └── dependencies/ # FastAPI dependency injection
├── logs/             # Application logs
├── models/           # ML model weights (gitignored, see below)
├── scripts/          # One-off scripts (model conversion, etc.)
├── main.py           # FastAPI app entry point
├── requirements.txt  # Python dependencies
└── .env.example      # Environment variables template
```

## Setup

1. Create virtual environment:
   ```bash
   python3.11 -m venv venv
   source venv/bin/activate
   ```

2. Install dependencies:
   ```bash
   pip install -r requirements.txt
   ```

3. Create `.env` from `.env.example`:
   ```bash
   cp .env.example .env
   ```

4. Run dev server:
   ```bash
   python main.py
   ```

   API available at `http://localhost:8000`
   OpenAPI docs at `http://localhost:8000/docs`

## Endpoints

- `POST /api/v1/detect-faces` — detect faces in a single image, returning
  embeddings, bbox, age/gender, blur/quality scores.
- `POST /api/v1/process-video` — sample frames from an uploaded video,
  detect faces, and cluster them via DBSCAN into per-person tracks. The
  expensive ArcFace embedding + MagFace quality pass only runs every
  `embed_interval_seconds`; frames in between get a cheaper detection-only
  pass (see `VideoProcessingService.process_video_for_best_faces` in
  `app/services/video_processing_service.py`), and are assigned to a
  track after the fact by bounding-box IoU. Returns one `best_face` (by
  quality score, with an optional base64 JPEG crop) plus `all_faces` per
  track.

## Face quality scoring (MagFace)

Face detection (`/api/v1/detect-faces`) can return a `quality_score` per
face (embedding magnitude from MagFace — higher is better; roughly
<20 very poor, 20-25 poor, 25-30 acceptable, >30 good). This is optional:
if the model file below isn't present, `quality_score` is simply omitted
from results and everything else works normally.

The model (`models/magface_iresnet50.onnx`, ~170MB) is not committed to
git — it's a binary weights file. To set it up:

1. Get `magface_iresnet50_MS1MV2_ddp_fp32.pth` (~375MB), either by
   copying it from another environment that already has it, or by
   downloading it from the official [MagFace model zoo](https://github.com/IrvingMeng/MagFace).
2. Place it at `backend/models/magface_iresnet50_MS1MV2_ddp_fp32.pth`.
3. Convert it to ONNX (one-time, needs `torch` — not a runtime dependency,
   safe to uninstall afterwards):
   ```bash
   pip install torch --index-url https://download.pytorch.org/whl/cpu
   python scripts/convert_magface_to_onnx.py
   pip uninstall torch
   ```
4. Start (or restart) the server — `models/magface_iresnet50.onnx` is
   picked up automatically at startup.

To point at a model file kept outside `backend/models/`, set
`FACE_QUALITY_MODEL_PATH` in `.env`. GPU inference for the quality model
is a separate toggle from face detection: `FACE_QUALITY_USE_GPU`.

## GPU acceleration

Both face detection (InsightFace/buffalo_l) and quality scoring (MagFace)
can run on an NVIDIA GPU via onnxruntime's CUDA execution provider.
Requires a GPU + driver supporting CUDA 12.x — check with `nvidia-smi`.
No system-wide CUDA toolkit install needed; the required CUDA/cuDNN
runtime libraries come from pip.

1. Install GPU deps on top of the base requirements (the two conflict on
   disk since they share the `onnxruntime` package name, so remove the
   CPU one first):
   ```bash
   pip uninstall -y onnxruntime
   pip install -r requirements-gpu.txt
   ```
2. Enable it:
   ```bash
   export FACE_USE_GPU=true
   export FACE_QUALITY_USE_GPU=true
   ```
   (Note: `app/core/config.py` reads these from the real process
   environment, not from `.env` — for Docker, set them in the
   `environment:` block in `docker-compose.yml` instead.)
3. Run the server as usual. `app/core/gpu_env.py` locates the pip-installed
   CUDA/cuDNN `.so` files and puts them on `LD_LIBRARY_PATH` automatically
   — no manual path wrangling needed. (It does this by re-executing the
   process once at startup, since glibc's dynamic linker only reads
   `LD_LIBRARY_PATH` once at process start; you may see the server "start
   twice" in logs, that's expected.)
4. To confirm it's using the GPU, check the startup logs for
   `Applied providers: ['CUDAExecutionProvider', 'CPUExecutionProvider']`
   — if it silently falls back to `['CPUExecutionProvider']` only, the
   CUDA libs weren't found (check `nvidia-smi` and step 1).

### Docker

Build with `--build-arg GPU=1` (installs `requirements-gpu.txt` instead
of the CPU-only onnxruntime), uncomment the `deploy.resources.reservations`
GPU block and the `FACE_*_USE_GPU` env vars in `docker-compose.yml`, and
make sure the host has the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
installed so Docker can see the GPU.

## Stack

- **FastAPI** - Modern async web framework
- **SQLAlchemy 2.0** - Async ORM
- **PostgreSQL 16 + pgvector** - Vector similarity search
- **Taskiq** - Async task queue
- **InsightFace + OpenCV** - CV/face detection
- **MagFace (ONNX Runtime)** - optional face quality scoring
