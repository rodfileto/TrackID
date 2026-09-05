# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

TrackID is a facial recognition and tracking system: a **Rust backend** (axum) with a **Python ML sidecar** (InsightFace/ArcFace + MagFace for face detection/embedding, OpenCV + DBSCAN for video processing), a **PostgreSQL + pgvector** store for vector similarity search, a **Memgraph** graph database for the ontology and entity graph, and a React admin dashboard frontend (the TailAdmin React scaffold, with a TrackID-specific video-processing page and a wired auth login/signup flow).

The project is also the vehicle for a 3-paper academic research pipeline (tactical case linking → strategic network intelligence → forensic evidentiary validation). See `docs/RESEARCH_STRATEGY.md`, `docs/PAPER1_TACTICAL_LINKING.md`, `docs/PAPER2_INTELLIGENCE.md`, and `docs/PAPER3_FORENSIC_EVIDENTIARY.md` before making architectural changes that would affect the evaluation story of any of these papers — e.g. whether a feature belongs in the fast tactical-triage path vs. the slow evidentiary-verification path is a load-bearing distinction, not a stylistic one.

The backend was migrated from FastAPI to Rust (see `docs/RUST_TRANSITION_PLAN.md`). Only the ML sidecar remains Python, because InsightFace is Python-native.

## Commands

### Rust backend (from `backend-rust/`)

```bash
cargo build          # compile
cargo run            # dev server — BIND defaults to 0.0.0.0:8080 (0.0.0.0:8000 in Docker)
```

Env vars (all have dev-friendly defaults): `DATABASE_URL` (`postgres://postgres:password@localhost:5432/trackid`), `ML_SIDECAR_URL` (`http://localhost:8001`), `MEMGRAPH_URI` (`bolt://localhost:7687`), `S3_*`, `ONTOLOGY_DIR` (`../backend/app/ontology/vendor`), `JWT_SECRET`, `RESOLUTION_TAU_HIGH/TAU_LOW`, `RESOLUTION_CANDIDATE_POOL_SIZE`, `RESOLUTION_TOP_K`, `BIND`.

There is no test suite yet. Schema is bootstrapped idempotently at startup (`db::ensure_face_schema` — `CREATE TABLE IF NOT EXISTS` + HNSW indexes), not via migrations.

### ML sidecar (from `backend/`)

```bash
python3.11 -m venv venv && source venv/bin/activate   # first time only
pip install -r requirements.txt                        # first time / after requirements.txt changes

uvicorn ml_sidecar.main:app --host 0.0.0.0 --port 8001  # run the sidecar
```

The sidecar is a slim FastAPI app (`ml_sidecar/main.py`) exposing `/ml/v1/detect`, `/ml/v1/detect-lightweight`, `/ml/v1/embed`, and `/ml/v1/video-jobs` (create + poll). It owns all CV/ML and is **stateless** w.r.t. Postgres/Memgraph/MinIO. The Rust backend calls it over HTTP+JSON.

### Frontend (from `frontend/`)

```bash
npm install
npm run dev       # Vite dev server with HMR -> http://localhost:5173, proxies /api to localhost:8000
npm run build      # tsc -b && vite build
npm run lint       # eslint .
npm run preview    # preview a production build
```

There is no frontend test suite. The app requires authentication: `/signin` and `/signup` are wired to `/api/v1/auth/*` (token in `localStorage`, `AppLayout` guarded, `Authorization: Bearer` header on API calls).

### Papers (from `papers/`)

The 3-paper research pipeline (see `docs/RESEARCH_STRATEGY.md`) is authored as Quarto (`.qmd`) articles under `papers/`, one subdirectory per paper. `papers/_quarto.yml` holds shared defaults (format, bibliography, author); each paper's own `_metadata.yml` overrides just what differs for that paper (title, keywords, journal-specific format tweaks) — see `papers/README.md` for how the merge works.

Quarto CLI + TinyTeX are installed natively (not Docker — see `papers/README.md` for setup):

```bash
cd papers && quarto preview      # live reload
cd papers && quarto render       # render all papers
```

The editor is desktop VS Code with the Quarto extension (`code --install-extension quarto.quarto`) — see `papers/README.md`.

### Full stack via Docker

```bash
# Recommended for local dev: infra + Rust backend + sidecar in Docker, frontend local with HMR
docker compose -f docker-compose.dev.yml up      # postgres + redis + memgraph + minio + rust-backend + ml-sidecar
cd frontend && npm run dev

# Full containerized stack (production-shaped)
docker compose up -d                              # + frontend (nginx, :3000)
```

```bash
docker logs trackid-rust-backend -f
docker logs trackid-ml-sidecar -f
docker exec -it trackid-postgres psql -U postgres -d trackid
docker exec -it trackid-redis redis-cli
```

## Architecture

### Domain model (the ontology)

The domain is grounded in a BFO + Common Core Ontologies (CCO) upper/mid stack with a thin `tid:` extension, vendored as TTL under `backend/app/ontology/vendor/` and imported + seeded into Memgraph at startup by the Rust service (`backend-rust/src/ontology.rs`, using `oxttl`/`oxrdf`). Two `tid:` files:

