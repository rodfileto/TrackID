# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This project is a research prototype and has not reached `1.0`. Until then the
API and the database schema may change in `0.x` releases without a deprecation
period; `0.x` patch releases fix defects without adding behaviour.

## [Unreleased]

### Changed

- `golang.org/x/net` 0.62.0, for CVE-2026-78661 (HTML parsing; TrackID doesn't call the parser, but every binary
  carried it). The three images take their distribution's security updates when they are built.
- The fingerprint sidecar runs Jackson 2.18.11 instead of the 2.15.3 SourceAFIS 3.18.1 names, for the
  vulnerabilities fixed since, on a pinned JRE (Temurin 21.0.13). The Postgres image rebuilds `gosu` 1.20 with a
  current Go, since the base image's build carried an old standard library.
- `storage.Client.Download` is built on the new `Open`, which streams, and `Digest`, which hashes a stored file
  without holding it in memory; a missing object or file is `storage.ErrNotFound`. CI runs the S3 tests against
  the dev stack's SeaweedFS.

- Object storage is SeaweedFS instead of MinIO, whose community edition no longer publishes images
  (`docs/production-plan.md`, decision 11). The dev stack's `minio` service is now `seaweedfs`, on the same host
  port and keys, so `.env` doesn't change. Its data doesn't move by itself:
  `scripts/migrate_minio_to_seaweedfs.sh` copies the old volume's buckets across and leaves the volume in place.
  `scripts/backup_data.sh` and `restore_data.sh` copy the bucket with rclone instead of MinIO's `mc`.
- The map's tile server is read at runtime from `/api/config` (`MAP_TILE_URL`, `MAP_TILE_ATTRIBUTION` on the
  server), so one frontend build fits every deployment. `VITE_MAP_TILE_*` still sets a build's default.
- The Outfit font is bundled (`@fontsource-variable/outfit`) instead of loaded from Google Fonts, so the app's pages
  ask nothing of an outside server.
- The Postgres image is pinned to pgvector 0.8.7 (`db/postgres/Dockerfile`).

- A report's number is now its issuing unit's, by discipline and year, rather than global per report type (migration
  021): `analysis_reports` keeps `issuing_unit_id` and a `discipline` derived from the type, so a facial review and
  comparison share a sequence and two units can issue the same number. A report issued in TrackID takes its case's
  unit; `reports.ImportedReport.IssuingUnitID` names another. Reports already numbered keep their numbers.

### Added

