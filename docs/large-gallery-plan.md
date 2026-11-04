# Large gallery: bulk enrollment and deduplication

Planned 2026-11-03, rewritten twice the same day: after checking the first version against the
code, and after settling that the first upload must match every enrollment against every other.
Phases 1–3 built 2026-11-04, in core only: see [Status](#status-2026-11-04).

## Goal

A gallery deployment loads an existing identity register, of the order of **10⁸ face photos**,
into one server, and clusters every enrollment against every other at least once. The aim is to
find **one face under more than one identity**. The case side stays small: a few thousand cases
and their traces (N_Q ≈ 10⁴), matched against the whole gallery. After the upload the deployment
works day to day, with new enrollments and cases arriving a few at a time.

The work is split into two commands:

1. **`bulk-enroll`** loads enrollments and vectorizes their photos. It is bound by reading and
   decoding images and runs once per photo (again only when the face model changes).
2. **`bulk-match`** matches, calibrates and applies thresholds, in batches. It is bound by GPU
   compute and by examiners' review, and reruns at every batch and every threshold version.

Kept apart, a recalibration never redoes the expensive one-off work, and a slow read never holds
up matching.

Figures are per 10⁸ faces (N). A deployment's own numbers (its register, sample measurements,
server, importer and batch order) belong with that deployment, not here. Nothing has been
measured at this size: the figures are estimates from the code, and Phase 0 replaces them.

## Why the current pipeline does not hold

Checked in the code.

**The per-face sync** (`embedding.SyncFace` → `biometricmatch.RunBand` → `cluster.Run`)
- `computeCandidates` loads every embedding with `matched_at IS NULL`, read as `embedding::text`
  (about 5 KB per face in Go, so about 500 GB for 10⁸). It also loads every biometric feature
  and every decision, even when one face is new.
- Matching runs one ANN query at a time. `recordDecisions` writes every decision and every
  `matched_at` in one transaction.
- Any new trace or embedding triggers a sync. During an upload, a sync would take the whole
  backlog, and `cluster.Settle` would rerun it up to `MaxSettlePasses` (5) times, each pass ending
  in a full `cluster.Run`.

**Vector search**
- One HNSW index covers both embedding types and both provenances, filtered after the index scan.
  With pgvector's default `ef_search` (40) and enrolled faces far outnumbering traces,
  `FindNearestCasesByFaceEmbedding` returns nothing.
- Through HNSW, matching every face against every other is 10⁸ approximate queries on an index
  of 120–230 GB, which no longer stays in memory. Approximate search also misses a few percent
  of neighbours at default settings, and for deduplication those misses are lost duplicates.
- The Postgres image builds on `pgvector/pgvector:pg16`, an unpinned tag.

**Thresholds and clustering**
- `DefaultFaceBand` (0.6, Review = Confirm) has never been calibrated on operational data.
- `RunBand` never re-decides a matched pair, and `biometric_decisions_pair_role_analysis_key`
  (`UNIQUE NULLS NOT DISTINCT (a, b, role, case_analysis_id)`) allows one SYSTEM row per pair. A
  new threshold version therefore cannot reach pairs already decided, and a pair that scored
  below the old review threshold was never written at all.
- `reconcile` makes a cluster of every feature, singletons included, in both layers.

**Vectorizing**
- `identity.Ingest` enqueues one Asynq task per feature when given an enqueuer (without one it
  enqueues nothing).
- `ListKnownFaceFeaturesMissingEmbedding` reads every missing id at once.
- `EmbedFaceInBox` decodes the photo, draws it on a padded canvas, encodes that as PNG, and
  trackid-vision decodes it again. For multi-megapixel identity photos the round trip probably
  costs more than the JPEG decode itself.
- trackid-vision runs one image per `session.Run`, on one `DeviceID`.
- Nothing records why a photo produced no embedding (no face, several faces, undecodable), so
  failures are silent and get retried.

**Review**
- Examiner decisions are made inside analyses, and `case_analyses.biometric_case_id` is
  `NOT NULL`: every analysis belongs to a case. A pair of two enrolled faces belongs to no case.

## Command 1: `bulk-enroll` (load and vectorize)