- **`face-record.ttl`** — `cco:Person` (the referent) with digital representations `FaceRecord`/`FaceImage`/`FaceEmbedding` (and the `SurveillanceFaceImage`/`EnrollmentFaceImage` provenance split), linked by `face_record_of`/`has_face_record`, `depicts`, `derived_from`, `has_face_image`, `has_face_embedding`.
- **`target-centric.ttl`** — `TargetSystem`/`Situation` (InformationContentEntity) and `Incident`/`Observation` (occurrent), linked by `tracked_in`, `explains`, `organized_under`, `has_observation`, and the bridge `Observation -[produces]-> FaceRecord`.

Key semantics: a "cluster" is **emergent** (N `FaceRecord`s `face_record_of` the same `Person`); "unidentified" is a **state** (a `Person` with no `IDENTIFIES` edge), not a class. The DL reasoner (HermiT via ROBOT, `backend/scripts/fetch_robot.sh`) remains an offline Java tool — the Rust importer does structural import only.

### Rust backend (`backend-rust/src/`)

One axum service owning everything except CV/ML:

- **`main.rs`** — routes + handlers (video processing, face resolution, target-centric CRUD, auth), env-driven config, `AppState` (pg pool, S3, sidecar client, graph, job store, resolution thresholds, auth).
- **`db.rs`** — sqlx + pgvector queries and idempotent schema (`face_embeddings`, `face_review_items`, `users`, `videos`, `person_tracks`, `face_detections`).
- **`graph.rs`** — neo4rs operations for the instance layer (`Person`, `FaceRecord`, `Identity`, `TargetSystem`, `Situation`, `Incident`, `Observation` + edges, analyst-action audit nodes).
- **`resolve.rs`** — the confidence-gated resolution control loop (match → gate: τ_high auto-commit / τ_low queue / new Person) and the review-queue workflow.
- **`ontology.rs`** — TTL import, conformance check, idempotent Memgraph seed.
- **`auth.rs`** — argon2 hashing + JWT (HS256) + `from_fn_with_state` middleware; `AuthUser` is inserted into request extensions.
- **`sidecar.rs`** — HTTP client for the ML sidecar. **`storage.rs`** — MinIO/S3 via `object_store` (upload + SigV4 presigned URLs). **`jobs.rs`** — in-memory video-job store. **`error.rs`** — JSON `{"detail": ...}` error type.

The vector-search hot path (`db::find_candidates`) runs pgvector HNSW cosine (`<=>`) over the `face_embeddings` table, grouped best-per-Person (surveillance) and best-per-Identity (enrollment).

### ML sidecar (`backend/ml_sidecar/` + `backend/app/`)

The Python sidecar wraps `FaceDetectionService` / `VideoProcessingService` (`backend/app/services/`) over HTTP. Model lifecycle lives in `backend/app/core/ml_models.py` (loaded once at startup; GPU vs CPU via `FACE_USE_GPU`). `backend/app/core/gpu_env.py` handles the CUDA library path. Do not instantiate InsightFace per-request, and follow the `run_in_threadpool` pattern for blocking CV/ML work.

`FaceDetectionService` exposes `detect_faces()` (full: detection + ArcFace embedding + age/gender + blur/quality) vs `detect_faces_lightweight()` (SCRFD only, ~3-5x faster). Face embeddings are 512-d normalized ArcFace vectors; similarity is cosine, with the tripartite threshold policy (auto-merge / tactical-queue / auto-reject) per the research docs — not a single cutoff.

### Data layer

**Postgres 16 + pgvector** (`pgvector/pgvector:pg16`) is the only vector store — no separate vector DB. **Memgraph** holds the ontology taxonomy and the entity/relationship graph; Postgres holds the artifacts (embeddings, video results, users, review queue) with UUID pointers into Memgraph (the two-tier split). **MinIO** holds media blobs (video files, face crops).

### Frontend

Vite + React 19 + TypeScript + TailwindCSS 4, structured as pages (`src/pages/`) + layout (`src/layout/`) + routing in `src/App.tsx` (`react-router` v7). The Vite dev server proxies `/api/*` to `http://localhost:8000` (`vite.config.ts`); in production nginx does the same (`frontend/nginx.conf`). Auth helpers live in `src/auth.ts`; API calls use relative `/api/...` paths with the `Authorization: Bearer` header.

### Docker composition

Two compose files share the `postgres`/`redis`/`memgraph`/`minio`/`ml-sidecar` services and a `rust-backend` service built from `backend-rust/Dockerfile`. `docker-compose.dev.yml` omits the frontend (run it locally for HMR) and sets `DEBUG=True`; `docker-compose.yml` adds the nginx-served frontend and sets `DEBUG=False`. The Rust container bind-mounts `./backend/app/ontology/vendor:/ontology:ro` (set via `ONTOLOGY_DIR`).
