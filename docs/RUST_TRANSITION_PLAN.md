# TrackID Backend Transition Plan: Python → Rust + ML sidecar

Status: **proposal — for review, updated for the face-record domain model.**
Nothing here is implemented yet.

## 1. Decision summary

Move the backend from a single FastAPI process to:

- **Rust (axum)** — owns everything *except* the CV/ML work: REST routes, auth,
  video-job orchestration, Postgres + pgvector (vector search), Memgraph
  (ontology + graph), MinIO media persistence, review queue.
- **Python sidecar (FastAPI)** — owns only InsightFace / MagFace / video frame
  processing / DBSCAN clustering. It is an "embedding + detection oracle" with
  no database or object-storage credentials of its own.

The frontend does **not** change. The `/api/v1/*` URL contract is preserved
end-to-end; only the Vite dev proxy target moves from FastAPI to the Rust
service once it reaches parity.

**Domain-model note (this revision's headline):** the micro-target node type
`TargetEntity` is **removed**. The domain is now grounded in the `tid:`
face-record ontology (`backend/app/ontology/vendor/tid/face-record.ttl`), which
reuses `cco:Person` as the referent and models the digital layer
(`FaceRecord`/`FaceImage`/`FaceEmbedding`) explicitly. The Rust port is the
**first implementation** of this model; the existing Python `TargetEntity`
pipeline is legacy reference, not a 1:1 port source. The investigative
container structure (Target/Situation/Event/Case) is **kept but deferred** — it
is re-modeled against CCO in a later phase, not ported now.

## 2. Why

- **Memory / CPU efficiency** on the request-heavy and vector-search paths.
  pgvector HNSW queries, threshold gating, and the review-queue are exactly the
  kind of latency- and allocation-sensitive work Rust is well suited for.
- **Vector search is the target** (the user's stated goal): face-to-Person
  resolution is the product's hot path and is what we want running compiled,
  not interpreted.
- **ML stays Python** because InsightFace (and its alignment/NMS/ArcFace
  preprocessing) is Python-native and not worth porting.
- **The ontology is a first-class asset** now: it is BFO/CCO-clean, validated
  (conformance + HermiT consistency), and auto-seeded. The Rust schema should be
  *derived from* it, not hand-rolled to match legacy code.

## 3. Domain model (the anchor)

From `face-record.ttl` (imported, validated, and seeded automatically — the
importer/seed/reasoner all auto-discover `vendor/tid/`):

```
cco:Person (material referent)          ← the thing we're identifying
   ▲ face_record_of / has_face_record
tid:FaceRecord (GDC)                     ← one observation or enrollment
   ├─ has_face_image → tid:FaceImage ── depicts → tid:Face (fiat object part)
   └─ has_face_embedding → tid:FaceEmbedding ── derived_from → FaceImage
```

Provenance split: `SurveillanceFaceImage` vs `EnrollmentFaceImage` (CCTV vs
identity-document photo) — this replaces the old "identity template vs
observation" dual candidate-pool with an ontology-level distinction.

Three semantic consequences that drive the Rust schema:

1. **"Cluster" is emergent, not a node.** N `FaceRecord`s `face_record_of` the
   same `Person`. There is no cluster node to create — clustering *is* the act
   of linking a new `FaceRecord` to an existing `Person`.
2. **"Unidentified" is a state, not a class.** A `Person` with no `IDENTIFIES`
   edge is unidentified; the same node becomes identified when an `Identity`
   attaches. Nothing is re-typed or moved.
3. **`Face` (the physical part) vs `FaceImage` (the depiction) stay distinct**,
   preserving the Paper-3 evidentiary distinction already documented in
   `ONTOLOGY_BFO.md`.

### Legacy → new mapping (reference, not port)

| Legacy (Python) | New (Rust) | Notes |
|---|---|---|
| `TargetEntity` (entity_type=`TargetPerson`) cluster | `cco:Person` node | cluster ⇒ emergent from `face_record_of` edges |
| `ResolutionEmbedding` (Postgres `resolution_embeddings`) | `FaceEmbedding` row | Postgres = artifact, per existing two-tier split |
| `IdentityFaceTemplate` | `EnrollmentFaceImage` + its `FaceEmbedding` | provenance split, not a second table |
| `Identity` | `Identity` (document-backed), `IDENTIFIES → Person` | mostly unchanged |
| `Event` (observation) | `FaceRecord` (observation kind) | container structure deferred |
| `Target`/`Situation`/`Case` | **deferred** — re-model against CCO later | see §7 Phase 5 |

## 4. Current architecture (baseline)

Backend (FastAPI, `backend/app/`):

| Layer | Modules | Notes |
|---|---|---|
| API | `api/{cases,entities,events,face_detection,identities,resolution,targets,video_processing,videos}.py` | thin routers under `/api/v1` |
| Services | `face_detection_service`, `quality_estimation_service`, `video_processing_service`, `video_persistence_service`, `media_storage_service`, `entity_resolution_service` | framework-agnostic business logic |
| Core | `config`, `database`, `graph`, `ml_models`, `ontology`, `storage`, `video_jobs`, `gpu_env` | wiring + lifecycle |
| Ontology | `ontology/{importer,validation,persistence,reasoner,models}.py` | BFO/CCO/tid import → Memgraph |
| Graph | `graph/{service,schemas}.py` | Memgraph instance layer |
| Models | `models/{resolution,video_processing}.py` | SQLAlchemy ORM (pgvector) |

Stores: **Postgres 16 + pgvector** (embeddings, review queue, video results),
**Memgraph** (ontology + instance layer), **MinIO** (media blobs), **Redis**
(unused — taskiq is a dependency but no queue is wired).

**Mid-migration state (important):** the `tid:` ontology is written and
validated, but the Python code still speaks `TargetEntity`/`TargetPerson`. The
Rust port does **not** wait for the Python side to migrate first — it builds
directly against the `face-record.ttl` model. The two will coexist until the
Python orchestration layer is retired.

## 5. Target architecture

```
React (Vite dev :5173)
   │  /api/*  (proxied)
   ▼
Rust (axum :8080 → later :8000)
   │  routes, auth, job orchestration, review queue
   ├─► Postgres + pgvector   (sqlx + pgvector crate, HNSW vector search)
   ├─► Memgraph              (neo4rs, Bolt) — taxonomy seed + Person/FaceRecord graph
   ├─► MinIO                 (object_store / aws-sdk-s3) — media persistence
   │
   └─► Python ML sidecar (FastAPI :8001)
          InsightFace (ArcFace), MagFace, cv2 frame extraction, DBSCAN
```

### Module → language mapping

| Current module | New home | Rationale |
|---|---|---|
| `services/face_detection_service.py` | **Python sidecar** | InsightFace |
| `services/quality_estimation_service.py` | **Python sidecar** | MagFace ONNX |
| `services/video_processing_service.py` | **Python sidecar** | cv2 + sklearn DBSCAN, tightly coupled to detection |
| `core/ml_models.py`, `core/gpu_env.py` | **Python sidecar** | model lifecycle + CUDA path |
| `dependencies/{face_detection,video_processing}.py` | **Python sidecar** | DI wiring for the above |
| `api/*` (all routers) | **Rust** | HTTP surface |
| `services/entity_resolution_service.py` | **Rust — re-modeled, not ported** | new Person/FaceRecord resolution (§7 Phase 3) |
| `services/video_persistence_service.py` | **Rust** | Postgres writes |
| `services/media_storage_service.py` | **Rust** | MinIO/S3 |
| `core/{config,database,graph,ontology,storage,video_jobs}.py` | **Rust** | wiring + lifecycle |
| `models/*` (SQLAlchemy) | **Rust** (sqlx migrations) | ORM → typed queries; schema derived from the ontology |
| `ontology/{importer,validation,persistence}.py` | **Rust** (`oxttl`/`sophia` + `neo4rs`) | TTL parse + structural checks |
| `ontology/reasoner.py` | **stays offline** (HermiT/ROBOT, Java) | dev-time consistency check, not runtime |
| `graph/{service,schemas}.py` | **Rust — re-modeled** (Person/FaceRecord) | instance layer; container structure deferred |

**Auth** is greenfield — the current backend has none. It is *added* in Rust
(`argon2` + `jsonwebtoken` + tower middleware), not ported.

## 6. The ML sidecar contract (the load-bearing seam)

The sidecar is **stateless** with respect to Postgres/Memgraph/MinIO: it takes
bytes/paths in, returns detections/tracks out. Orchestration (Rust) owns all
persistence and job state.

### 6.1 Face detection (image)

- `POST /ml/v1/detect` — multipart image (or raw bytes) →
  `{ faces: [{ bbox, confidence, embedding, estimated_age?, estimated_gender?,
  landmarks?, blur_score?, quality_score? }] }`
  — wraps `FaceDetectionService.detect_faces`.
- `POST /ml/v1/detect-lightweight` →
  `{ faces: [{ bbox, confidence, kps, blur_score? }] }`
  — wraps `detect_faces_lightweight`.
- `POST /ml/v1/embed` — image + `kps` → `{ embedding: [512 floats] }`
  — wraps `extract_face_embedding`.

### 6.2 Video processing (long-running job)

Mirrors today's `/process-video` shape, scoped to ML only:

- `POST /ml/v1/video-jobs` (multipart: video file + params JSON) → `{ job_id }`
- `GET /ml/v1/video-jobs/{job_id}` →
  `{ status, frame_count, expected_frames, total_detections, percent,
  result: { tracks: [...] } }`
  — `tracks` matches the existing `VideoProcessingResponse` schema, with face
  crops as base64 data-URIs.

**Transport decision:** start with **HTTP + JSON** (matches existing Pydantic
schemas, testable with curl, no codegen). Upgrade the video-job path to
**gRPC streaming** later only if base64-crops-over-HTTP becomes a bottleneck.

## 7. Phased plan

Each phase leaves the system runnable in the dev server.

### Phase 0 — De-risk (timeboxed spikes, no rewrite)

**Status:** spikes 1–3 **passed** (2026-09-01). Code in `spikes/` (run with
`cargo run --bin neo4rs_spike` / `--bin pgvector_spike` /
`--bin axum_sidecar_spike` against `docker-compose.dev.yml`'s memgraph +
postgres, plus `spikes/ml_sidecar/main.py`).

1. **`neo4rs` ↔ Memgraph** — ✅ connects over Bolt, creates and reads back the
   full face-record graph (`Person`/`FaceRecord`/`FaceImage`/`FaceEmbedding` +
   `face_record_of`/`has_face_image`/`has_face_embedding`/`depicts`/`derived_from`).
2. **`sqlx` + `pgvector` crate** — ✅ extension, insert, HNSW cosine index,
   `<=>` search with correct ordering (0.0 vs 1.0 cosine distance on orthogonal
   probes).
3. **Sidecar contract** — ✅ an axum handler (`axum_sidecar_spike.rs`) uploads a
   multipart file to a mock FastAPI sidecar (`ml_sidecar/main.py`), deserializes
   the `detect` JSON (512-d embedding intact), kicks a video job, and polls it
   to completion. The full Rust↔Python seam works over HTTP+JSON.
4. **Decide transport** — ✅ **HTTP+JSON confirmed** for v1. The multipart +
   JSON + job-polling pattern round-trips cleanly with `reqwest`; gRPC would add
   codegen + a second transport for no current benefit. Revisit only if
   base64-crops-over-HTTP becomes a bottleneck on large videos.

*Exit:* **GO** on the Rust graph + vector + sidecar primitives. Pin `neo4rs`
(currently `0.9.0-rc.10`, a release candidate — expect minor API churn); it
negotiates Bolt 4.x only, which Memgraph accepted, and uses application-level
`id` properties rather than Neo4j element IDs (matching the existing Python
pattern).

### Phase 1 — Extract the ML sidecar (Python only, no Rust yet)

**Status:** **done** (2026-09-01), verified end-to-end locally against real
InsightFace models.

Split the current backend into two Python processes behind the same `/api/v1`
surface:

- **Sidecar** (`:8001`, `backend/ml_sidecar/main.py`): a slim FastAPI app
  exposing §6, wrapping the existing `FaceDetectionService` +
  `VideoProcessingService`. ML model loading moved here (`ml_models.py`,
  `gpu_env.py` lifespan). It is stateless w.r.t. Postgres/Memgraph/MinIO.
- **Orchestration** (`:8000`, `backend/main.py`): the existing app minus the
  in-process ML — `video_processing.py` / `face_detection.py` / `resolution.py` /
  `identities.py` call the sidecar over HTTP (sync `httpx` in
  `app/core/ml_client.py`, via `run_in_threadpool`) instead of
  `Depends(get_..._service)`. The `app/dependencies/{face_detection,
  video_processing}.py` factories were deleted; crops now arrive as base64 data
  URIs (`app/services/media_storage_service.py` no longer uses cv2).

*Why first:* it proves the seam is clean at **zero Rust risk**, and keeps the
video page working while Rust is built.

*Exit:* identical frontend behavior, now exercising the HTTP boundary —
verified: `detect-faces` round-trips through the sidecar with the real model,
and `process-video` runs (frames → detect → DBSCAN) in the sidecar while the
orchestration polls and persists to Postgres + MinIO (`video_id` returned, row
visible in `GET /api/v1/videos`).

### Phase 2 — Rust orchestration for the video-processing slice

**Status:** **done** (2026-09-01), verified end-to-end locally against the
Python ML sidecar + Postgres + MinIO. Code in `backend-rust/` (run
`cargo run`, listens on `:8080` by default).

Stand up the axum app and port **only** the routes the video page needs:

- `POST /api/v1/process-video` (upload → temp file → sidecar job → `job_id`)
- `GET /api/v1/process-video/{job_id}` (poll Rust in-memory job store)
- `GET /api/v1/videos`, `GET /api/v1/videos/{video_id}` (Postgres via sqlx)
- Persistence: `video_persistence_service` + `media_storage_service` in Rust
  (Postgres + MinIO), reusing the two-phase insert for the circular FK.

Vite proxy `/api` now → Rust `:8080`. **Frontend `VideoProcessing.tsx` is
unchanged.** This slice is a faithful port of the existing standalone video
pipeline — it does not yet touch the face-record graph (that unification is
deferred; see §9 open decision 6).

*Exit:* the video page runs entirely on the Rust backend + Python sidecar —
verified: upload → sidecar job → poll → completed with `video_id`, row in
Postgres, video + crops in MinIO (correct content-types), and presigned crop
URLs serving 200. Stack: `axum 0.8` + `sqlx 0.9` + `pgvector 0.4` +
`object_store 0.12` (SigV4 presigning against MinIO) + `reqwest 0.12`.

### Phase 3 — Face resolution in Rust (the core goal, re-modeled)

**Status:** **done** (2026-09-01), verified end-to-end with real InsightFace
embeddings. Code in `backend-rust/` (`graph.rs`, `resolve.rs`, `db.rs`,
`sidecar.rs`); idempotent schema bootstrapped at startup.

Build the new resolution pipeline against the face-record model — **not** a port
of `entity_resolution_service.py`:

- **Schema**: `face_embeddings` + `face_review_items` (Postgres, HNSW cosine) +
  Memgraph `Person`/`FaceRecord`/`Identity` nodes with `face_record_of` /
  `IDENTIFIES` edges. "Cluster" is emergent; "unidentified" is a state.
- **Ingest**: `POST /api/v1/observations` → sidecar `detect` → `FaceRecord` +
  `FaceEmbedding` (surveillance) → match → gate.
- **Match**: sqlx + pgvector HNSW over both pools (surveillance grouped by
  Person, enrollment grouped by Identity), best-per-entity, merged + re-ranked.
- **Gate**: τ_high auto-commit / τ_low queue / new unidentified Person —
  tripartite policy (Paper 1). Plus the review-queue workflow
  (`confirm`/`confirm-new`/`confirm-identity`/`reject`).
- **Enrollment**: `POST /api/v1/identities` → `Identity` + enrollment
  `FaceEmbedding`; first match resolves-or-creates the Person (`IDENTIFIES`).

*Exit:* verified — same face → `auto_committed` (sim 1.0), different face →
`new_person`, enrolled identity → `auto_committed` via the identity pool →
`Person` named after the identity; review-queue `confirm-new` transitions the
item and creates the person. `GET /api/v1/persons`, `/persons/{id}`,
`/identities/{id}`, `/review-queue` all exercised.

### Phase 4 — Ontology import + taxonomy seed in Rust

**Status:** **done** (2026-09-01), verified against the real BFO + CCO + tid
files. Code in `backend-rust/src/ontology.rs`.

- Port `ontology/{importer,validation,persistence}.py` to Rust using `oxttl` /
  `oxrdf` for TTL and `neo4rs` for the idempotent `MERGE` taxonomy seed. Domain
  /range class-expression flattening (union/intersection/restrictions) is
  reproduced; the tid: provenance split (Surveillance/Enrollment) imports as-is.
- Keep `ontology/reasoner.py` (HermiT via ROBOT) as an offline Java tool.
- `Person`/`FaceRecord` instance validation against the taxonomy
  (`validate_edge` domain/range, `is_occurrent`/`is_continuant`/…) is ported as
  functions and ready to wire into the instance layer (multi-label stamping).

*Exit:* verified — `conformant=true (0 violations)` (matches the Python
`python -m app.ontology.validation` result), and the seed produced **1443
classes / 183 relations / 1442 subclass / 203 domain / 202 range edges** in
Memgraph (1443 = 1437 BFO+CCO + 6 tid; 183 = 177 + 6 tid). `FaceRecord`
`SUBCLASS_OF` → `bfo:GenericallyDependentContinuant`; `face_record_of` has
`DOMAIN` FaceRecord and `RANGE` Person, exactly as authored. Startup import +
seed runs in ~4s (per-item MERGE; batching is a future optimization).

### Phase 5 — Container structure (re-modeled) + remaining routes + auth

**Status:** **complete** (2026-09-02) — container structure (5a), auth, and the
final cutover are all done and verified.

- **Re-model Target/Situation/Event/Case against CCO** — **done via the ontology
  (`tid:target-centric.ttl`)** and implemented in `backend-rust/src/graph.rs`:
  `TargetSystem`/`Situation` (ICE) + `Incident`/`Observation` (occurrent), with
  `organized_under`/`explains`/`has_observation`/`tracked_in` edges and the
  `produces`/`produced_by` bridge to `FaceRecord`. `POST /api/v1/observations`
  now creates an `Observation` (provisionally assignable to an incident/target
  system) that `produces` each resolved `FaceRecord`. `Case` is deliberately
  out of scope until after the transition.
- **Auth** — **done** (`backend-rust/src/auth.rs`): `POST /api/v1/auth/register`
  (argon2 hash, 409 on duplicate), `POST /api/v1/auth/login` (JWT HS256 via
  `rust_crypto`), `GET /api/v1/auth/me`; `from_fn_with_state` middleware gates
  every `/api/v1/*` route except the auth endpoints.
- **Final cutover** — **done**: the Rust service runs on `:8000`
  (`backend-rust/Dockerfile`, `rust-backend` + `ml-sidecar` services in both
  compose files, `ONTOLOGY_DIR=/ontology` bind-mount of the vendored TTLs). The
  FastAPI orchestration is deleted (only `ml_sidecar/` + the sidecar's
  `app/{core,services}` subset + `app/ontology/vendor` remain). The Rust
  `ensure_face_schema` now also creates the video tables, so a fresh Postgres
  works without alembic. Frontend: `SignInForm`/`SignUpForm` wired to the auth
  endpoints, token in `localStorage`, `AppLayout` guarded (redirect to
  `/signin`), `Authorization: Bearer` header on the video-processing API calls,
  and the user dropdown "Sign out" clears the token.

## 8. Video-processing wiring (the dev-server test path)

### What already works today

`frontend/src/pages/VideoProcessing.tsx` ↔ FastAPI, unchanged by this plan:

| Frontend call | Backend route | Backend file |
|---|---|---|
| `startVideoProcessing` → `POST /api/v1/process-video` | upload → job | `api/video_processing.py` |
| `fetchVideoJobStatus` → `GET /api/v1/process-video/{id}` | poll status | `api/video_processing.py` |
| `fetchVideoList` → `GET /api/v1/videos` | list persisted | `api/videos.py` |
| `fetchPersistedVideo` → `GET /api/v1/videos/{id}` | persisted result | `api/videos.py` |

Vite dev proxy forwards `/api/*` → `http://localhost:8000`.

### Target wiring (Phase 2)

The same four frontend calls, same URLs, now landing on Rust:

```
VideoProcessing.tsx
  POST /api/v1/process-video ──────────► Rust :8080  (receive upload, temp file)
                                            │  POST /ml/v1/video-jobs
                                            ▼
                                     Python sidecar :8001  (frames → detect → DBSCAN → tracks)
                                            │  progress + result (tracks w/ crops)
                                            ▼
  GET /api/v1/process-video/{id} ◄──────── Rust polls sidecar, surfaces progress
  GET /api/v1/videos               ◄──────── Rust ← Postgres (sqlx)
  GET /api/v1/videos/{id}          ◄──────── Rust ← Postgres + MinIO presigned URLs
```

### How to test it in the dev server

Dev stack: `docker-compose.dev.yml` (postgres/redis/memgraph/minio + backend) +
`cd frontend && npm run dev`.

1. **Phase 1:** add the sidecar to the compose stack (`:8001`); the existing
   `backend` service remains orchestration and calls it over HTTP. Frontend
   unchanged. Exercise the video page — upload, watch progress, load persisted
   results.
2. **Phase 2:** replace the `backend` service's image with the Rust binary, set
   the proxy target to `:8080`. Same manual test — the page's
   upload → progress → persisted-results flow is the acceptance criterion.

## 9. Risks & mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| `neo4rs` (Rust Bolt client) immature vs Python `neo4j` driver | **High** | Phase 0 spike first; fallback = keep a thin Python *graph* sidecar alongside the ML sidecar |
| Domain model still shifting (TargetEntity removal in flight) | **High** | Rust is the first impl of the face-record model; keep schema behind a thin migration layer so a `tid:` change is a re-seed, not a rewrite |
| Base64 crops over HTTP heavy for long videos | Medium | crops already base64 today; upgrade to gRPC streaming later |
| Cross-store atomicity (Postgres + Memgraph) already a known v1 gap | Medium | Not worsened by the move; document, keep best-effort ordering |
| `sqlx` query-time type checks vs SQLAlchemy flexibility | Medium | `sqlx` offline-mode + explicit migrations; keep service-layer logic simple |
| InsightFace GPU path (`gpu_env.py`) must stay correct in sidecar | Low | moves wholesale; no logic change |
| LLM code quality for Rust ≠ Python | Low | Rust/axum/sqlx well-represented in training data |

## 10. Open decisions

1. **Transport** for the sidecar seam: HTTP+JSON now (recommended) vs gRPC from
   the start. → resolve in Phase 0.
2. **Sidecar process count**: single (default, GPU-bound) vs horizontal scale
   later. → defer; keep the seam gRPC-friendly.
3. **Port plan**: Rust takes `:8000` only at final cutover (Phase 5). →
   recommend final-cutover to keep the dev proxy stable.
4. **Where crops are extracted**: sidecar returns base64 (stateless) vs sidecar
   uploads to MinIO (needs S3 creds). → keep base64 for v1.
5. **`object_store` vs `aws-sdk-s3`** crate for MinIO. → resolve in Phase 2.
6. **Video → FaceRecord unification**: should a completed video-processing run
   also produce `FaceRecord`s that feed resolution, or stay a standalone
   "who's in this video" pipeline? → defer; Phase 2 ports the standalone
   pipeline faithfully, unification is a separate design task.
7. **Container-structure re-model** (Target/Situation/Event/Case against CCO):
   out of scope for the initial Rust slice; needs a Paper-1/2-scoped design pass.

## 11. Compatibility & rollback

- Every phase keeps the existing `/api/v1/*` surface and the frontend working,
  so the plan can be paused at any phase without stranding the product.
- Phase 1 is independently valuable even without Rust (cleaner separation of ML
  from orchestration).
- The Python orchestration code is only deleted at Phase 5, after the Rust
  service reaches behavioral parity.
