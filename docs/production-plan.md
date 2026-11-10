# Production deployment

Proposed 2026-11-10. **Phase 1 is built** (2026-11-10) and was tested the same day as a separate
stack on the development machine. The test went through TLS at nginx: sign-in and its rate limit,
the WebSocket, every map page in a headless browser with no CSP violation, tiles from a server set
at runtime, and a synthetic world loaded through the internal network. The worker embedded it on
the image's CPU onnxruntime, with the same clustering results as on the GPU, and its evidence was
stored locked and came back byte for byte. **Phase 2 is built** the same day (strict production
mode, secrets from files, `/ready` and `worker -check`, trusted proxies, body limits). JSON logs
were left out. **Phase 3's workflow is written** (`.github/workflows/images.yml`) and checked
with actionlint, its CI gate against the repository's real runs, and its scan against images
built here. It publishes at the first version tag. **Phase 4 is built** the same day and was
tested on two stacks: an encrypted backup of a loaded one, restored into a fresh one, which then
served the same rows and evidence byte for byte. `verify-storage` caught a file overwritten and
another deleted with the storage keys, and both were brought back from their locked versions.
`asynqmon` was left out: it hasn't been released since 2023, so `worker -queues` reports the
queues instead. **Phase 5 is built** the same day and was tested on the development machine. The
GPU image (`Dockerfile.gpu`, `deploy/compose.gpu.yml`) ran the API and the worker on CUDA, with the
same clustering results as on the CPU. The `nominatim` profile imported an extract and answered
the API on the internal network. The `llm` profile served a model on the GPU to the API's
container. WordCraft, built for the image by `deploy/build-wordcraft.sh` and mounted with
`deploy/compose.wordcraft.yml`, rendered a PDF inside the image, and its editor started behind
nginx with no CSP violation. A WordCraft built on a newer system doesn't run in the image (it
needs glibc 2.43; the image has 2.36), which is why the script compiles it in a Debian 12
container.

The goal: an institute that has never seen the code can go from a release tag to a running,
hardened TrackID on one server of its own, by following one document and editing one
configuration file. An organization that extends core with its own server and pages, through
`server.Options` and `src/extensions/`, builds its own image the same way and swaps in its
binary and frontend.

## What is there now

- **Only a development stack.** `docker-compose.yml` runs Postgres (pgvector + PostGIS, built
  from `db/postgres`), Redis, MinIO and the SourceAFIS sidecar, with fixed dev passwords and
  every port published on the host. `scripts/start_dev.sh` runs the API, the worker and Vite
  from source with `go run` and `npm run dev`. There is no image of the application itself.
- **Two processes, plus one-off commands.** `cmd/trackid` is the API, and it serves
  `frontend/dist` (or `TRACKID_WEB_DIST`) on the same origin. `cmd/worker` handles the Asynq
  jobs (embedding, fingerprint, video, assist). `cmd/migrate up` and `cmd/create-superuser`
  run once. The realtime plan decided on **one API instance**: WebSockets live in its memory.
- **Configuration is environment only.** It is read through `internal/config` and
  `.env` / `internal/env`. Almost everything is optional, and the server degrades instead of
  failing: no S3 makes uploads return 503, no vision makes detection return 503, and an empty
  `JWT_SECRET` lets the server start while every login returns 500. That is right for
  development and risky in production, where a typo should stop the start.
- **Native and external pieces.** Vision is cgo plus `libonnxruntime.so` (CPU or CUDA build)
  and ONNX weights, which are not bundled (`docs/MODELS.md`: YuNet MIT and AuraFace Apache-2.0
  by default, SCRFD non-commercial). WordCraft (`WORDCRAFT_CLI`, `WORDCRAFT_WEB_DIR`) is a Rust
  build. The geocoder defaults to public Nominatim, and map tiles to public OSM
  (`VITE_MAP_TILE_URL`, set at **build** time).
- **Security posture is written down, not packaged.** `SECURITY.md` "What you must do when
  deploying" lists real secrets, TLS at a reverse proxy, no exposed ports, rate limiting of
  `/api/auth/login` at the proxy, threshold calibration, licence review, and backups with
  tested restores. No file in the repo does any of it.
