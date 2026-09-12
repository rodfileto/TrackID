# TrackID — Package Restructuring Plan

## Goal

Turn `trackid` into a reusable package for intelligence and forensic agencies that
manage criminal cases for biometric traces. The package ships a stable core — the
Postgres and Neo4j schemas plus all non-import logic — and exposes a single extension
point: **import commands** that translate an organization's own data formats into the
core schema.

An external organization builds its own distribution module that depends on this one
and contributes only its import commands (and, if needed, its own frontend pages and
external-system integrations). Nothing organization-specific may live in this repo.

## Design principles

1. **Core = schema + logic + API.** Import is the only per-organization code.
2. **The database is the boundary.** Every logic entry point reads Postgres, never an
   external file or a third-party system.
3. **Pipeline:** `import → Postgres → logic → Neo4j / frontend`.
4. **Biometric clustering is generic** — it groups samples by modality (face,
   fingerprint, …), driven by the database, independent of any source format.

## Current state

- Go module `github.com/rodrigorfcm/trackid/backend` (Go + Gin + sqlc + goose +
  Neo4j + pgx + MinIO).
- Postgres schema (`users`, `criminal_cases`, `identity_document`, `identity_register`,
  `gender`) + Neo4j schema (`001_init`, `002_forensic_evidence_capture`,
  `003_biometric_cluster`).
- Generic infrastructure (auth, storage, database, config, router) is currently
  interleaved with one organization's code: an external ABIS client, a CSV importer for
  that organization's fingerprint-case data, import commands, and matching frontend
  "Toolkit" pages.

## Target structure

```
trackid  (module github.com/rodrigorfcm/trackid)
├── go.mod
├── cmd/
│   ├── migrate/                 # postgres goose migrations (up/down/status)
│   └── ...                      # core logic commands (DB-driven)
├── api/                         # generic router: /health, /auth/*, /me + route-registration hook
├── auth/                        # JWT + repository + handlers
├── config/                      # base Config (Port, DATABASE_URL, REDIS_URL, JWT_SECRET, NEO4J_URL)
├── database/                    # pgx connection helper
├── db/                          # sqlc-generated queries + models (core tables only)
├── migrations/                  # embedded *.sql
├── graph/                       # embedded *.cypher + Neo4j client + DB→graph sync
├── cluster/                     # generic biometric clustering (reads DB → writes Neo4j)
├── storage/                     # MinIO/S3 client (no organization-specific helpers)
├── env/                         # dotenv loader
├── frontend/                    # base template (auth, layout; no organization pages)
├── quarto/                      # technical report / ontology docs
├── scripts/                     # dev/start/stop/backup scripts
├── docker-compose.yml
└── PLAN.md
```

## Core package responsibilities

| Package | Responsibility |
| --- | --- |
| `api` | Builds the `gin.Engine`, registers only core routes, exposes a hook for external route registration |
| `auth` | User registration/login, JWT issuance and middleware |
| `config` | Reads core environment variables; no third-party-system settings |
| `database` | Opens and pings the pgx connection pool |
| `db` | sqlc-generated queries/models for core tables |
| `migrations` | Embedded Postgres migration files |
| `graph` | Embedded Cypher migrations, Neo4j driver, and DB→graph materialization |
| `cluster` | Biometric clustering by modality; reads comparison records from Postgres, writes `BiometricCluster` nodes |
| `storage` | Object-storage client (upload/get, bucket management) |
| `env` | `.env` loading (godotenv wrapper) |

## Extension contract for external organizations

An organization consumes this package as a Go dependency and provides:

1. **Import commands** — small `main` programs that read the organization's file/system
   formats and write normalized records into the core Postgres schema. These are the
   only translation-aware code.
2. **Route registration** — optional HTTP handlers for organization-specific tools,
   attached through the hook exposed by `api`.
3. **Frontend pages** — a copy of the core frontend template plus organization-specific
   pages.

The organization never modifies the schema or the logic packages; it only feeds the
schema through its import commands.

## Data-flow pipeline

```
organization import cmd  ──►  Postgres (core schema)
                                    │
          core logic commands  ◄────┘   (read DB only)
                                    │
                                    ├──►  Neo4j (core graph: evidence, decisions, clusters)
                                    └──►  core API ──► frontend
```

This replaces two current commands that read external files directly:

- The "import evidence into graph" command (today CSV → Neo4j) splits into an
  organization-side **import into Postgres** plus a core **graph sync** that reads the
  evidence tables and materializes the graph.
- The "cluster fingerprint evidence" command (today Neo4j → Neo4j) becomes the generic
  `cluster` package reading comparison records from Postgres and writing clusters.

## Required refactors

1. **Repo root becomes the Go module.** Move `backend/*` up one level; module path
   becomes `github.com/rodrigorfcm/trackid`. Update all import paths.
2. **`internal/` → public packages.** Packages external organizations import
   (`api`, `auth`, `config`, `database`, `db`, `graph`, `cluster`, `storage`) cannot
   stay under `internal/`. Only truly private helpers remain internal.
