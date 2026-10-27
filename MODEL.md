# TrackID — Forensic Data Model

This document explains the core data model: how forensic biometric data (fingerprints,
faces) and forensic intelligence are structured in Postgres. It is the
specification to implement against — the evidence hierarchy and the biometric-feature
concept are the heart of the package.

## 1. The unifying concept: `BiometricFeature`

Everything in the system reduces to one atomic thing that gets compared: a **biometric
feature**. A feature is *one* biometric sample — a fingerprint template, an enrolled face
photo, a latent print lifted from a scene, a face detected in evidence footage.

Every feature has:

- `feature_type` — the modality and granularity:
  - `FINGERPRINT_TEMPLATE` — an enrolled ten-print (a full set), **KNOWN**
  - `FACE_RECORD` — an enrolled face photo, **KNOWN**
  - `FINGERPRINT_LIFT` — a latent print from a scene, **QUESTIONED**
  - `FACE_CAPTURE` — a face detected in evidence, **QUESTIONED**
- `provenance` — `KNOWN` (came from an enrollment) vs `QUESTIONED` (came from crime-scene
  evidence). Provenance is derivable from the feature's source, but stored explicitly.

Two mirrored hierarchies produce features. A **decision** (match/no-match) is always made
between *two features*. A **cluster** is a group of same-modality features believed to
belong to one person.

## 2. The two hierarchies

### 2.1 Enrollment hierarchy (KNOWN)

```
person ──< identity_document ──< identity_register ──< identity_file ── biometricfeature
```

| Table | Role |
| --- | --- |
| `person` | the real individual (stable business key `person_id`) |
| `identity_document` | an identity document (e.g. an ID card, an ABIS enrollment record); optionally carries the holder's government-issued fiscal/taxpayer number |
| `identity_register` | one enrollment event under a document (biographic data: name, parents, birthdate) |
| `identity_file` | a raw file produced by that enrollment: `photo` (face), `nist` (ten-print), `pdf`, … |
| `biometricfeature` | the typed feature(s) extracted from a file (a NIST file → one `FINGERPRINT_TEMPLATE`; a photo → one `FACE_RECORD`) |

A person can hold many documents; a document can hold many registers; a register can hold
many files; each file yields one or more features.

### 2.2 Evidence hierarchy (QUESTIONED)

```
case_requests ──< case_request_questions
case_requests ──< case_request_cases >── biometric_cases   (a request opens or joins its cases)
biometric_cases ──< case_files (>── case_requests: the material a request sent in)
biometric_cases ──< case_evidences ──< case_traces ──< case_codifications
case_files (video) ── case_videos;  case_evidences >── case_files (video): a frame's source
case_traces ── biometricfeature
```

| Table | Role |
| --- | --- |
| `biometric_cases` | the base case (`case_id`; `case_type` = `CRIMINAL` \| `CIVIL`; `modality` = `FACIAL` \| `FINGERPRINT`) |
| `case_requests` | what cases answer: an authority's request (ofício) with its document, dates, the occurrence or inquiry number it cites (and the occurrence it resolved to), and its questions (`case_request_questions`, quesitos). Registered first: it finds or creates its occurrence and opens or joins its cases, one per exam asked (`case_request_cases`, which also numbers it per case); `requests` package |
| `case_files` | raw files attached to a case (`evidence`, `video`, `document`, `forensic_report`, `report_draft`, `face_crop`, `codification_image`), with their SHA-256; `case_request_id` marks the material a request sent in |
| `case_videos` | a video sent in and its processing: the tracks of faces trackid-vision's `videoproc` found (`video` package) |
| `case_evidences` | one evidence item within a case (e.g. a lift card, an image); a frame taken from a video records the video, its time and frame number (`source_*`) |
| `case_traces` | one biometric trace within an evidence (one face in an image, one lift on a card); its `trace_type` is the feature type it yields, `FACE_CAPTURE` or `FINGERPRINT_LIFT`; its `trace_role` is `QUESTIONED` (to identify) or `REFERENCE` (known material sent in for comparison); a questioned trace carries the examiner's `suitability` (`SUITABLE`, `LEAD_ONLY`, `UNSUITABLE`) |
| `case_codifications` | one processed encoding of a trace (e.g. a minutiae set, an embedding) |
| `biometricfeature` | the typed QUESTIONED feature for a trace |