- **Auth.** A 24-hour JWT in a Bearer header, kept by the frontend in local or session storage.
  `ActiveUser` re-checks the account on every request. There is no public sign-up.
- **Files go through the API.** The browser never talks to MinIO (no presigned URLs), so object
  storage can stay on the internal network.
- **Releases are git tags** (`RELEASING.md`). Nothing is built or published from them.

## Decisions (proposed)

1. **Target: one host, Docker Compose.** This matches the one-API-instance decision and what
   a forensic institute can run and audit. Kubernetes/Helm, high availability and multi-host
   setups are out of scope until there is a reason to add them.
2. **One application image, several commands.** A multi-stage `Dockerfile` at the repo root
   builds the frontend (node), the Go binaries (`trackid`, `worker`, `migrate`,
   `create-superuser`, with cgo), and a slim glibc runtime (Debian slim) that holds the CPU
   `libonnxruntime.so`, pinned and SHA-256-checked. Compose picks the command. It runs as a
   non-root user with a read-only root filesystem and a writable `/tmp` for the video lab.
3. **Weights are mounted, never baked in.** `/models` is a read-only volume. A
   `deploy/fetch-models.sh` downloads the default, permissively licensed set (YuNet + AuraFace)
   with checksums. SCRFD stays a deliberate, documented opt-in. This keeps the image's licence
   clean and lets an institute bring its own weights.
4. **A `deploy/` directory holds the production stack**, separate from the dev
   `docker-compose.yml`:
   - `compose.yml`: proxy, api, worker, a one-shot `migrate` that api and worker wait on
     (`service_completed_successfully`), postgres, redis (with a password), object storage
     (SeaweedFS, decision 11), and the sidecar. Everything sits on an internal network, and **only the proxy publishes a
     port** (443, plus 80 for the redirect). All images are pinned by version, never `latest`.
   - `.env.example`: production-only, with no defaults that work. Every secret is a
     `CHANGE_ME`, and the comments say how to generate each one.
   - `nginx/trackid.conf`: TLS with the institute's certificate, HTTP→HTTPS, `limit_req` on
     `/api/auth/login`, WebSocket upgrade for the realtime path, `client_max_body_size` sized
     for evidence and video, and security headers (HSTS, a CSP that fits the SPA and the
     configured tile server, `X-Content-Type-Options`, `frame-ancestors`). nginx rather than
     Caddy because rate limiting is built in and institutes mostly use internal CAs, not ACME.
   - Optional profiles, off by default: `gpu` (CUDA image variant plus the NVIDIA runtime),
     `nominatim` (self-hosted geocoder), and `llm` (a local OpenAI-compatible server for
     assist and ask).
5. **A strict production mode in the app.** `TRACKID_ENV=production` turns "degrade and log"
   into "refuse to start" for the settings a deployment can't do without: `JWT_SECRET` of at
   least 32 bytes, `DATABASE_URL`, S3, `REDIS_URL` and `APP_URL`. A feature someone actually
   enabled must also come up. For example, vision paths that are set but fail to load stop
   the start instead of logging. Development stays as forgiving as it is now.
6. **Secrets can come from files.** Each secret variable also accepts `<NAME>_FILE`, the
   Docker secrets convention, so secrets need not sit in `.env` or show in `docker inspect`.
7. **Readiness separate from liveness.** `/health` stays as is. A new `/ready` checks Postgres,
   object storage and Redis, and compose's healthcheck and the proxy use it. The worker gets a
   healthcheck too: a `worker -check` subcommand that pings Redis and the database.
8. **Release images from CI.** On a `v*` tag, a workflow builds and publishes the app,
   Postgres and sidecar images to GHCR, tagged with the version. It adds an SBOM, a
   vulnerability scan (Trivy or Grype) and a cosign signature. The `deploy/` files on a tag
   refer to that tag's images, so a deployment is "check out the tag and fill in `.env`".