- Optional pieces for a deployment (`docs/production-plan.md` phase 5, `docs/deployment.md`). `Dockerfile.gpu`
  builds a CUDA variant of a release's image, and `deploy/compose.gpu.yml` runs the API, the worker and the language
  model on the GPU. The `nominatim` and `llm` compose profiles run a Nominatim and an Ollama inside the stack.
  `deploy/build-wordcraft.sh` builds WordCraft for the image (its CLI in a Debian 12 container, since a newer
  system's build needs a newer glibc), `deploy/compose.wordcraft.yml` mounts it, and nginx serves its editor at
  `/wordcraft/` with a policy that allows its inline start-up script.

- Geocoding on IBGE's census address list (`docs/geocoding-plan.md` phase 1). `cmd/load-cnefe` loads a state's
  CNEFE (Censo 2022) into TrackID's database (migration 024), and `scripts/load_cnefe.sh <UF>` fetches and loads one
  in development. `geocode.Layered`, now the API's geocoder, searches the list (number, nearest number, quadra/lote),
  then Nominatim, then the municipality's point from core's own table, keeping the first result that agrees with the
  municipality and state the address names. Results carry `flags` saying how sure they are (`NEAREST_NUMBER`,
  `STREET_ADJUSTED`, `LOCATOR_PARTIAL`, `STREET_ONLY`, `MUNICIPALITY_ONLY`, `NAMED_PLACE`, `SOURCES_DISAGREE`).
  `geocode.ParseAddress` reads Brazilian addresses for all 27 states.

- Backups and restores for a deployment (`docs/production-plan.md` phase 4). `deploy/backup.sh` dumps the database,
  encrypted to an age key when `BACKUP_AGE_RECIPIENT` is set, then copies the evidence bucket into a mirror that
  each run adds to and checks, and keeps `BACKUP_KEEP` dumps. `deploy/restore.sh` restores one into a stack and
  ends by checking every stored file. `storagecheck` and `cmd/verify-storage` read every file the database refers
  to and check it is there and, for case files, that its SHA-256 is the one recorded: after a restore, and from
  time to time, to catch a file deleted or overwritten in storage. `worker -queues` shows each task queue's state.

- Release images (`.github/workflows/images.yml`): a version tag publishes `trackid`, `trackid-postgres` and
  `trackid-sourceafis` to ghcr.io once CI has passed on the tagged commit and Trivy finds no fixable HIGH or
  CRITICAL vulnerability, each with an SBOM and provenance and signed with cosign, keyless. Main and pull requests
  build and scan them without publishing. `RELEASING.md` and `docs/deployment.md` say how to release and how to
  check a signature; `docs/MODELS.md` lists the third-party software in each image.

- Production mode (`TRACKID_ENV=production`, `docs/production-plan.md` phase 2): the API and the worker refuse to
  start without the database, Redis and object storage, a JWT_SECRET of 32 bytes or more and an https APP_URL
  (`config.CheckProduction`), and stop when an integration they were configured with doesn't come up
  (`config.Unavailable`), instead of running without it. Development is unchanged. `deploy/compose.yml` sets it.
- Secrets from files: each of `env.Secrets` may be given as `<NAME>_FILE`, the path of a file holding it, as Docker
  and Kubernetes secrets provide them.
- `GET /ready`, which checks the database, object storage and the task queue (`/health` stays liveness only), and
  `worker -check`, the worker's healthcheck.
- A request body limit on every route (`api.DefaultMaxBodyBytes`, 128 MiB), except those that take videos, which
  keep `video.MaxBytes`. A test fails if a route that takes videos is left out.
- `TRUSTED_PROXIES`: the API believes `X-Forwarded-For` only from these addresses, rather than gin's default of
  any.

- A production deployment (`docs/deployment.md`, `docs/production-plan.md` phase 1): an application image
  (`Dockerfile`: every command under `cmd/`, the frontend, and onnxruntime checked against its SHA-256) and
  `deploy/` with nginx (TLS, a rate limit on sign-in, security headers), the app services, and Postgres, Redis,
  SeaweedFS and the fingerprint sidecar on a network with no route outside. Evidence is stored with S3 object lock:
  `cmd/init-storage` (`storage.EnsureLockedBucket`) creates the bucket in COMPLIANCE mode with the retention the
  institute sets (`S3_RETENTION_DAYS`, no default), and refuses an existing bucket without lock.

- Live notifications (`docs/realtime-plan.md`): a bell in the header tells examiners, without reloading, when an
  analysis step is assigned to them, manually or by distribution. Rows are written in the assignment's transaction
  (`notifications`, migration 022) and reach the browser over a WebSocket (`realtime`, `/api/realtime`) opened with a
  one-time ticket; Postgres `LISTEN/NOTIFY` carries them across processes, so this needs no Redis. What a notification
  says is built when it is read, under the reader's current access, and shows no case identity during blind work.
  The API now also shuts down cleanly on SIGINT and SIGTERM.
- Searching TrackID from a case's face at a threshold the analyst picks (intelligence analysts; `POST
  /api/cases/:caseId/traces/:traceId/face-search`, "Search in TrackID" on the facial comparison panes): the enrolled
  persons and other cases' faces at or above it, from the face's stored embedding, the face itself left out. It
  starts at the matcher's review threshold and shows the matcher's own; it decides nothing, and is logged as a face
  search on the case. The person lookup's result tables are shared for it.
- Face threshold calibration that doesn't depend on where the truth comes from (`calibration` package,
  `cmd/calibrate-face-threshold`): from a random sample of identities with two faces or more, the genuine and impostor
  score curves, the equal error rate, and the lowest threshold at each target false match rate, with the
  `match-threshold` line to set it. A `calibration.Source` says which faces are one person; `IdentitySource` reads
  the identity tables, configured by the field that marks one person (`person`, `fiscal`, `name_birth`) and the
  document types trusted. It writes nothing.
- Withdrawing an enrolled register from matching (migration 020): `identity.WithdrawRegister` marks its KNOWN
  features withdrawn, with who and why, and deletes their embeddings, so face search and matching no longer find
  them and nothing embeds them again (the embedding job, the backfill and the bulk vectorizer skip them). The
  AUTOMATIC cluster layer drops the links only a SYSTEM decision made to a withdrawn feature; links people confirmed
  stay in both layers until people undo them. The identity's rows stay. `identity.RestoreRegister` undoes it and asks
  for the embeddings again. For enrolled photos that came from an outside registry because someone confirmed them,
  and were later found to be someone else.
- `Spinner` (`components/ui/spinner/Spinner.tsx`): a turning ring for work under way, in three sizes, announced to
  screen readers when given a label.
- Scene collection (`docs/scene-collection-plan.md`, migration 019): a coordinator or a fingerprint examiner
  registers a scene examined on call (`POST /api/collections`, the "Scene collection" page), which opens a
  fingerprint case before any request, with its vestiges and their seals. The collection awaits the request that
  formalizes it, which joins the case and may ask no task yet; coordinators list the scenes awaiting one. The
  collector works the case without holding an analysis, and can open the comparison that follows, answering that
  request.
- Vestiges and seals: the physical items of a case, received with a request or collected at a scene, with every
  seal applied and broken (`case_vestiges`, `case_vestige_seals`). A request names the vestiges its questions are
  about. Each case records whether its analyses are reported in one laudo or each in its own (`report_mode`); the
  laudo that covers several comes later.
- Panels an organization can add to the case, person and cluster pages (`Extension.casePanels`, `personPanels`,
  `clusterPanels`), shown to intelligence analysts only.
- Organizations and units (`docs/organizations-plan.md`, phase 1, no pages or access limits yet): a tree of units
  per organization, with an optional state and municipality. Every case has a unit, given to `cases.Ingest` or
  else the one that receives new cases, and a request can name the unit that asks.
- Intelligence operations (`docs/operations-plan.md`): named lines of work with a stable slug, listed, created
  and edited by analysts at `/intelligence/operations`. An organization can attach its own panel to one
  (`Extension.operationPanels`). Linking cases, persons, occurrences and clusters comes next.
- Identity conflicts (`cluster.ListConflicts`): one biometric under different names, or one identity with different
  biometrics, flagged with their evidence on the Clusters list, the cluster page and the person page, and in the AI mode.
  They replace the cluster page's older warnings, which also fired on two documents under one name.
- AI mode (`/intelligence/ask`, `docs/ask-plan.md`): analysts ask about the criminal scenario in plain
  language, or follow a person through cases, occurrences, inquiries and nearby occurrences by conversation, and a
  language model answers through fixed read-only tools, showing what they returned and flagging numbers no query
  returned. Biometric links are labelled reviewed or lead, and `person_identities` reports the biometric evidence that a
  person may hold another identity, for an examiner to review. Any server speaking the OpenAI protocol works
  (`ASK_ENDPOINT`, `ASK_MODEL`, `ASK_API_KEY`); one outside the institute only ever reads synthetic data.
  Each question is recorded in `ask_queries`, and reads of persons and cases in `access_log`.
- An occurrence's place and date are optional, for documents that give neither, and the
  `UNDETERMINED` occurrence type covers a crime whose document doesn't say which.
- `requests.RegisterInput.ConcludedAt`: an importer can register a request already answered, whose
  analysis is recorded closed, including types not yet offered in the app.
- A request can hold labelled references to other systems' numbers, and the requests list can
  search by them.
- `requests.Update`, `requests.MoveToCase` and `occurrences.Update`, for correcting what was
  registered.
- `occurrences.SetPlace` gives an occurrence its place once known.
- `reports.RecordImported`: a report issued elsewhere becomes an analysis's issued one, its document
  the original PDF.
- An organization can run its own server (`server.Run` with `Options.Register`) and add pages, menu
  items and translations through `frontend/src/extensions/index.tsx`.
- Examination workflow: analyses are distributed to examiners in rounds, the
  second examiner works blind, and a disagreement goes to a third; assignments
  are an append-only history.
- Access profiles (examiner, coordinator, intelligence analyst, admin), an
  access log, and a users page for account management.
- Occurrences and locations, a Nominatim geocoder, and an intelligence map.
- SourceAFIS fingerprint matching behind a sidecar service, with a versioned
  review band.
- Facial reports: the authority's request (*ofício*) registered first, lead
  reports and *laudos* drafted as `.docx`, edited in the browser via WordCraft,
  and issued to PDF — both files kept with their SHA-256.
- Fingerprint image lab: levels, inversion, colour channel, lighting
  correction, CLAHE and Gabor enhancement rendered at 500 ppi, with a
  processing record for the laudo.
- Face lab: one-to-one facial comparison following FISWG's opinion scale.
- Request routing to tasks, and inquiry (IPL) numbers kept apart from police
  report numbers.
- Video face tracks and frames taken as evidence with their time.
- Large-gallery load and deduplication in two commands. `bulk-enroll` loads a
  JSON Lines register and vectorizes its photos, recording an outcome per
  photo and writing FP16 vector shards. `bulk-match` matches in batches
  (exact, on the GPU, through a FAISS tool under `tools/bulk-match`), keeps a
  score store, calibrates from identity records and examiners' decisions, and
  applies threshold versions by re-banding the stored scores.
- Storage references to files kept outside the bucket (`s3://`, and
  `file://` inside `STORAGE_FILE_ROOTS`).