A biometric case is any case with questioned biometric material to identify. `case_type`
is its legal nature: `CRIMINAL` (e.g. latent prints from a crime scene) or `CIVIL`
(non-criminal identification such as disaster victim identification or unidentified dead
bodies). `modality` is the biometric the case works with. The two are independent, and
the rest of the hierarchy is the same for both case types.

A request is not the occurrence: the occurrence is the event, a request is an administrative act
about it, one occurrence can lead to several, and a civil case has a request but no occurrence.
`requests.Create` links a request to the occurrence its cited number names exactly, and the case
too when it has none. Every analysis answers a request (`case_analyses.case_request_id`); the
case's first request is what its automatic review answers.

A codification is a processing artifact, not a comparison record — the pairwise comparison
outcome lives entirely in `biometric_decisions` (section 3), keyed by the two features'
feature ids (section 5) rather than by codification.

### 2.3 Occurrences and locations (POLE: Event and Location)

```
locations ──< occurrences ──< biometric_cases
              occurrences ──< occurrence_classifications >── occurrence_types
```

| Table | Role |
| --- | --- |
| `locations` | a place, keyed by the importer's `location_id`: a PostGIS point (`geography`, WGS 84), how exact it is (`precision`: `EXACT` \| `GEOCODED` \| `STREET` \| `NEIGHBOURHOOD` \| `MUNICIPALITY`), IBGE municipality code, UF, address, `place_type` (`PUBLIC_ROAD`, `RESIDENCE`, `COMMERCE`, `BANK`, `GOVERNMENT_OFFICE` -- *órgão público* --, `TRANSPORT`, `RURAL`, `OTHER`) |
| `occurrences` | a registered occurrence (a police report, *boletim de ocorrência*), identified by the agency's report number, unique per `(agency, occurrence_number)`, or by a TrackID number (`occurrence_year`/`occurrence_seq`), or both: when (`occurred_from`/`occurred_to` window, reported, registered), where (`location_id`), crime-only facts (`attempted`, counts), `status`, `description` |
| `occurrence_types` | the fixed list of occurrence types, codes only (the frontend translates), following ISP-RJ's crime classification in groups `LETHAL`, `PERSON`, `ROBBERY`, `THEFT`, `FRAUD`, `DRUGS`, `CIVIL`; plus `IDENTITY_FRAUD`, `UNIDENTIFIED_LIVING_PERSON` and `DISASTER_VICTIM`, which ISP does not count but which open biometric cases; each with the `case_type` it opens. Attempts are `occurrences.attempted`, not types |
| `occurrence_classifications` | an occurrence's types (a report can carry several) |

One occurrence can open many biometric cases (a FACIAL case for its CCTV, a FINGERPRINT case
for its latents), so the link is `biometric_cases.occurrence_id`, NULL until set. An occurrence
is not necessarily a crime: as `case_type` covers CIVIL identification, the occurrence covers
the report that starts it. Deciding that two reports share a place is the importer's job:
trackid neither geocodes nor deduplicates addresses.

**Registering** an occurrence in the app goes through `occurrences.Create` (POST
`/api/occurrences`): it validates the input (types all criminal or all civil; crime-only fields
empty on a civil one; not in the future; the police agency and its report number both or
neither, unique together), numbers it `"<year>.<sequence>"` (`occurrence_year`/`occurrence_seq`,
as cases are numbered), and places it at an existing location or a new point. A
new point's `municipality_code` comes from the `municipalities` table: IBGE
boundaries loaded by `cmd/load-municipalities` from the GeoJSON `tools/municipalities` writes
with geobr. Occurrences an importer ingests keep their own (agency, number) and no TrackID number.
A case is linked from its page through `occurrences.LinkCase` (PUT
`/api/cases/:caseId/occurrence`), which refuses a criminal case on a civil occurrence or the
reverse.

Addresses and points are converted by the `geocode` package: a `Geocoder` interface with a
Nominatim (OpenStreetMap) implementation, configured by `GEOCODER_*` (see `.env.example`). Its
results are ODbL data, to be credited as "© OpenStreetMap contributors" wherever shown.

`geography` has no sqlc type, so points cross into Go as `lon`/`lat` only, and spatial
queries (`ListOccurrencesWithinRadius`) stay in SQL. This is where intelligence analysis by
place and time starts: when a cluster joins traces from different cases, their occurrences
form a series to put on a map.

## 3. Decisions (`biometric_decisions`)