9. **One operator guide, `docs/deployment.md`.** It covers requirements (CPU, RAM, disk, and
   GPU if used), install, first admin (`docker compose run --rm api create-superuser`),
   configuration reference, backups and restore, upgrades, and a checklist that replaces the
   deploy section of `SECURITY.md` (which then links to it).
10. **Extending organizations reuse the recipe, not the image.** They compile their own main
    from their own module, which core's build context can't see. So each writes a short
    Dockerfile with the same stages: frontend with its extensions, its main with cgo, onnxruntime
    checked against the same checksum, and the same runtime. It then points `deploy/` at it with
    `TRACKID_IMAGE`. `docs/deployment.md` describes this, with no organization's specifics.
11. **Object storage: SeaweedFS, or the institute's own S3.** MinIO's community edition stopped
    shipping images and binaries in October 2025, and its repository went into maintenance
    mode in December 2025 (see the trial below). Core only needs the S3 API, through the
    `minio-go` client, which is still maintained. So both the production stack and the dev
    `docker-compose.yml` move to SeaweedFS (Apache-2.0), pinned to a release at least two
    weeks old and run as one `weed server -s3` container. An institute that already runs S3
    points `S3_*` at it and leaves the service out. Backups copy the bucket with `rclone`
    instead of MinIO's `mc`.
12. **Evidence buckets can be made unchangeable (object lock).** SeaweedFS supports S3
    versioning and object lock, and core never deletes from storage. A production bucket can
    therefore be created with locking and a default COMPLIANCE retention, so that no one,
    including an administrator with the storage keys, can delete or overwrite a stored file
    for that period. A re-upload of the same content-hashed key adds a version and doesn't
    fail, and `storage.NewClient` works on such a bucket unchanged. Locking can only be
    switched on when the bucket is created, so `deploy/` creates the bucket before the first
    start instead of leaving it to `NewClient`. **Production buckets are locked by default**
    (decided 2026-11-10). The retention period is each institute's policy, so there is no
    default: `deploy/` refuses to create the bucket until `S3_RETENTION_DAYS` is set. The dev
    stack stays unlocked, so test data can be thrown away.

### Trial of SeaweedFS (2026-11-10)

Run against `chrislusf/seaweedfs:4.51` (released 2026-10-25, signed with cosign), single
container, on a separate compose project so the dev stack was untouched. Everything passed:

- **Core's `storage` client:** creates the bucket and round-trips 0 B, 10 KiB, 5 MiB and
  64 MiB objects with equal SHA-256, reads `s3://` references, and keeps the content type.
- **Versioning and object lock** (through `minio-go`): a bucket created with locking has
  versioning on, and an overwrite makes a new version while the old one stays readable.
  Deleting a version under COMPLIANCE retention or legal hold is refused, the governance
  bypass doesn't lift COMPLIANCE, and shortening a retention is refused. A plain delete only
  adds a delete marker. A bucket default retention locks new objects automatically.
- **Persistence and backup:** data survives a container restart, and an `rclone` copy of the
  bucket to disk and back into a new bucket reports 0 differences.
- **The whole application:** API and worker (vision on the GPU, SourceAFIS sidecar) ran with
  SeaweedFS as their only storage, and trackid-sim loaded an 80-person world (52 cases,
  158 objects, 31 MiB) in about a minute. The worker embedded every face and extracted the
  prints with no errors. Clustering gave facial pairwise precision 1.00 and recall 0.94, with
  0 wrong-person identifications. Evidence downloaded through
  `/api/cases/:id/evidences/:id/download` matched its `hash_id` (SHA-256) byte for byte.
- **Cost:** the image is about 200 MB, and the server used about 500 MiB of RAM at rest, more
  than MinIO but nothing on a server.

Not tested: multi-node setups, erasure coding, and behaviour with a full disk. Core supports
single-node only and points to SeaweedFS's documentation for anything bigger.

## Phases

### Phase 1: a stack that runs stock core

