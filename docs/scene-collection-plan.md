# Where material comes from: requests, and scene collection

Decided 2026-11-08. It builds on "Request first" (`docs/facial-reports-plan.md`) and on
`docs/request-routing-plan.md`, whose flow is **occurrence → request → case**.

## The problem

A file can carry only one origin: the request that sent it in (`case_files.case_request_id`).
Anything else has no origin at all.

- **"New case"** (`POST /api/cases`, open to any signed-in user) creates an empty case of any
  modality. Its evidence is then uploaded on the case page (`POST /cases/:caseId/evidences`),
  with no origin.
- **A later request** joins the case and ticks the case's existing files as "sent in by this
  request" (`requests.AttachFiles`). Evidence a forensic examiner collected at a scene then reads
  as if the ofício had sent it. That is wrong in the record and would be wrong in a laudo.
- **No seal numbers** are recorded anywhere, for material received or collected.

## How the work really arrives

- **Facial exams** come only from a request. The ofício sends the material (CCTV, photos), and
  the case is created from it.
- **Fingerprint comparisons** come only from a request too.
- **The scene is the one exception.** The examiner goes to the scene on call, collects vestiges
  and lifts latent prints there. The request that formalizes the scene examination usually
  arrives later.
- **A request is often general** ("examine the scene"). What follows depends on what the scene
  gives: nothing, lifts to compare, objects for the lab, or both. The unit opens those
  follow-ups; the request does not name them.
- **Units differ.** Some have separate sectors for the scene, the lab and comparison, each with
  its own laudo. In others one professional does the whole cycle and issues one laudo.

## A case is the material of one modality

A biometric case stays what it is: the material of one modality (facial or fingerprint) from one
occurrence. Its analyses are the tasks, each with its type and the request it answers
(`case_analyses.case_request_id`). One request can lead to several analyses in the same case,
opened one after another as results come in:

```
Request (general: "scene examination")
 └─ Fingerprint case
      ├─ Collection (on call, before the request)
      ├─ CRIME_SCENE_EXAMINATION ── answers the request        (later: not built yet)
      ├─ FINGERPRINT_LAB_EXAMINATION ── when objects need it   (later: not built yet)
      └─ FINGERPRINT_COMPARISON ── when there are latents to compare
```

Rejected: a case per analysis or per request (it splits the material, and a comparison could no
longer see the latents a lab exam developed); a case for the whole occurrence with the modality
moved to the analyses (modality is read in 56 Go files, for little gain, since the occurrence
already groups its cases).

## Decisions

1. **Cases are created from a request, or from a scene collection. Nothing else.**
   - Facial cases and fingerprint comparisons: only through registering a request
     (`requests.Register`), as today.
   - The "New case" button and `POST /api/cases` go away. This withdraws decision 5 of
     "Request first" ("New case without a request stays for internal or intelligence work").
   - Importers are not affected: they call `cases.Create` or `cases.Ingest` from Go
     (trackid-sim's `load`, trackid-pf's `laudolocal`), not the HTTP route.
2. **A scene collection creates a CRIMINAL FINGERPRINT case.** One form, saved in one
   transaction: the occurrence (found or created, as on the request form), the collection (who,
   when, where, notes), its vestiges with their seal numbers, then the files.
   - Coordinators and fingerprint examiners (users who examine the FINGERPRINT modality,
     `user.Examines`) can register one. Examiners then need the occurrence form's routes
     (`RequireOccurrenceForm`), which today are for coordinators and intelligence.
   - A scene where nothing was collected is registered too, saying so
     (`nothing_collected`). Its laudo will describe the scene and say nothing was found.
3. **A collection waits for its request.** `case_collections.case_request_id` is empty until the
   request that formalizes the scene arrives. The coordinator registers it as usual, joining
   the scene's case, and names the collection it formalizes. Collections waiting for their
   request are listed for coordinators. When the scene analysis is built, it answers that same
   request.
4. **Follow-ups answer the request that started the chain.** A comparison (and later a lab exam)
   is opened on the case when there is something to examine, answering the same request
   (`analyses.OpenWith` with `RequestID`, as today). Coordinators and fingerprint examiners can
   open one. An examiner who opens a comparison takes its examination; its verification is
   distributed by the coordinator, as now.