An append-only event log of every decision made about a pair of features. This is the
**source of truth** for a pair's confirmation state; clusters (section 4) are a *derived
view* of it, rebuilt by `cluster.Run`.

| Column | Meaning |
| --- | --- |
| `feature_a_id`, `feature_b_id` | the two feature ids (always stored `a < b`) |
| `modality` | `FACIAL` \| `FINGERPRINT`, as in `biometric_cases` |
| `role` | `SYSTEM` (automatic), `VERIFICATOR`, `REVIEWER`, `INCONSISTENCE` |
| `decision` | `POSITIVE` \| `NEGATIVE` \| `INCONCLUSIVE` |
| `conclusion` | a comparison's level on the laudo scale (`cluster.Conclusion`): `IDENTIFICATION`, `STRONG_SUPPORT`, `MODERATE_SUPPORT`, `LIMITED_SUPPORT`, `INCONCLUSIVE`, `LIMITED_EXCLUSION`, `EXCLUSION`; it maps onto `decision` (the database's `conclusion_decision`), NULL outside a `FACIAL_COMPARISON` |
| `system_source` / `username` | who/what decided (exactly one set, per role) |
| `confidence`, `threshold` | score and the cutoff it was classified against (SYSTEM only) |
| `notes` | examiner rationale (esp. on INCONSISTENCE) |
| `case_analysis_id` | the analysis a human decision was made in (NULL for SYSTEM) |
| `decided_at` | when |

A pair progresses through roles (SYSTEM → VERIFICATOR → REVIEWER, with INCONSISTENCE only
on disagreement); the full chain matters for audit, not just the outcome. Human decisions belong
to an analysis (section 3.1), one row per role per analysis, so a pair can be decided again in a
later round. Each round is judged on its own -- round 0 is the SYSTEM decision (with any human
decisions from before analyses), then one round per analysis -- and the pair keeps the outcome of
its latest *settled* round, CONFIRMED or REJECTED (`cluster.DecidingRound`). A later round that is
still pending, disputed or ends inconclusive does not undo it: a pair a review confirmed stays
confirmed while a comparison is under way, and if the comparison lacks the quality to conclude.

The automatic matcher's cutoffs are data, not code: `match_thresholds` holds one append-only
version per row (review and confirm cutoffs, as similarities on the scale of `confidence`, with
their source), and the latest row per embedding type is current. A pair at or above the confirm
cutoff gets a SYSTEM POSITIVE, which is CONFIRMED on its own; between the two cutoffs, a SYSTEM
INCONCLUSIVE, which stays PENDING_REVIEW for an examiner — the review band; below, nothing.
Each SYSTEM decision records the cutoff it was classified against in `threshold` and cites its
version (`related_reference_kind = 'match_threshold'`). With no version set, faces use
similarity 0.6 with no band. Versions are set with `cmd/match-threshold`, from each
organization's own validation.

**Who may decide** is a user profile (`users`), the same for fingerprints and
faces. `account_role` (`SUPER_ADMIN` | `ADMIN` | `USER`) is account management only: a SUPER_ADMIN
manages admins and users, an ADMIN manages plain users, and nobody changes their own role or
deactivates themselves (`auth.CheckAccountChange`). What a user examines is separate:
`examines_fingerprint` and `examines_facial`, either, both or neither (`UserProfile.Examines`).
Within a modality they examine, a user may take any of the three human steps (VERIFICATOR,
REVIEWER, INCONSISTENCE), with independence -- never two steps on the same pair -- to be checked
where human decisions are recorded. Users are
deactivated (`is_active`), never deleted, and a username never changes, because decision rows
name their decider by username. The first SUPER_ADMIN comes from `cmd/create-superuser`; there is
no public sign-up.

### 3.1 Analyses, distribution and reports

A biometric case is worked through **analyses** (`case_analyses`; `analyses`
package): rounds of examination, each of a type that fixes its modality, its methodology and the
report it leads to. For faces, following FISWG:

| Type | What | Opened | Report |
| --- | --- | --- | --- |
| `FACIAL_REVIEW` | one-to-many: the facial recognition system's candidates for the case's faces; an investigative lead or a negative result, not a forensic examination | automatically, one per FACIAL case (`analyses.OpenAutomatic`, from `cases.Create` and `cases.Ingest`) | `FACIAL_REVIEW_REPORT` |
| `FACIAL_COMPARISON` | one-to-one forensic examination of specific pairs (`case_analysis_pairs`), each with a face of the case | by a coordinator (`analyses.Open`) | `FACIAL_COMPARISON_REPORT` (laudo) |