**Input: a manifest.** JSON Lines, one enrollment per line, in the shape of
`identity.Enrollment`. Each file carries either a `path` (uploaded to core's bucket, as today)
or a `uri` (an external object, kept where it is). The deployment's importer only has to
produce the manifest: core never sees the source system's format.

**Steps** (each resumable, each a dry run without `-commit`, like the other commands):
1. **Rows.** `identity.Ingest` per enrollment, in parallel, with no enqueuer. Files with a `uri`
   are stored as external references. Today `storage.Client.Download` accepts only
   `<bucket>/<key>`, so storage gains a resolver: core's bucket, another S3 bucket, a local file
   path.
2. **Vectorize.** Page by keyset over KNOWN face features with no embedding and no final
   outcome. For each page:
   - fetch and decode the photos in parallel on the CPU, handing decoded pixels to trackid-vision
     (no PNG round trip);
   - detect and embed in batches, FP16, one worker per GPU;
   - write embeddings with `COPY`.
3. **Crop** (`-crops`). A face crop (112 px, a few KB; a box crop for now, see Decision 6) is stored in core's bucket, so display and
   comparison never need the external original.

**What it records for every photo** (a new table, one row per feature): the outcome
(`EMBEDDED`, `NO_FACE`, `MULTIPLE_FACES`, `UNDECODABLE`, `FETCH_FAILED`), the number of faces
found, the chosen face's detector score and its size relative to the image. Final outcomes are
not retried. The quality fields feed calibration (false-match hubs are mostly poor photos), and
`MULTIPLE_FACES` on an identity photo is itself worth reviewing.

**The model version is fixed for the whole upload.** A new model means vectorizing everything
again, and its scores need a new calibration.

**Handover: the match path.** `feature_embeddings` gains `match_path` (`SYNC` | `BULK`, default
`SYNC`). `bulk-enroll` writes `BULK`. The per-face sync reads only `SYNC` rows, so a trace
arriving during the upload never pulls 10⁸ pending embeddings into the worker. `bulk-match`
sets `matched_at` on a batch's rows once that batch is done. Every embedding after the upload is
`SYNC`, as today.

**Vector shards.** Besides Postgres (the source of truth), `bulk-enroll` writes FP16
vector shards to local disk (about 100 GB per 10⁸), so `bulk-match` reads files instead of
pulling about 200 GB out of Postgres at every batch.

## Command 2: `bulk-match` (match, calibrate, apply)

Three subcommands. Choosing a threshold version stays a separate, deliberate act through the
existing `cmd/match-threshold`, so a version is always set by a person, with its `source`
recorded.

### `match`: exact scores, stored

- Takes a batch: `BULK` embeddings not yet matched, chosen by the deployment's sampling (a
  random or stratified selection, by count). Batches are recorded in a new table with their
  members, model version and timings.
- Compares, exactly, on the GPU:
  - the batch against itself;
  - the batch against every face matched in earlier batches;
  - the batch against every trace (N_Q is small, so this costs almost nothing).
- For each face it keeps the neighbours scoring at or above a **floor**, well below any
  plausible review threshold, with a cap per face (k, for example 100). Faces that hit the cap
  are recorded: they are the hubs.
- The scores go into a new **score store**, keyed by `feature_embeddings` ids (bigints, not the
  text graph ids), with the score, the batch and the model version. The store is append-only.
  Re-matching is needed only for a new model.
- Marks the batch's embeddings matched.

Every pair is compared exactly once across all batches: the total work is N²/2 comparisons
whatever the batch sizes. That is about 5 × 10¹⁸ FP16 operations per 10⁸ faces, hours to about a
day depending on the GPUs, and the selection of the top neighbours may cost as much as the
multiply (Phase 0 measures both). Scores near the floor and the thresholds are recomputed in
FP32 before storing.

### `calibrate`: a report, no writes

For a model version, over all stored scores:
- **Genuine scores from identity data.** Pairs whose records say they are the same person: the
  same document or register, or the same fiscal number. Their scores are computed directly, so
  genuine pairs below the floor are counted too. That gives the false non-match rate at each
  threshold.
- **Impostor scores from identity data.** Pairs whose records disagree: different fiscal
  numbers, or different name and birth date where there is no fiscal number. Each impostor
  comparison was made exactly once, so FMR(t) is the number of stored impostor pairs scoring at
  least t, divided by the number of impostor comparisons made. That holds as long as t is above
  the floor and few faces hit the cap. The curve is slightly contaminated by the very fraud
  cases being sought, which are rare.
- **Examiner decisions** on reviewed pairs: precision inside the band, where identity data and
  score disagree. That is where fraud and lookalikes both sit.