- Root `Dockerfile` and `.dockerignore` (decision 2); `deploy/compose.yml`,
  `deploy/.env.example`, `deploy/nginx/`, `deploy/fetch-models.sh` (3 and 4).
- SeaweedFS in place of MinIO in the dev `docker-compose.yml` as well. `backup_data.sh` and
  `restore_data.sh` move from `mc` to `rclone`, and trackid-sim's stress stack, which waits on
  MinIO's health URL, follows (decision 11). `deploy/` creates the bucket with object lock
  (decision 12).
- A self-signed certificate helper for trying it out, clearly marked as not for production.
- First version of `docs/deployment.md`.
- **Done when** a clean VM with only Docker goes from `git clone` of a tag to logging in over
  HTTPS. Synthetic data from trackid-sim is then loaded and a face search, a fingerprint match
  and a report draft all work, and nothing but 80/443 answers from outside (checked with an
  `nmap` of the host).

### Phase 2: app changes for production

- `TRACKID_ENV=production` strict startup (decision 5), `*_FILE` secrets (6), `/ready` and
  `worker -check` (7), with tests for each.
- Set gin's trusted proxies (`TRUSTED_PROXIES`, defaulting to none) so logged client IPs are
  the proxy's view, not a header anyone can set.
- Request-size limits on the upload routes that still have none, so that the proxy is not
  the only limit.
- Optionally, `log/slog` JSON logs behind `LOG_FORMAT=json` for institutes that ship logs to a
  collector.

### Phase 3: published, verifiable releases

- CI workflow for images, SBOM, scan and signature (decision 8). `RELEASING.md` gains the step,
  and each release note says whether `migrate` changes the schema (as it already should).
- Publishing images is public and outward-facing, so the first run waits for an explicit go.

### Phase 4: operations

- `deploy/backup.sh`: `pg_dump -Fc` plus a mirror of the bucket, encrypted (age or gpg), with a
  retention policy. `deploy/restore.sh`, and a documented restore drill: restore to a scratch
  stack, run `migrate`, and check the evidence SHA-256s against `features`. This replaces the
  dev helpers' role for production.
- Upgrade procedure: read the CHANGELOG, take a backup, change the image tag, run `migrate`,
  restart, and check `/ready`. Rollback means restoring the backup, since migrations are
  forward-only.
- Monitoring in the guide: disk on the Postgres and object-storage volumes, Asynq queue depth
  and failed tasks (`asynqmon` as an optional internal-only service), certificate expiry.

### Phase 5: optional pieces

- CUDA image variant (`-cuda` tag) for the `gpu` profile.
- `nominatim` and `llm` profiles with sizing notes.
- A documented GPU image build (`Dockerfile.gpu`), not published: it is about 10 GB.

WordCraft is not in the image (decided 2026-11-10, question 4). A deployment that edits and
issues reports builds WordCraft itself and mounts the build into the API container
(`docs/deployment.md`, "Reports").

## Not in scope

Kubernetes/Helm, more than one API instance, SSO/LDAP/OIDC login, moving the token out of
browser storage, and in-app login rate limiting. Each is worth its own plan, and SSO is
probably the first an institute will ask for.

## Questions

1. *Decided:* SeaweedFS replaces MinIO (decision 11), and production buckets are locked with a
   retention period each institute sets (decision 12).
2. *Decided 2026-11-10:* public, signed images on GHCR from CI on each `v*` tag (decision 8).
   `deploy/compose.yml` refers to `image: ghcr.io/rodfileto/trackid:<tag>`.
3. *Decided 2026-11-10:* runtime frontend configuration. The API serves `/api/config` from
   environment variables (map tile URL and attribution to start with), and the frontend reads
   it at startup instead of the build-time `VITE_MAP_TILE_*`. One image then fits every
   deployment. This goes in phase 1, since the published image depends on it.
4. *Decided 2026-11-10:* WordCraft stays out of the image. Each deployment that issues reports
   builds it, at the revision and with the language data it wants, and mounts it.
5. *Decided 2026-11-10:* a documented GPU build (`Dockerfile.gpu`) on top of the published CPU
   image, not a published GPU image.