Fingerprint analyses (crime scene, laboratory, documents) are not modelled yet, so a FINGERPRINT
case has none. An analysis is `OPEN` until its report is issued, and answers one of the case's
requests. In a comparison each pair is decided on the conclusion scale (`biometric_decisions.conclusion`),
and the verifier must reach the same level for the pair to be settled; a different level is a
disagreement for the third examiner. The first examiner records each questioned face's
suitability while they examine it.

**Reports** (`reports` package; `docs/facial-reports-plan.md`). Each analysis has one report
(`analysis_reports`), a `DRAFT` until its examiner issues it. The server generates the draft as a
.docx from a Portuguese template and the data (`reports.Build`): an executive summary, then the
lead report's or the laudo's sections. Everything that comes from the database is locked text;
the sections the examiner writes are editable regions (`w:permStart`/`w:permEnd` in a document
protected read-only), which WordCraft's web build enforces in the analysis page and the server
checks again on every save (`analysis_report_versions`). Issuing (`reports.Service.Issue`, which
calls `analyses.IssueReport`) refuses a draft whose locked text no longer matches the data (the
examiner refreshes it, keeping their text) or a laudo with an empty exam or answer; then, in one
transaction, it numbers the report `"<year>.<sequence>"` per type, freezes the data as
`snapshot`, puts the number and date in the examiner's document, renders the PDF with
`wordcraft-cli` (the same layout as the editor) and keeps the .docx and the PDF as
`forensic_report` files with their SHA-256 (`docx_file_id`, `case_file_id`), and closes the
analysis. An issued report never changes: a laudo complementar is a new comparison whose report
`complements_report_id` the first. `report_deliveries` records where an issued report was sent.

**Stages and decisions** (`examination` package). An open analysis goes through
stages, each worked by one examiner in one step -- the `biometric_decisions` role their decisions
take:

