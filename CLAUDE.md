# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

TrackID is a facial recognition and tracking system: a FastAPI backend (face detection/embedding via InsightFace + pgvector similarity search) and a React admin dashboard frontend. The backend is the actively-developed part; the frontend is currently the unmodified [TailAdmin React](https://tailadmin.com) template scaffold (demo pages like Calendar, Charts, Basic Tables) — no TrackID-specific UI has been wired in yet, so don't assume routes like `/videos` or `/images` reflect real product features.

The project is also the vehicle for a 3-paper academic research pipeline (tactical case linking → strategic network intelligence → forensic evidentiary validation). See `docs/RESEARCH_STRATEGY.md`, `docs/PAPER1_TACTICAL_LINKING.md`, `docs/PAPER2_INTELLIGENCE.md`, and `docs/PAPER3_FORENSIC_EVIDENTIARY.md` before making architectural changes that would affect the evaluation story of any of these papers — e.g. whether a feature belongs in the fast tactical-triage path vs. the slow evidentiary-verification path is a load-bearing distinction, not a stylistic one.

## Commands

### Backend (from `backend/`)

```bash
python3.11 -m venv venv && source venv/bin/activate   # first time only
pip install -r requirements.txt                        # first time / after requirements.txt changes
cp .env.example .env                                    # first time only

python main.py                                          # run dev server -> http://localhost:8000 (docs at /docs)
```

There is no test suite, linter, or migration setup yet (`alembic` is a dependency but no `alembic/` directory exists, and `backend/app/models/` is currently empty). When adding the first tests, check whether a runner convention (pytest config, `conftest.py`) has since been added before assuming pytest defaults.

### Frontend (from `frontend/`)

```bash
npm install
npm run dev       # Vite dev server with HMR -> http://localhost:5173, proxies /api to localhost:8000
npm run build      # tsc -b && vite build
npm run lint       # eslint .
npm run preview    # preview a production build
```

There is no frontend test suite configured.

### Full stack via Docker

```bash
# Recommended for local dev: infra in Docker, frontend local with HMR
docker-compose -f docker-compose.dev.yml up      # postgres (pgvector) + redis + backend
cd frontend && npm run dev

# Full containerized stack (production-shaped)
docker-compose up -d                              # postgres + redis + backend + frontend (nginx, :3000)
```

```bash
docker logs trackid-backend -f
docker exec -it trackid-postgres psql -U postgres -d trackid
docker exec -it trackid-redis redis-cli
```

## Architecture

### Backend layering (`backend/app/`)

Requests flow `api/` → `dependencies/` → `services/`, with `schemas/` (Pydantic) at the API boundary and `models/` (SQLAlchemy ORM, currently unpopulated) for persistence:

- **`api/`** — FastAPI routers. Thin: validate input, call a service via `Depends`, return a schema.
- **`dependencies/`** — FastAPI dependency-injection factories that wire concrete implementations (e.g. `get_face_detection_service` injects the process-global `FaceAnalysis` model into `FaceDetectionService`).
- **`services/`** — Framework-agnostic business logic. `face_detection_service.py` takes/returns plain numpy arrays and dicts — no DB or HTTP concerns — which is what makes it unit-testable without spinning up FastAPI.
- **`core/`** — Cross-cutting setup: `config.py` (pydantic-settings, env-driven), `database.py` (async SQLAlchemy engine/session, `get_db` dependency), `ml_models.py` (ML model lifecycle).
- **`schemas/`** — Pydantic request/response models, one module per feature area (mirrors `api/`).
- **`models/`** — SQLAlchemy ORM models (not yet defined — this is the next major piece of backend work per the README's "Next Steps").

### ML model lifecycle

The InsightFace `FaceAnalysis` model is expensive to load (model download + init) and is loaded **once** at process startup via `load_face_app()` in `main.py`'s `lifespan` context, cached in a module-level global in `ml_models.py`, then fetched per-request through `get_face_app()`. Do not instantiate `FaceAnalysis` per-request. GPU vs CPU execution provider is chosen via `FACE_USE_GPU`.

Face detection runs in a threadpool (`starlette.concurrency.run_in_threadpool`) since InsightFace inference is synchronous/CPU-bound and would otherwise block the async event loop — follow this pattern for any other blocking CV/ML work called from an async route.

`FaceDetectionService` exposes two paths with different costs:
- `detect_faces()` — full pipeline: detection + ArcFace embedding + age/gender + blur score. Use when you need the 512-d embedding.
- `detect_faces_lightweight()` — detector only (SCRFD), ~3-5x faster, no embedding. Use for cases like a quick face-count/bbox preview; call `extract_face_embedding()` afterward only for faces you actually need embeddings for.

Face embeddings are 512-d normalized ArcFace vectors; similarity is cosine similarity (`compare_embeddings`), with ~0.4 cited as a typical "same person" threshold in code comments — but see the research docs for the more nuanced tripartite threshold policy (auto-merge / tactical-queue / auto-reject) that the actual product logic should implement rather than a single cutoff.

### Data layer

PostgreSQL 16 with the `pgvector` extension (via the `pgvector/pgvector:pg16` image) is the intended store for both relational case data and vector similarity search (HNSW indexing, per the research docs) — there's no separate vector DB. SQLAlchemy 2.0 async (`asyncpg` driver) is the ORM; sessions come from `get_db()` in `core/database.py`.

### Frontend

Vite + React 19 + TypeScript + TailwindCSS 4, structured as pages (`src/pages/`) + layout (`src/layout/`) + routing in `src/App.tsx` (`react-router` v7). The Vite dev server proxies `/api/*` to `http://localhost:8000` (see `vite.config.ts`), so frontend code should call relative `/api/...` paths rather than hardcoding a backend origin. In production the nginx container performs the same proxying (`frontend/nginx.conf`).

### Docker composition

Two compose files share the same `postgres`/`redis`/`backend` service definitions: `docker-compose.dev.yml` omits the frontend service (run it locally with `npm run dev` for HMR) and sets `DEBUG=True`; `docker-compose.yml` adds a built, nginx-served frontend container and sets `DEBUG=False`. Both bind-mount `./backend:/app`, so backend code edits take effect without a rebuild.
