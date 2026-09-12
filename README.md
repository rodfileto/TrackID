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
  - the Postgres schema (users, criminal cases, identity, comparisons) and the Neo4j
    schema (identity enrollment, forensic evidence, biometric clusters);
  - the non-import logic: authentication, the `cases` read API, the Postgres → Neo4j
    graph sync, and biometric clustering by modality;
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
                                     ├──►  Neo4j (evidence, decisions, clusters)
                                     └──►  core API ──► frontend
```

The database is the boundary between the two halves. Every logic entry point reads from
Postgres — never from an external file or third-party system — so organizations can
reuse the full core unchanged regardless of their source formats.

## Structure

- `cmd/` — command-line tools: `migrate` (Postgres schema), `trackid` (API server),
  `sync-graph` (Postgres → Neo4j), `cluster-biometrics` (biometric clustering)
- `api/` — the HTTP router: health, auth, and the core `cases` endpoint, plus a
  route-registration hook for integrations
- `auth/` — user registration/login, JWT issuance and middleware
- `cases/` — read API for the `criminal_cases` table
- `cluster/` — biometric clustering by modality, reading Postgres and writing Neo4j
- `config/`, `database/`, `env/` — configuration, connection, and dotenv loading
- `db/` — sqlc queries and models for the core Postgres schema (`queries.sql`) and the
  goose migrations (`db/migrations`)
- `graph/` — embedded Neo4j (Cypher) schema migrations, their applier, and the
  Postgres → Neo4j sync of evidence and decisions
- `storage/` — MinIO/S3 object-storage client
- `frontend/` — React + Vite, based on the [TailAdmin React](https://github.com/TailAdmin/free-react-tailwind-admin-dashboard)
  template
- `quarto/` — technical report and ontology documentation
- `scripts/` — development helpers (start/stop the stack, apply the graph schema)
- `docker-compose.yml` — Postgres, Redis, MinIO, and optional Neo4j

## Extension contract

To integrate with this package, an organization:

1. depends on `github.com/rodrigorfcm/trackid`;
2. writes import commands that read its own formats and write into the core tables —
   `criminal_cases` (evidence items) and `comparisons` (evidence-to-evidence edges),
   via the queries declared in `db/queries.sql`;
3. optionally registers extra routes through the `api.Dependencies.Register` hook and
   copies the base frontend to add its own pages.

## Development

```sh
scripts/start_dev.sh   # starts the stack, migrates, and runs the API + frontend
```