### Changed

- Every uploaded file says where it came from: one of the case's requests or one of its scene collections, and
  the vestige it was taken from. A request can no longer claim a file a collection gathered, nor one another
  request sent. Frames taken from a video come from where the video did. Fingerprint examiners reach the
  occurrence form's lookups.
- Renamed `criminal_cases` to `biometric_cases` with `CRIMINAL`/`CIVIL` case
  types.
- Consolidated the migrations into a smaller set with one schema vocabulary.
- The face-match threshold and the fingerprint review band are versioned data
  rather than constants.
- A pair may hold one SYSTEM decision per threshold version; the latest
  counts. Embeddings carry a match path, and the per-face sync matches only
  `SYNC` ones.

### Removed

- Opening a case by itself (`POST /api/cases`, the "New case" button): a case is opened by registering a request,
  or a scene examined on call. Import commands still create cases with `cases.Create` and `cases.Ingest`.
- Neo4j: the graph (clusters, persons) now lives entirely in Postgres.

## [0.1.0] - 2026-09-12

### Added

- First tagged snapshot: the reusable core package (the Postgres schema, the
  enrollment and evidence hierarchies, the unified `biometricfeature` table,
  decisions and clusters), the cases read API, the base frontend, face
  detection and embedding via trackid-vision, and the extension contract for
  import commands.