5. **Reports together or separate: chosen when the case is opened.** Whoever opens the case (the
   coordinator registering the request, or whoever registers the scene collection) chooses
   whether its analyses are reported in one laudo or each in its own
   (`biometric_cases.report_mode`, `SEPARATE` or `TOGETHER`). It can be changed until the case's
   first laudo is issued.
   - It is recorded now and takes effect with the scene laudo (see "Later"). Until then, every
     laudo is issued per analysis, as today.
6. **Every piece of material has one origin.**
   - `RECEIVED`: sent in by a request (`case_request_id`), as today.
   - `COLLECTED`: collected at a scene (`collection_id`).
   - Files derived inside TrackID (face crops, codification images, frames captured from a
     video, reports) keep pointing at their source, as now; they have no origin of their own.
   - Uploading evidence or video on the case page must name its origin: a request of the case,
     or a collection of the case.
   - Existing rows keep what they have. Files with neither stay "origin not recorded", shown as
     such. The database allows that for imports; the API refuses it for new uploads.
7. **A request does not claim material it did not send.** `requests.AttachFiles` refuses
   collected files. A request on a case lists the vestiges its questions are about
   (`case_request_vestiges`) instead.
8. **Vestiges and seals.** A vestige is a physical item, received or collected: a lift card, an
   envelope of lifts, a seized object, a pen drive. It carries its origin and its seals.
   - A vestige can have several seals over time. The lab opens a seal to examine, and seals
     again with a new number. Each seal records its number, who applied it and when, and who
     broke it and when. At most one seal is unbroken.
   - Seal numbers are searchable but not unique: numbering differs between issuers.
   - Files point at the vestige they were taken from (`case_files.vestige_id`), so a scanned lift
     card shows its vestige and seal, and a video its pen drive.
   - This covers lab exams too: a request's sealed material is received as vestiges with their
     seal numbers.

## Schema (migration 019)

As built (`db/migrations/019_scene_collection.sql`), two changes from the sketch below: the
collection has no `occurrence_id` of its own, since it is the case's (`biometric_cases.occurrence_id`);
and `vestige_id` is on `case_files`, not `case_evidences`, so videos have it too.

```sql
ALTER TABLE biometric_cases ADD COLUMN report_mode TEXT NOT NULL DEFAULT 'SEPARATE'
    CHECK (report_mode IN ('SEPARATE', 'TOGETHER'));

CREATE TABLE case_collections (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,              -- "Collection 1, 2, ..." in the case
    occurrence_id BIGINT REFERENCES occurrences(id) ON DELETE SET NULL,
    location_id BIGINT REFERENCES locations(id), -- the scene, when it differs from the occurrence's
    collected_from TIMESTAMPTZ NOT NULL,
    collected_to TIMESTAMPTZ CHECK (collected_to >= collected_from),
    collected_by BIGINT NOT NULL REFERENCES users(id),
    nothing_collected BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT,
    case_request_id BIGINT REFERENCES case_requests(id) ON DELETE SET NULL, -- NULL: awaiting request
    registered_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (biometric_case_id, sequence)
);

CREATE TABLE case_vestiges (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,              -- "V1, V2, ..." in the case
    description TEXT NOT NULL,
    case_request_id BIGINT REFERENCES case_requests(id) ON DELETE RESTRICT,
    collection_id BIGINT REFERENCES case_collections(id) ON DELETE RESTRICT,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (biometric_case_id, sequence),
    CHECK ((case_request_id IS NULL) <> (collection_id IS NULL)) -- exactly one origin
);

CREATE TABLE case_vestige_seals (
    id BIGSERIAL PRIMARY KEY,
    vestige_id BIGINT NOT NULL REFERENCES case_vestiges(id) ON DELETE CASCADE,
    seal_number TEXT NOT NULL CHECK (seal_number <> ''),
    applied_at TIMESTAMPTZ NOT NULL,
    applied_by BIGINT REFERENCES users(id), -- NULL when applied outside (the requesting agency)
    broken_at TIMESTAMPTZ,
    broken_by BIGINT REFERENCES users(id),
    CHECK ((broken_at IS NULL) = (broken_by IS NULL))
);
CREATE UNIQUE INDEX case_vestige_seals_one_unbroken ON case_vestige_seals (vestige_id) WHERE broken_at IS NULL;
CREATE INDEX case_vestige_seals_number_idx ON case_vestige_seals (seal_number);

CREATE TABLE case_request_vestiges (
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    vestige_id BIGINT NOT NULL REFERENCES case_vestiges(id) ON DELETE CASCADE,
    PRIMARY KEY (case_request_id, vestige_id)
);

ALTER TABLE case_files ADD COLUMN collection_id BIGINT REFERENCES case_collections(id) ON DELETE RESTRICT;
ALTER TABLE case_files ADD CONSTRAINT case_files_one_origin_check
    CHECK (case_request_id IS NULL OR collection_id IS NULL);
ALTER TABLE case_evidences ADD COLUMN vestige_id BIGINT REFERENCES case_vestiges(id) ON DELETE SET NULL;
```