| Stage | Step | Who | Decides |
| --- | --- | --- | --- |
| `EXAMINATION` | `VERIFICATOR` | the first examiner (FISWG's practitioner) | every pair |
| `VERIFICATION` | `REVIEWER` | a second examiner, blind to the first (FISWG's verification) | every pair |
| `INCONSISTENCE` | `INCONSISTENCE` | a third examiner, only when the first two disagreed | the disputed pairs |
| `REPORT` | -- | the first examiner | issues the report, closing the analysis |

A review's pairs are the matcher's candidates for the case's faces (every SYSTEM decision touching
one, ranked by score, scores shown); a comparison's are the pairs a coordinator listed. Only pairs whose
other side is an enrolled person or another face of the same case are examined. An examiner's
decisions are drafts (`analysis_decision_drafts`), changeable until they submit the step; the
submit writes them to `biometric_decisions` with the analysis id and moves the analysis on (an
analysis with no pairs goes from EXAMINATION straight to REPORT), and the API then enqueues the
face sync so clusters follow. Drafts are shown only to their examiner, and a reassignment of the
step discards them.

**Distribution** (`distribution` package) hands out each step when its stage comes:
`analysis_assignments` is an append-only history with one current examiner per step
(`current_analysis_assignments`), and an open analysis whose current stage has none is pending.
`distribution.Distribute` assigns up to a requested number of pending items, oldest first, each to
the active examiner of the modality with the fewest open items who holds no other step of that
analysis (lowest user id on a tie), and leaves the rest pending. An examiner who has already
worked on the pairs the step decides -- a human decision on one of them, or a draft in another
analysis (`ListPairDeciders`) -- gets it only when every eligible examiner has, whatever the loads.
Distribution can be limited to one analysis type and previewed as a dry run. A coordinator can
reassign the current step or unassign it back to pending (`distribution.Assign`, method `MANUAL`,
recording who did it); it does not apply that preference, but the screen marks those examiners
and warns when one is chosen (`distribution.PriorDeciders`). Both take a transaction-scoped advisory lock, so
two distributions cannot hand out the same item.

**Who sees and decides** (`docs/access-profiles-plan.md`). The account role (`SUPER_ADMIN`,
`ADMIN`, `USER`) manages accounts only and gives no access to cases or persons; access comes from
capabilities, each independent: examining a modality, intelligence and
coordination. An examiner lists and opens only the cases where they hold a step of an analysis,
open or closed, and adds or changes evidence there; an intelligence analyst or a coordinator reads
every case (`distribution.Rights`, enforced on every `cases/:caseId` route), and such a read by
someone holding no step there goes to `access_log`, as do full person pages.
Persons, clusters, face search and the occurrence map are intelligence only; an examiner sees a
person's identity. Distribution, reassignment and opening comparisons are coordination. Working on
an analysis -- deciding on its pairs, submitting, issuing its report -- is narrower and the same
for everyone: only the examiner holding the step of its current stage, while they examine its
modality and it is open (`analyses.CanWork`). Blind work overrides every capability: a user working
a step blind sees only that workspace on its case (`distribution.BlindIn`) and loses intelligence,
coordination and person search everywhere until they submit (`distribution.BlindAnywhere`). A user
who creates a case of a modality they examine takes the first step of its automatic analyses at
once (a `MANUAL` row by themselves); otherwise they stay pending. Pairs that do not sit in one
case -- trace vs trace across cases, enrollment vs enrollment -- are not in any analysis yet.

## 4. Clustering

`clusters` / `cluster_members` / `cluster_merges` persist stable, citable cluster numbers:

- A cluster is created once and its id is never reused or renumbered.
- Two features join the same cluster when a confirmed (POSITIVE) decision links them.
- A cluster **merges** when a new decision links two existing clusters (oldest id survives,
  recorded in `cluster_merges`).
- A cluster **splits** when a decision is revoked (the component containing the smallest
  feature id keeps the id; the rest get new ids).
- A singleton (one feature, no match) is still an unknown-subject cluster.

### 4.1 Two layers: automatic and reviewed

Clusters come in two layers (`clusters.basis`), reconciled the same way by
`cluster.Run`, each with its own stable ids and merge history:

| Layer | Built from | Read by |
|---|---|---|
| `AUTOMATIC` | every CONFIRMED pair, a SYSTEM match alone included | intelligence views: the Clusters page and graph (`cluster.ListOverview`, `cluster.BuildGraph`) |
| `REVIEWED` | pairs people confirmed (`cluster.Reviewed`): VERIFICATOR and REVIEWER agreeing, or INCONSISTENCE | forensic views: a case's clusters (`cases.ListCaseClusters`), a person's profile, cases and search badges (`person`) |

- A SYSTEM confirmation stays automatic even while a human round on the pair is pending (that
  round does not undo it, see `cluster.DecidingRound`), so it never reaches a forensic view.
- Every REVIEWED cluster sits inside one AUTOMATIC cluster; a feature is in one cluster per
  layer (`cluster_members` is unique per `(basis, feature_id)`, and a composite FK keeps a
  member's basis equal to its cluster's).
- The case page lists each reviewed cluster's other samples as **direct** (a reviewed decision
  with one of the case's traces, naming who settled it) or **indirect** (a path of reviewed
  decisions, shown with the samples it runs through). A coordinator can open a direct comparison of
  an indirect link to an enrolled person; pairs across cases are not distributed yet.
- The case page's clusters panel shows only reviewed clusters holding at least one reviewed
  pair (no panel at all otherwise), and nothing of the automatic layer: other cases' faces
  reached only by SYSTEM matches never show, not even as a count.
- Separately, the case page lists the case's **identity decisions**
  (`cases.ListCaseIdentityDecisions`): for each face, every enrolled identity the matcher reached
  at or above the review threshold (it writes nothing below it) or that examiners decided in a
  comparison, with its state -- awaiting review, confirmed, excluded, inconclusive or disputed.
  It is the review's own record, so a SYSTEM POSITIVE shows as awaiting review, never as
  confirmed. The case's blind verifier (REVIEWER step, analysis at VERIFICATION) gets none.
- Each reviewed link has a **basis** (`cluster.ReviewedLink`), from the round that settled it: a
  lead (a review) or a laudo at its conclusion level (a comparison). A lead identifies while it is
  the pair's only human round; a comparison that concludes replaces it, one still pending or
  ending inconclusive leaves it standing (shown beside it), and an exclusion takes the pair out of
  the layer. A laudo identifies at or above the identification level (`identification_levels`, as
  data; `STRONG_SUPPORT` by default) and only supports the link below it. The case page, the
  person profile and the sim's `eval` carry it; a cluster can
  hold links of both bases, so it is per link, never per cluster.

This separates the two uses of a match instead of changing the decision rule: a SYSTEM POSITIVE
is still CONFIRMED, so matching stays fast for intelligence, while forensic work rests only on
reviewed links.