3. **Generic router.** `api.NewRouter(deps)` registers only core routes and returns the
   engine; external code registers its own routes via the returned `*gin.Engine` or an
   explicit `RegisterRoutes` contract. Remove organization-specific dependencies from
   `api.Dependencies`.
4. **Config split.** `config.Load()` keeps only core settings. Third-party-system
   settings move to the organization's own config.
5. **Migrations embedding.** Embed both `*.sql` and `*.cypher` via `go:embed`; expose
   `migrate.Up(db)` and `graph.Apply(driver)`. `scripts/apply_graph_schema.sh` becomes a
   thin wrapper.
6. **Fix sqlc config.** `sqlc.yaml` currently points `schema` at the wrong path
   (`internal/migrations` vs the real migrations directory); correct it and regenerate.
7. **Remove organization-specific code** (the relocating organization owns this; core
   only removes it):
   - `internal/infobio/` (external-system client + handlers)
   - `internal/fingerprintcase/importer.go` (+ test) — keep `handler.go` (generic
     `criminal_cases` listing) in core
   - `cmd/import-fingerprint-cases/`, `cmd/import-latentes-graph/`
   - `cmd/trackid/services.go` organization-service wiring
   - `internal/config/config.go` external-system fields
   - `internal/api/router.go` external routes
   - `internal/storage/client.go` organization-specific object-key helper
   - `data/` sample dataset
   - frontend `Toolkit/` pages and their service modules
   - `scripts/start_dev.sh` external-system env references
8. **Generic DB-driven cluster logic.** New `cluster` package reads comparison records
   from Postgres, groups by modality, and writes `BiometricCluster` nodes (reusing
   migration `003_biometric_cluster.cypher`).
9. **Schema generalization.** Identity tables carry fields tied to one organization's
   enrollment system (document identifier, NIST/storage paths). Generalize: keep a
   stable `meta JSONB` for organization-specific attributes, drop the
   organization-specific identifier index from `001_init.cypher`.

## Cleanup

- `db/queries.sql` references an `infobio_enrollments` table that no migration ever
  creates — dead; remove the queries and regenerate sqlc.
- `README.md` still describes a "Rust backend"; rewrite for the Go package.
- `autonid/` (unrelated production scripts) and `.claude/` (local editor config) should
  not be part of the package; remove or move out.
- `.env.example` references a `sync-*` command that does not exist here; the
  organization owns it.

## Phases

### Phase 1 — Module restructure
- Move `backend/*` to repo root; set module path `github.com/rodrigorfcm/trackid`.
- Fix import paths and `sqlc.yaml` schema path; `go mod tidy`.
- Verify: `go build ./...` and `go test ./...` pass.

### Phase 2 — Public API surface
- Move `internal/{api,auth,config,database,db,graph,storage}` to public packages.
- Split `config`; genericize `api.NewRouter` with a route-registration hook.
- Embed SQL + Cypher migrations.
- Verify: `go build ./...`, `go test ./...`, and `cmd/migrate up` against a fresh DB.

### Phase 3 — Remove organization-specific code
- Delete the external-system client, CSV importer, organization import commands,
  organization routes/config/helpers, toolkit pages, and sample data.
- Keep the generic `criminal_cases` listing handler in core.
- Verify: `go build ./...`, `go test ./...`, `npm run build` and `npm run lint` in
  `frontend/`.

### Phase 4 — Generic DB-driven logics
- Implement `cluster` (biometric clustering by modality) and `graph` sync reading
  Postgres.
- Add/confirm core evidence + biometric-feature tables so logic is DB-driven.
- Verify: `go test ./...` plus an end-to-end run against the dev stack
  (`scripts/start_dev.sh`, import a sample, run clustering, inspect Neo4j).

### Phase 5 — Schema generalization & comparisons table
- Extract comparisons into a normalized `comparisons` table
  (`evidence_a`, `evidence_b`, `case_type`, `comparison_type`, `responsible_user`);
  drop the comparison columns (`comparison_type`, `related_reference`,
  `related_reference_kind`) from `criminal_cases`, which stays the evidence identity.
- Update `graph.Sync` and `cluster.Run` to read from `comparisons` (joining
  `criminal_cases` for modality/description).
- Move organization-specific identity attributes into `meta JSONB`; drop the
  organization-specific index in the Cypher init migration.
- Verify: fresh `cmd/migrate up` + `graph.Apply`, then `go test ./...`.

### Phase 6 — Frontend trim
- Reduce `frontend/` to the base template (auth + layout + shared UI), removing
  organization-specific pages and services.
- Verify: `npm run build` and `npm run lint`.

### Phase 7 — Docs & release
- Rewrite `README.md` (structure, extension contract, pipeline).
- Tag a version for external consumption once stable.

## Out of scope

- The organization's distribution module, its import commands, and its external-system
  integrations (handled in the organization's own project).
- Frontend theming beyond the shared template.
- Identified-person linkage — person-reference comparisons and `IDENTIFIED_AS`
  materialization — deferred until an import path feeds the core identity tables.
