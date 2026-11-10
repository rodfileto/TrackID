# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This project is a research prototype and has not reached `1.0`. Until then the
API and the database schema may change in `0.x` releases without a deprecation
period; `0.x` patch releases fix defects without adding behaviour.

## [Unreleased]

### Changed

- A report's number is now its issuing unit's, by discipline and year, rather than global per report type (migration
  021): `analysis_reports` keeps `issuing_unit_id` and a `discipline` derived from the type, so a facial review and
  comparison share a sequence and two units can issue the same number. A report issued in TrackID takes its case's
  unit; `reports.ImportedReport.IssuingUnitID` names another. Reports already numbered keep their numbers.

### Added

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