## 5. Feature ids

Stable, so decisions, clusters and the API key on the same strings:

- enrolled (KNOWN): the bare `identity_file.id`
- questioned (evidence): `"TRACE:<case_trace_id>#feature"`

(`graph.KnownFeatureID`, `graph.QuestionedFeatureID`.) Postgres is the only store: an earlier
Neo4j projection of evidence, identities and clusters was removed on 2026-10-27, since nothing
read it.

## 6. Flow

```
organization import cmd ──► Postgres (the tables above)
                               │
    cmd/worker          ──► embeddings and templates ──► SYSTEM decisions ──► clusters
    examiners (the API) ──► human decisions ──► clusters
    cluster-biometrics  ──► clusters, on demand
```

Every logic entry point reads Postgres — never an external file or a third-party system.

File bytes live in object storage (MinIO/S3), referenced by `storage_ref`. The layout is
trackid's, content-addressed by sha256: `identity/<register>/<file_type>/<sha256>`,
`evidence/<case>/<sha256>`, `codification/<case>/<trace>/<sha256>`, `videos/<case>/<sha256>` and
`reports/<case>/<sha256>` (report drafts and issued documents). An import cmd holding
raw bytes uploads them with `identity.UploadFile` / `cases.UploadEvidenceFile`, which return the
`FileInput` to pass to `Ingest`; `Ingest` itself never touches storage.

## 7. Implementation status

Done (core), end to end:

- **Schema** (`db/migrations/001`–`008`, one file per subject): `users` and `access_log`
  (`001_users`); `person`, `identity_document`, `identity_register`, `identity_file` (the
  enrollment hierarchy, `002_identity`); the occurrence layer (`003_occurrences`);
  `biometric_cases`, `case_files`, `case_evidences`, `case_traces`, `case_codifications`,
  `case_codification_points`, `case_decisions` (the evidence hierarchy, `004_biometric_cases`);
  `biometricfeature` (unified KNOWN+QUESTIONED, exactly-one-source constraint against
  `identity_file`/`case_traces`), `feature_embeddings`, `biometric_templates`,
  `match_thresholds` (`005_features`); the analyses (`006_analyses`); `biometric_decisions`
  (append-only decision log, `007_biometric_decisions`); `clusters`/`cluster_members`/
  `cluster_merges` (`008_clusters`). This is the model's current shape written directly: the
  21 migrations that built it up were merged on 2026-10-20, before any deployment, so no
  intermediate step is carried. Facial reports add requests, trace roles and suitability, the
  conclusion scale and report documents (`009_reports`) and video (`010_video`).
- **Occurrences** (`db/migrations/003_occurrences.sql`): `locations` (PostGIS), `occurrences`,
  `occurrence_types` (seeded), `occurrence_classifications`, `municipalities`, and
  `biometric_cases.occurrence_id` (section 2.3), ingested by `occurrences.Ingest`.
- **Decisions and clustering** (`cluster` package): `cluster.DeriveEdgeStatus`
  (`cluster/decision.go`, unit-tested) turns a pair's `biometric_decisions` chain into
  PENDING_REVIEW/CONFIRMED/DISPUTED/REJECTED; `cluster.Run` derives AUTOMATIC clusters from
  CONFIRMED pairs and REVIEWED clusters from the pairs people confirmed (section 4.1), in
  Postgres; a cluster holding an enrolled feature identifies its questioned members.
- **Commands**: `cluster-biometrics`, `match-embeddings`, `match-threshold` and the backfills
  (dry-run by default, `-commit` to write).

Verified against a live Postgres: the full migration chain applies cleanly (goose version 10
reached), and a smoke insert through
`identity_register → identity_file → biometricfeature (KNOWN)` and
`case_evidences → case_traces → biometricfeature (QUESTIONED)`, joined by one
`biometric_decisions` row, reproduces the expected cluster (`cluster.Run`) and identification.

**Not in core** (by design — core is the shared model/library; ingestion is a downstream
concern): an organization import command that populates these tables from source files.
Downstream apps (e.g. trackid-pf) call the `db` package's generated queries
(`UpsertIdentityFile`, `UpsertBiometricFeatureFromIdentityFile`/`FromCaseTrace`,
`UpsertCaseFile`/`CaseEvidence`/`CaseTrace`/`CaseCodification`, `InsertBiometricDecision`, …)
directly, the same way `auth` calls `CreateUser`.
