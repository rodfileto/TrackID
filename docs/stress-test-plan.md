# System stress test

Planned and built 2026-10-29 (tooling in `../trackid-sim`, see How to run). It answers two
questions with numbers:

1. **Burst:** when a large batch of operations arrives in a short interval (requests, evidence,
   enrollments), how long does TrackID take to ingest, match and cluster it, and which stage
   gives way first? The target is peak load, not yearly totals. How much data accumulates over
   time is not a goal of this test.
2. **Concurrency:** how many people can work in the UI at once (a class, an institute's shift)
   before the API slows down or returns errors?

Both run on a **separate stack**, so the dev database and the world loaded in it are never
touched. All data comes from `../trackid-sim`, as everywhere else in the project.

## What is there now

- **Load generation exists.** `trackid-sim generate -humans N -seed S` sizes the world.
  `load.Ingest` pushes it through trackid's own contracts (`identity.Ingest`, `cases.Ingest`),
  and `load.Wait` waits for the worker. No separate data generator is needed.
- **Faces stop at about 200 identities.** The face pool has 200 base faces and `AssignFaces`
  never shares a face between humans. Above that, extra humans get no photos or facial traces.
  To scale faces, the plan has to reuse faces or synthesise embeddings (see Decisions, 3).
- **Fingerprints scale freely.** `fpgen` draws one finger per human from (seed, human id).
- **Fingerprint matching is all-vs-all.** `biometricmatch.RunTemplatesBand` scores every
  template against every other through the SourceAFIS sidecar. Measured so far: about 200
  templates in about a minute (ROADMAP C3). A ten-print world (about 1,650 templates) was
  estimated at an hour or more. This is the expected first wall.
- **Face matching uses pgvector** with an HNSW index (`db/migrations/005_features.sql:70`),
  one query per embedding. It should scale far better than fingerprints, but it has not been
  measured.
- **Clustering** (`cluster.Run`) loads every confirmed pair and runs union-find in memory, then
  rewrites the clusters in one pass. It has not been measured above the demo world.
- **The worker** ran Asynq with `Concurrency: 10`, hardcoded; it is now `WORKER_CONCURRENCY`
  (default 10). There is one sidecar container (a JVM).
- **The API** has never been tested with concurrent users. The class flow uses login,
  `me/analyses`, the worksheet (`analyses/:id/pairs`), pair images, `decisions`, `submit`, the
  report draft and issue (WordCraft CLI), and the image lab (`imageproc/*`, CPU-heavy).
- **The compose ports were fixed** (55432, 56379, 59000/59001, 58090); they are now variables
  with those defaults, so a second stack starts next to the dev one.

## Decisions

Decided 2026-10-29: **GPU only.** Every run uses the GPU for detection and embedding, and no
CPU-only run is planned. The volume track measures **bursts**: operations concentrated in a short
interval, not a year's totals. The burst keeps **the demo world's current mix** (facial and
fingerprint requests, evidence files per request, enrollments against cases), scaled up by
`-humans` with no new mix parameters. **All the stress tooling lives in `../trackid-sim`.**
TrackID itself changes only where the separate stack needs it (port variables in
`docker-compose.yml`, and `WORKER_CONCURRENCY` in `cmd/worker`).

1. **A separate stack.** The ports in `docker-compose.yml` become variables with today's values
   as defaults (`${POSTGRES_PORT:-55432}`, …). The stress stack then runs as its own compose
   project (`docker compose -p trackid-stress -f ../trackid/docker-compose.yml`, with its
   own volumes) from the sim's `scripts/stress/env.sh`, which sets other ports,
   `DATABASE_URL`, the bucket and the API port. The dev stack is not affected, and nothing is
   erased that does not belong to `trackid-stress`. `trackid-sim stress` refuses to run without
   `TRACKID_STRESS=1` (set by `env.sh`) or on a database that already holds cases or
   registers.
2. **Tools, all in the sim: Go for the pipeline, k6 for the API.** The burst track is a sim
   subcommand (`trackid-sim stress`) built on `generate` + `load`, which records timings. The
   concurrency track uses [k6](https://k6.io) scripts in the sim's `scripts/stress/k6/`. The
   stack scripts (`scripts/stress/env.sh`, `up.sh`, `down.sh`) live in the sim too. k6 runs as a
   local binary or container and sends nothing outside the machine.
3. **Faces above 200 identities: reuse faces, and do not score accuracy.** For burst runs
   only, the sim gets a flag (`-face-reuse`) that lets humans share pool faces. Matching and
   clustering then do realistic work, but their precision means nothing. Burst runs report
   time and resources, never `eval` scores. The alternative, synthetic random embeddings
   inserted directly, skips the detector and embedder, which are part of what is measured.
4. **Fingerprints scale through humans and fingers.** Sizes are swept by humans and, when the
   sim allows it, by fingers per human (ten-print), because the template count drives the
   O(n²) cost.
5. **Measurements.** Each run writes one JSON record to `out/stress/` in the sim, never to
   claude.ai or any outside service. The record holds the world parameters, the git SHAs of
   both repos, the machine (CPU, RAM, GPU) and the results:
   - wall time per stage: generate, upload, detect/embed, extract, match (face, fingerprint),
     cluster;
   - the queue depth over time (Asynq), retries and failures;
   - Postgres: table and index sizes, slowest statements (`pg_stat_statements`), and the
     connection count;
   - container CPU and memory (`docker stats` sampled), including the sidecar's JVM heap;
   - for the API: p50/p95/p99 latency per endpoint, error rate, and requests per second.
6. **Pass criteria (proposed, user's to set).**
   - Burst: every operation in the burst is accepted (no 5xx, no dropped upload). The queue
     drains with no failed tasks. Each burst level records the time until the backlog clears and
     the time from arrival to the last SYSTEM decision and cluster. No stage may grow faster than
     its expected order (face matching about n·log n, fingerprint matching n²) without a known
     cause.
   - Concurrency: 30 simultaneous examiners (twice a 12–15 student class), p95 under 500 ms on
     reads and under 2 s on submits, 0 errors other than 4xx by design. Report issue and image
     lab renders are measured separately, with their own budget.

## Phases

**Phase 0: the stack and a baseline.**
- In trackid, parametrise the compose ports. In the sim, add `scripts/stress/env.sh` and an
  `up`/`down` script for the
  `trackid-stress` project. `down` removes only that project's volumes.
- Enable `pg_stat_statements` in the stress Postgres.
- Add a timing harness around `generate` → `load` → `Wait` → `cluster` that writes the JSON
  record. Run it on the demo-sized world (seed 1, default humans) as the baseline, and check
  that the numbers agree with ROADMAP C3/C4 (about 200 templates in about a minute).

**Phase 1: burst sweep.**
- **Concurrent arrivals.** `load.Ingest` today walks the world one human and one case at a
  time, so it trickles rather than bursts. The harness gets a burst mode that sends the world's
  operations from many parallel senders (`-senders K`), all within a set window (`-window W`).
  Each operation is one person's enrollments, or one case with its evidence and its request,
  uploaded and written through trackid's own contracts (`identity.Ingest`, `cases.Ingest`,
  `requests.Create`), as `load` does. *Changed while building:* not through the HTTP API, which
  has no enrollment endpoint; the API under load is the concurrency track's. Everything is drawn
  before the clock starts, so the burst times trackid, not the simulator's rendering.
- **Levels.** A batch of B operations sent within window W, with B swept as {1, 2, 5, 10, 20}
  times the demo world's size, and W short (proposed: 5 min, plus one run with everything sent
  at once). Stop at the first level whose backlog takes longer than a time cap to clear
  (proposed: 2 h), and record that level as the limit.
- **Record per level.** Queue depth over time, the time until the backlog clears, GPU
  utilisation and memory for detection and embedding, the sidecar's CPU, and the API's latency
  while the burst is being accepted.
- Fingerprint matching alone, at growing template counts, through the sidecar: find the
  crossover where all-vs-all becomes impractical. Record whether the bottleneck is the
  sidecar's CPU, the HTTP round trips or Postgres reads.
- The worker's concurrency at {5, 10, 20} (`WORKER_CONCURRENCY=20 scripts/stress/sweep.sh`)
  against the sidecar's saturation.
- Ingest burst: a single request with many evidence files (video included) to measure upload,
  MinIO and the detection queue.

**Phase 2: concurrent users.**
- Load a world, then create N examiners with `trackid-sim users -examiners N`. The loaded
  requests ask a FACIAL_REVIEW of every facial case; the coordinator scenario distributes them.
  Examiners are facial only (as `users` makes them); fingerprint comparisons start with no pairs.
- k6 scenarios, each a virtual user following a real flow:
  - *Examiner:* log in, list my analyses, open a worksheet, fetch pair images, save drafts,
    submit.
  - *Coordinator:* distribution status, distribute, list requests.
  - *Analyst:* clusters, cluster graph, occurrence map, overview graph.
  - *Lab:* upload a source, estimate, render (CPU-heavy, few users).
  - *Reports:* refresh a draft and issue (WordCraft CLI, slow by nature). **Not built yet**: a
    review reaches REPORT only after its rounds, so it needs a run long enough, or a world
    prepared with analyses already at REPORT.
- Ramp from 1 to 60 virtual users, hold, then a spike. Run the mix that a class produces (most
  users are examiners) and each scenario on its own, to attribute any slowdown.
- Correctness under concurrency: two examiners submitting the same analysis step at once, and
  submits racing the worker's cluster rebuild. After each run, check that there are no
  duplicate decisions, no lost submits and no analysis stuck between stages (SQL checks kept
  with the scripts).

**Phase 3: combined and report.**
- Run the concurrency mix while a burst is being processed (ingest and clustering under user
  traffic), because that is what a live institute does.
- Write the results into this document's Status: the walls found, the numbers, and the fixes
  proposed, each as its own item (for example a candidate index for fingerprints instead of
  all-vs-all, incremental clustering, or a pool for the sidecar).

## How to run

All from `../trackid-sim`, on the GPU machine. The dev stack may be up or down; it is not
touched.

```sh
scripts/stress/up.sh                       # fresh stress stack: containers, migrations, thresholds, API, worker (GPU checked)
source scripts/stress/env.sh
bin/trackid-sim stress -humans 600 -senders 8 -window 5m   # one burst; record in out/stress/<time>-<label>/
scripts/stress/down.sh                     # stop it and erase its data

scripts/stress/sweep.sh -levels "1 2 5 10 20" -window 5m  # phase 1: fresh stack per level, stops at the limit
scripts/stress/sweep.sh -levels "1 2 5" -window 0         # everything at once
WORKER_CONCURRENCY=20 scripts/stress/sweep.sh ...         # the worker's concurrency

scripts/stress/class.sh -vus 30 -hold 5m   # phase 2: world, accounts, k6, consistency checks
```

- `trackid-sim stress` (`cmd/trackid-sim/stress.go`): generates the world (`-face-reuse`, on by
  default, through `world.ReuseFaces`), sends it with `load.Burst`, waits with
  `load.WaitObserved`, and writes `record.json`. The record holds the configuration, machine and
  commits; each operation's timing and error; the pipeline's progress over time; GPU, load,
  container and Postgres samples; when each stage finished, from the rows' own timestamps; row
  counts; table sizes; and the costliest statements (`pg_stat_statements`).
- `scripts/stress/class.sh` runs `k6/class.js` from the `grafana/k6:0.54.0` image on the host
  network. Its summary has p50/p95/p99 per request name, and `checks.sql` holds the consistency
  checks (blind steps held by one examiner, a step distributed twice, decisions by examiners never
  given the step, analyses that moved on without a decision, empty clusters, duplicate
  embeddings or templates).

## Open

- Ten-print in the sim. It draws **one finger per human**, a choice made because of the
  all-vs-all cost (ROADMAP C3, 2026-10-11). A ten-print burst run needs a sim flag for
  fingers per human. Until then, the template count grows only with humans.
- The pass criteria in Decisions, 6.

## Status (2026-10-29)

Built (phases 0–2, see How to run), and a first run of each track on this machine (8 cores,
60 GiB, RTX 2080 8 GiB; records in `../trackid-sim/out/stress/`):

- **Baseline burst, demo world** (300 humans, faces not reused; 412 operations, 701 files,
  129 MiB, 8 senders over 5 min): **passed**. 0 failed operations, p95 0.10 s per enrollment and
  0.08 s per case. The pipeline kept pace with arrivals: queue at most 5, last embedding,
  template and match within 2 s of the last arrival, cleared 10 s after it. GPU at most 6% and
  2.7 GiB. 486 face embeddings, 294 templates, 1,691 SYSTEM POSITIVE, 323 INCONCLUSIVE.
  Drawing the files beforehand took 3 min (outside the clock); it grows with the world, so a 20×
  level spends about an hour drawing.
- **30 examiners, spike to 60** (plus a coordinator, 3 analysts and 2 in the image lab; 2 min
  ramp, 5 min hold, 2 min spike): **passed**. 8,654 requests, 0 failed; reads p95 57 ms, writes
  p95 59 ms. The slowest request is `distribute` (p95 660 ms); the rest stay under 100 ms at p99.
  All consistency checks 0. The demo world runs out of work: 333 steps submitted, and 2,264
  examiner visits found nothing to do. A busier class needs a larger world (`class.sh -humans`).
- **Fixes to the test itself, found by the first runs**: virtual users sharing an account (k6
  numbers them across scenarios) got 403s by design, and examiners tried to submit analyses at
  REPORT; both corrected in the k6 script. A review with nothing to decide goes straight to
  REPORT by design, so the consistency check no longer counts it.

**Burst sweep, 2026-10-29** (face reuse on, 8 senders, 5 min window, worker concurrency 10):

| Level | Humans | Operations | Failed | Faces done | Fingerprints done | Cleared |
|---|---|---|---|---|---|---|
| 1× | 300 | 412 | 0 | 302 s | 302 s | 360 s |
| 2× | 600 | 778 | 0 | 301 s | 301 s | 350 s |
| 5× | 1,500 | 1,895 | 0 | 301 s | — | 696 s |
| 10× | 3,000 | 3,873 | 0 | 383 s | 1,889 s | 2,654 s |
| 20× | 6,000 | 7,541 | 0 | stalled | stalled | stopped |
| **10×, fixed** | 3,000 | 3,873 | 0 | **301 s** | **304 s** | **310 s** |

Times are from the burst's start; the last operation arrives at 300 s. The GPU never passed 40%.

- **Found and fixed: fingerprint syncs piled up.** `asynq.Unique` held its lock for the 30 s
  debounce, not for the sync's run, so long all-against-all syncs kept starting new ones. At 20×,
  eight ran at once, filled the worker and starved face embedding. Now one sync per modality
  runs at a time (`cluster.TryExclusive`, Postgres advisory locks; a sync that finds one running
  schedules a later one), and `cluster.Run` rewrites the clusters one at a time.
- **Added: a gallery in the sidecar.** Rebuilding a template from its bytes cost more than
  comparing it, and `/match` did it for every candidate of every probe. `/match-cached` keeps
  templates deserialised (about 26 KB of heap each on these prints) and matches one probe's
  candidates on every core. Through the Go client: 270 µs per comparison before, 42 µs after.
- **10× with both fixes cleared 10 s after the last arrival**, from 2,654 s, with the sidecar at
  most 3.7 cores, from 7.9. Fingerprint matching kept pace with arrivals, so no backlog formed.
- Drawing the files took 28–55 min per level (outside the clock). The draw cache
  (`out/stress/cache/`) now keeps them for a burst of the same world again.

Not run yet: 20× with the fixes, the window-0 and worker-concurrency variants. Not built: reports in the k6 mix, phase 3 (users during a burst,
which needs a case id prefix per world), ten-print.
