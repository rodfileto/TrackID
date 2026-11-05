# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This project is a research prototype and has not reached `1.0`. Until then the
API and the database schema may change in `0.x` releases without a deprecation
period; `0.x` patch releases fix defects without adding behaviour.

## [Unreleased]

### Added

- An occurrence's place and date are optional, for documents that give neither, and the
  `UNDETERMINED` occurrence type covers a crime whose document doesn't say which.
- `requests.RegisterInput.ConcludedAt`: an importer can register a request already answered, whose
  analysis is recorded closed, including types not yet offered in the app.
- A request can hold labelled references to other systems' numbers, and the requests list can
  search by them.
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

- Renamed `criminal_cases` to `biometric_cases` with `CRIMINAL`/`CIVIL` case
  types.
- Consolidated the migrations into a smaller set with one schema vocabulary.
- The face-match threshold and the fingerprint review band are versioned data
  rather than constants.
- A pair may hold one SYSTEM decision per threshold version; the latest
  counts. Embeddings carry a match path, and the per-face sync matches only
  `SYNC` ones.

### Removed

- Neo4j: the graph (clusters, persons) now lives entirely in Postgres.

## [0.1.0] - 2026-09-12

### Added

- First tagged snapshot: the reusable core package (the Postgres schema, the
  enrollment and evidence hierarchies, the unified `biometricfeature` table,
  decisions and clusters), the cases read API, the base frontend, face
  detection and embedding via trackid-vision, and the extension contract for
  import commands.
