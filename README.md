# TrackID

TrackID is a reusable package for intelligence and forensic agencies that manage
criminal cases for biometric traces (fingerprints, faces, and other modalities).

It provides a stable core — the Postgres and Neo4j schemas plus all non-import logic —
and a single, well-defined extension point: **import commands**. An organization builds
its own distribution on top of this package and contributes only the commands that
translate its own data formats into the core schema. Nothing organization-specific lives
in this repository.

## Scope

The package is deliberately split into two halves:

- **Core (this repository):**
  - the Postgres schema (users, the enrollment and evidence hierarchies, the unified
    `biometricfeature` table, `biometric_decisions`, and clusters) and the Neo4j schema
    (identity enrollment, forensic evidence, biometric clusters);
  - the non-import logic: authentication, the `cases` read API, the Postgres → Neo4j
    graph sync, biometric clustering by modality (with stable cluster ids), and
    identified-person linkage;
  - the base frontend (auth + app layout).
- **Organization layer (built by each agency):** the import commands that read their
  specific files or systems and write normalized records into the core schema, plus any
  extra routes or frontend pages they need.

## Data flow

```
organization import cmd  ──►  Postgres (core schema)
                                     │
         core logic commands  ◄──────┘   (read the DB only)
                                     │
                                      ├──►  Neo4j (evidence, identity, clusters)
                                     └──►  core API ──► frontend
```

The database is the boundary between the two halves. Every logic entry point reads from
Postgres — never from an external file or third-party system — so organizations can
reuse the full core unchanged regardless of their source formats.

## Structure

The package is split between a **public** surface (the packages downstream
organizations import to build their import commands) and an **internal** runtime
(the code that only this repository's own binaries need).

**Public — the reusable core (importable by organizations):**

- `api/` — the HTTP router and every core handler (health, auth, cases), plus the
  `Dependencies.Register` hook integrations use to attach their own routes
- `auth/` — user accounts: `UserProfile`, the data-access `Repository`, and JWT
  issuance (no HTTP handlers; those live in `api/`)
- `cases/` — the `Case` type and the generic `Ingest` extension contract for the
  QUESTIONED (evidence) hierarchy (no HTTP handlers)
- `identity/` — the generic `Ingest` extension contract for the KNOWN (enrollment)
  hierarchy
- `cluster/` — decision-status derivation and biometric clustering by modality
  (persisted, stable ids; extend/merge/split) plus identified-person linkage, all
  driven by `biometric_decisions`, reading Postgres and writing Neo4j
- `graph/` — embedded Neo4j (Cypher) schema migrations, their applier, the feature-type
  mapping, and the Postgres → Neo4j sync of evidence, the identity chain, and
  biometric features
- `biometricmatch/` — ANN similarity search over `feature_embeddings` writing SYSTEM
  decisions
- `storage/` — MinIO/S3 object-storage client
- `db/` — sqlc queries and models for the core Postgres schema (`queries.sql`) and the
  goose migrations (`db/migrations`)

**Internal — this repository's own runtime (not importable downstream):**

- `internal/config/`, `internal/env/`, `internal/database/` — configuration, dotenv
  loading, and the Postgres connection pool
- `internal/cmdutil/` — shared bootstrap helpers for the `cmd/` tools (`-commit`
  dry-run flag, DB/Neo4j opening)
- `internal/server/` — the composition root: wires the API router and mounts the
  prototype frontend
- `internal/web/` — serves the built frontend as a single-page application

**Commands and other:**

- `cmd/` — thin `main()` wrappers: `migrate` (Postgres schema), `trackid` (API server
  + frontend), `sync-graph`, `sync-identity`, `cluster-biometrics`, `identify`, and
  `match-embeddings`
- `frontend/` — React + Vite, based on the [TailAdmin React](https://github.com/TailAdmin/free-react-tailwind-admin-dashboard)
  template; `npm run build` produces the bundle `internal/web` serves
- `quarto/` — technical report and ontology documentation
- `scripts/` — development helpers (start/stop the stack, apply the graph schema)
- `docker-compose.yml` — Postgres, Redis, MinIO, and optional Neo4j

## Extension contract

To integrate with this package, an organization:

1. depends on `github.com/rodfileto/trackid`;
2. writes import commands that read its own formats and write into the core tables —
   the enrollment hierarchy (`person`, `identity_document`, `identity_register`,
   `identity_file`), the evidence hierarchy (`criminal_cases`, `case_files`,
   `case_evidences`, `case_traces`, `case_codifications`), the unified
   `biometricfeature`, and the `biometric_decisions` log — via the queries declared in
   `db/queries.sql`;
3. optionally registers extra routes through the `api.Dependencies.Register` hook and
   copies the base frontend to add its own pages.

## Development

```sh
scripts/start_dev.sh   # starts the stack, migrates, and runs the API + frontend
```

## Models and licences

Face detection and embedding run through [trackid-vision](https://github.com/rodfileto/trackid-vision)
(see its README for the full table). The code here is not bound by these terms; the
weights are, and they are not bundled — you download them.

| Model | Role | Terms |
|---|---|---|
| YuNet `face_detection_yunet_2026may.onnx` (OpenCV Zoo) | detector, **default** (`VISION_DETECTOR=yunet`) | MIT. Trained on WIDER Face (CC BY-NC-ND); whether that reaches trained weights is unsettled, and the author states no restriction. |
| AuraFace-v1 `glintr100.onnx` (fal) | recognizer | Apache-2.0; fal describes the training data as commercially and publicly available, without naming it. |
| SCRFD `scrfd_10g_bnkps.onnx` (InsightFace) | detector, opt-in (`VISION_DETECTOR=scrfd`) | **Non-commercial research only** (InsightFace's terms). Do not use in a commercial product or service without their licence. |

Notes for anyone deploying this:

- **Thresholds are yours to validate.** `faceMatchThreshold` and the detector's score
  threshold are defaults, not calibrated on any operational data. A laboratory should
  validate them on its own data before casework use.
- **Embeddings from different detectors are not comparable** (the same face aligned by
  SCRFD vs YuNet had cosine 0.88 on average). Re-embed everything when switching.
- Terms change; check each model's source before relying on this table.

## Not yet in scope

- Per-organization distribution modules, their import commands, and external-system
  integrations (built in each organization's own project).
- Frontend theming beyond the shared template.
- Person↔person merge — reconciling two civil identities that are the same real
  individual (a later, auditable step, distinct from cluster merge).