- **For each candidate band:** FMR and FNMR, the pairs it would put in review (to compare with
  the examiners' capacity), and the projected **mean false degree at the final gallery size**
  (FMR × N_final).

Which record fields define same and different identity is a setting, because deployments differ
in what their records carry.

### `apply`: band, decide, cluster

- Bands the stored scores under the current threshold version: at or above Confirm is POSITIVE,
  between Review and Confirm INCONCLUSIVE, below Review nothing.
- Writes SYSTEM decisions in bulk (`COPY` into a staging table, then insert), citing the
  version.
- Runs clustering once and reports:
  - the cluster size distribution and the largest cluster;
  - the number of clusters holding more than one identity;
  - the review queue: pairs above Review whose identities differ, ranked by score. Those are the
    fraud candidates, and also what calibrates the next version.

A new version re-bands what is already stored, with no new matching: see Decision 3.

## The percolation limit

Union-find has a hard limit. If false links fall like a random graph's, each face's average
number of false links is FMR × N, and once that passes 1 the AUTOMATIC layer merges into one
giant cluster. At 10⁸, Confirm therefore needs FMR well below 10⁻⁸; about 10⁻⁹ gives a mean
false degree of 0.1.

Real false matches are not random: poor photos, children and lookalikes become hubs that link
to many faces. So:
- `calibrate` projects the degree at the final size, not at the current one;
- `apply` reports the largest cluster after every batch;
- a guard against chaining is part of `apply`. Candidates: confirm a SYSTEM link only when it is
  mutual within the top k, cap a hub's SYSTEM links, or alarm on cluster size. To be chosen on
  the pilot's numbers.

SYSTEM INCONCLUSIVE pairs stay PENDING_REVIEW and never join a cluster, so a wide review band
costs examiner time, not cluster quality.

## The batch cycle

1. **Pilot.** `bulk-enroll` the whole register (or a large part of it). `bulk-match match` a
   pilot batch of a few million, chosen at random or stratified by whatever drives photo quality
   (capture era, region, device).
2. `calibrate`. Set a provisional version with `match-threshold`, its review band sized to the
   examiners' capacity. `apply`.
3. Examiners review the queue.
4. `calibrate` again, with their decisions. Set a new version, then `apply`, which re-bands
   everything stored.
5. `match` the next batch, which is compared against everything before it, then `apply`. Batches
   grow as the threshold settles.
6. After the last batch: build the steady-state index (below) and hand over to the per-face sync.

A duplicate spread across two batches appears only once both are matched, so fraud results are
complete only after the last batch. AUTOMATIC clusters split and merge as versions change, which
is what that layer is for. REVIEWED clusters follow people's decisions only and do not move.

## Decisions

Settled 2026-11-04, when building:

1. **The GPU matcher is a Python tool, with FAISS-GPU** (`tools/bulk-match`, run with uv).
   - It does exact FP16 brute-force top-k (`knn_gpu`) and rescores every candidate in FP32.
   - It searches both directions (batch against earlier faces, and earlier faces against the
     batch), so a cap hit on one side does not drop the pair.
   - Its input and output are files: work-directory `.npy` files written and read by Go, which
     loads the scores. The tool needs no database access.
   - The rest of `bulk-match` is Go.
   - Python adds a second language to core, as `tools/municipalities` already did. Going through
     Go on trackid-vision's ONNX Runtime would have meant more work.

2. **Vector shards are written by default.** Postgres stays the source of truth.
   - `bulk-enroll` writes FP16 shards as it vectorizes; `-no-shards` turns that off.
   - Before matching, `bulk-match match` fills from Postgres any batch or earlier face that no
     shard holds. That covers a lost shard, and a run killed before it flushed.
   - `bulk-enroll -rebuild-shards` writes them all again.

3. **A SYSTEM decision per pair and threshold version.**
   - Two partial unique indexes replace `biometric_decisions_pair_role_analysis_key`:
     - human rows: one per pair, role and analysis, as before;
     - SYSTEM rows: one per pair and `related_reference` (the version).
   - The latest SYSTEM row is the pair's SYSTEM decision. The decision chain is read in
     `decided_at, id` order, and `DecidingRound` already took the last SYSTEM row, so it did not
     change.
   - `apply` writes a row only where the decision changes. It writes a SYSTEM NEGATIVE for a pair
     that had POSITIVE or INCONCLUSIVE and now scores below Review. An unchanged pair keeps the row
     of the version that first gave it its decision.
   - Readers that list SYSTEM candidates take the latest row per pair. A review no longer offers a
     pair whose latest SYSTEM decision is NEGATIVE.

4. **Same and different identity, by record fields** (`-identity-key`, default `fiscal,name_birth`;
   also `person`).
   - A face's key is the first of those fields its record has.
   - Two faces are the same identity when they share a document or a key. They are different when
     both have a key from the same field and the keys differ.
   - Otherwise the relation is unknown, and the pair counts on neither curve.
   - Keys shared by more than `-max-group` faces (a placeholder fiscal number, say) are ignored.

5. **External originals through a storage resolver.** `storage.Client.Download` reads:
   - `<bucket>/<key>`, as before;
   - `s3://<bucket>/<key>` on the same endpoint;
   - `file:///<path>`, only inside `STORAGE_FILE_ROOTS`, with symlinks resolved.

   This holds for the whole application, so a photo kept outside the bucket also displays.

6. **trackid-vision is unchanged**, for now: this work stays inside core.
   - `bulk-enroll` hands each photo's original bytes straight to `DetectAndEmbed`, with no canvas
     and no PNG round trip. `-pad` restores the padded path for photos cropped tight to the face.
   - It runs one service per GPU (`-devices`).
   - The batch API (decoded pixels in, FP16 batches) stays the next step for throughput.
   - Crops are box crops at 112 px, not the recognizer's 5-point alignment, which trackid-vision
     does not expose.

7. **Still open:** a home for reviewing gallery pairs (an analysis type with no case, or one review
   case per batch). Until then, `apply -queue-out` writes the review queue as CSV.

## Steady state after the upload

- **Trace search scans traces exactly.** A `provenance` column on `feature_embeddings` (copied
  from `biometric_features`) and a plain index on it. Searching the traces is an exact scan of
  about 10⁴ vectors, a few milliseconds, with no recall loss. This fixes the empty trace search
  and can ship first, independently.
- **KNOWN↔KNOWN for new enrollments uses a `halfvec` HNSW on the KNOWN faces only.**
  - It is a partial index with literal predicates in the SQL. Core reaches Postgres through pgx's
    `database/sql` driver, which caches prepared statements, and a generic plan cannot use a
    partial index on a parameter.
  - A test runs `EXPLAIN` on every vector query and fails on a sequential scan.
  - The index is built once, after the last batch, with `CREATE INDEX CONCURRENTLY`. Until then,
    face search against the gallery is slow.
  - Pin the pgvector version.
- **Paged sync.**
  - Unmatched `SYNC` embeddings stream in pages, keyset on `id`.
  - Feature info is joined per page, never loaded whole.
  - Decisions are inserted with `ON CONFLICT DO NOTHING`.
  - Each page sets `matched_at` in one `UPDATE … = ANY($1)` and commits on its own.
  - The candidate cap becomes a setting.
  - A few thousand enrollments a day then cost seconds.
- **Clusters without singletons.** A feature with no confirmed pair has no cluster row.
  `cluster.Run` reads only chains with a POSITIVE row; readers and tests handle a feature with no
  cluster.

## Out of scope (own plans)

- **Person merge.** Duplicates confirmed by review will need persons merged (or linked) in core.
- **Fingerprints at gallery size.** `RunTemplatesBand` is all-vs-all over templates loaded in
  memory. Templates may be extracted during the upload, but fingerprint matching stays off at
  gallery size until it has its own plan.
- **Approximate GPU search** (IVF-PQ, CAGRA), only if exact matching proves too slow.

## Phases

**Phase 0: measure.**
- Synthetic unit vectors at 10⁶ and 10⁷. Record:
  - the GPU matcher's throughput (multiply and top-k separately), by batch size;
  - the score store's size and insert rate at a given floor and k;
  - `apply`'s time to band, write decisions and cluster.
- `bulk-enroll` on a sample of real photos on the target server: throughput per stage (fetch,
  decode, detect, embed, write, crop).
- Random vectors say nothing about match rates, only about cost.
- Extrapolate to 10⁸ from these measurements.

**Phase 1: `bulk-enroll`.** Manifest, storage resolver, outcome table, `match_path`, keyset
vectorization, trackid-vision batching (Decision 6), the pixels-in path, crops, optional shards.
- Exit: a 10⁶ manifest loaded end to end, resumed after a kill, with every photo accounted for in
  the outcome table.

**Phase 2: `bulk-match match` and `apply`.** The GPU tool (Decision 1), the batch and score
tables, per-version SYSTEM decisions (Decision 3), bulk decision writes, and the cluster report.
- Exit: three batches over 10⁷ synthetic faces, where `apply` under two versions re-bands without
  re-matching. The decision chain shows both versions, and clusters agree with a from-scratch run.

**Phase 3: `calibrate` and review.** Identity labels (Decision 4), the report, a review home
(Decision 7), and the chaining guard chosen on the pilot's numbers.
- Exit: on trackid-sim's world (which has known identities), the report's genuine and impostor
  curves match the sim's ground truth.

**Phase 4: the pilot on the deployment's data**, then the batch cycle. The deployment drives
this phase.

**Phase 5: steady state.** The trace scan (can ship any time), `halfvec` partial HNSW with the
`EXPLAIN` test, paged sync, clusters without singletons.

## Open

- N counts faces, not documents: a document can carry more than one photo.
- The floor and k: set from the pilot, trading score store size against how low a later review
  threshold can go without re-matching.
- Whether `match` runs while `bulk-enroll` is still vectorizing. Both need the GPUs: split the
  cards, or run them in turn.

## Status (2026-11-04)

**Built** (phases 1–3, except the review home):
- migration `011_bulk_gallery.sql`:
  - `feature_embeddings.match_path`;
  - `bulk_enroll_outcomes`;
  - the batch tables;
  - the score store `bulk_match_scores` and `bulk_match_hubs`;
  - the per-version SYSTEM key;
- package `bulk`;
- `cmd/bulk-enroll` and `cmd/bulk-match` (`match`, `calibrate`, `apply`);
- `tools/bulk-match`;
- the storage resolver.

How each step was built:
- **`bulk-enroll` rows.** Manifest lines go through `identity.Ingest`, in parallel.
  `-start-line` resumes a long manifest.
- **`bulk-enroll` vectorize.** Keyset pages; each page's embeddings and outcomes are written in
  one transaction, through multi-row `INSERT … unnest`. Ctrl-C stops after the page in flight.
  `FETCH_FAILED` is retried only with `-retry-failed`.
- **`match`.**
  - A random batch, chosen by `-seed`. Stratified selection is left to the deployment.
  - The scores go into an unlogged staging table with `COPY`, through pgx (or lib/pq in tests).
  - One transaction then stores them, records the hubs and marks the batch matched. A batch left
    `SELECTED` by a failed run is resumed.
- **`calibrate`.**
  - Genuine scores are computed directly in Postgres, so those below the floor count too.
  - Impostor comparisons are counted exactly from the identity groups. FMR, FNMR, the review load
    and the projected mean false degree come out per threshold.
  - Examiners' decisions are counted by score bin.
  - It warns when more than 1% of faces are hubs.
- **`apply`.**
  - Bands in chunks of embedding ids, each its own transaction; rerunning writes nothing new.
  - Clusters once, then reports the size distribution, the largest cluster, and the clusters
    holding more than one identity.
  - Lists the review queue: pairs at or above Review, records saying different people, no human
    decision yet.
  - The chaining guard is an alarm: exit status 3 when the largest automatic cluster exceeds
    `-max-cluster`. Mutual top-k and per-hub caps wait for the pilot's numbers.

**Tested:**
- `bulk.TestBulkGallery`, on a synthetic register, runs the whole cycle:
  - load;
  - vectorize, with a kill and resume;
  - the outcome of every photo;
  - two batches, with a lost shard refilled;
  - the score store equal to brute force, every pair compared once;
  - `apply` under three versions (promote, then drop to NEGATIVE), with the chain showing each one;
  - the review queue led by the planted fraud;
  - calibration counts.

  It runs with a Go brute-force stand-in for the matcher and with the real Python tool.
- `tools/bulk-match` tests compare against brute force on GPU, FAISS CPU and NumPy.
- A smoke run on trackid-sim's synthetic face pool used the real models:
  - 410 photos, at 68 photos/s, unbatched, on one RTX 2080;
  - two batches;
  - the 10 planted duplicates leading the queue.

**Not built yet:**
- **Phase 0 measurements at 10⁶–10⁷.**
- **Clustering at gallery size.** `cluster.Run` (called by `apply`) still loads every decision
  and feature into memory. It is the next bottleneck at 10⁸, together with clusters without
  singletons (Phase 5).
- **The review home** (Decision 7).
- **trackid-vision batching** (Decision 6).
- **Phase 5, steady state:** the exact trace scan, the `halfvec` partial HNSW, the paged sync.

Deployment-specific figures live in the deployment's own repository.