A vestige's and a file's origin are `RESTRICT`: deleting a request or a collection that material
came from is refused rather than leaving material with no origin.

## Phases

**Status (2026-11-08): phases 1 and 2 done, on branch `scene-collection`.** Checked with DB tests
(`collections/collections_db_test.go`: a scene then its request, one origin per file, seals, a
scene with nothing collected and the report mode; `api/collections_db_test.go`: who registers a
scene, what the collector may do, the follow-up comparison) and the full core and trackid-pf
suites; the frontend type-checks, lints and builds. Not yet seen in a browser. Also done beyond
the list: whoever collected a case's material reads and writes the case (`distribution.Rights`,
the case list), and a frame taken from a video inherits the video's origin. Phase 3 is open.

**Phase 1: schema and backend.**
- Migration 019, with its Down.
- `collections.Register` (one transaction: occurrence, case, collection, vestiges, seals), and
  receiving files into a collection.
- Seals: break and apply, on received and collected vestiges.
- Request registration and follow-up requests: the collection a request formalizes, the vestiges
  it concerns, the case's report mode; `AttachFiles` refuses collected files.
- Opening a comparison: fingerprint examiners too, on a case they can reach.
- Evidence and video uploads on the case page require an origin.
- `POST /api/cases` removed.
- DB tests: a scene case and its later request; a request on a scene case can't claim its files;
  one unbroken seal per vestige; an examiner of another modality is refused; a scene with
  nothing collected.

**Phase 2: frontend.**
- A "Scene collection" page in place of "New case", for coordinators and fingerprint examiners.
- On the case page, a Collections card (collector, time, place, vestiges, seals, its request or
  "awaiting request"), and seals on the vestiges of received material.
- The request form: vestiges with seal numbers for sealed material; on an existing scene case,
  the collection it formalizes and the vestiges it concerns; the report mode on a new case.
- Evidence upload asks for its origin and vestige.
- A coordinators' list of collections awaiting their request.

**Phase 3: around it.**
- The comparison laudo cites each vestige's origin and seals.
- `docs/hash-chain-plan.md`: add the new tables to the material-custody group.
- `docs/MODELS.md`: the origin of material.
- trackid-sim: scene cases among the fingerprint ones, with the request arriving days later.

## Later: not in this plan

- **The scene analysis and its laudo** (`CRIME_SCENE_EXAMINATION`): request, occurrence, place,
  when, examiner, surfaces and methods, the result (vestiges with seals, or nothing found),
  photos. The legacy scene laudos trackid-pf imports (`laudolocal`) show the PF format. It will
  take over the case's collections and answer their request.
- **The lab exam** (`FINGERPRINT_LAB_EXAMINATION`).
- **One laudo for several analyses**, for `TOGETHER` cases: a link table between a report and the
  analyses it covers (each analysis in one report only), with `analysis_reports.case_analysis_id`
  kept as the analysis the report starts from; issued once every analysis it covers is done,
  with its type and number. Only useful once the scene laudo exists.

## Open points

- **Sealed media in facial cases.** A pen drive sent sealed with the ofício is a received vestige,
  and the videos on it point at it. Optional for facial evidence: material sent by digital
  transfer has no seal.
- **Where the scene is.** The collection's place defaults to the occurrence's. Its own
  `location_id` is for a scene apart from the reported place (a body found far from where the
  person was last seen, a stolen car abandoned elsewhere).
