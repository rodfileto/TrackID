---
title: "TrackID — Research Plan"
subtitle: "Paper portfolio, venues, and the synthetic-data programme behind it"
author: Rodrigo Fileto Cuerci Maciel
lang: pt-BR
---

## Premise

- All evaluation data comes from `trackid-sim` (synthetic, ground-truthed) — see [Synthetic data](#synthetic-data).
- Two research threads: **forensic intelligence** (cross-case linkage, clustering, collaboration) and
  **ACE-V prototype** (face + fingerprint decision workflow).

## Paper portfolio

Suggested order: P1 + P3 in parallel (no data dependency) → P2 → P7 → P6 → P5 → P4.

| # | Working title | Venue | Depends on |
|---|---|---|---|
| P0 | Synthetic multimodal forensic corpus + simulator | FSI or Science & Justice (paper) + JOSS (software) | trackid-sim Phase 1 (Phase 2, validation, removed 2026-10-20) |
| P1 | TrackID: extensible open core for forensic biometric case management | SoftwareX | tests, demo dataset (licence done: Apache-2.0) |
| P2 | ACE-V as an auditable decision graph (blind verification, recorded disagreement) | Science & Justice | blind verification implemented |
| P3 | Citable cluster identity in a continuously revised intelligence database | FSI / Science & Justice (short/technical note) | none — buildable now |
| P4 | Trace-to-trace linkage and unknown-subject clusters (simulation study) | FSI (alt: Crime Science, Policing) | trackid-sim Phase 5 |
| P5 | Human-in-the-loop face triage: thresholds, calibration, demographic differentials | IET Biometrics / FSI | trackid-sim Phase 3 (real images) |
| P6 | Design Science Research: breaking forensic data silos | DESRIST → Government Information Quarterly | Quarto technical-report as base |
| P7 | Auditability as an LGPD safeguard in biometric intelligence systems | FSI: Synergy / Brazilian public-law journal | decision log (done) |
| P8 (optional) | Graph schema as a forensic ontology aligned to CASE/UCO (TrackID's Neo4j projection was removed on 2026-10-27; the schema would be rebuilt as an export from Postgres) | FSI: Digital Investigation | — |

### Venue notes

- No-APC options: FSI, Science & Justice, JFS (AAFS) — all hybrid, free via subscription track.
- JOSS: no APC, reviews the repo itself (license, docs, tests) — pairs with P1/P0.
- OA alternatives if APC is ever funded: SoftwareX, FSI: Synergy.
- Preprint (arXiv/SSRN) + Zenodo DOI regardless of venue, for free-to-read access.
- Check CAPES Portal de Periódicos read-and-publish deals if a university co-author is added.

## Known gaps to close before submission

- No validation / calibration / score→LR mapping (blocks P5).
- No linear sequential unmasking (P2 as a methodological paper). Verification is blind on the
  server since 2026-10-19 (`f8194ed`): the verifier and the third examiner get only the two
  images, no case, names, score or earlier conclusion.
- No real class run yet: the review exercise has only been dry-run with scripted examiners (P2).
- Fingerprint analyses (ACE-V) are not modelled; fingerprints have an automated matcher
  (SourceAFIS sidecar, review band 40/60) but human fingerprint decisions only enter outside
  analyses (limits P2/P4 fingerprint claims). Fingerprint scores are also too easy: no genuine
  rolled pair falls near the threshold.
- Cross-case pairs cannot be distributed for review (P4).
- Both repos are Apache-2.0 with a `CITATION.cff` (2026-10-20). CI is set up in trackid only
  (trackid-sim needs a tagged trackid release first); it runs the database-backed tests,
  which locally only run where `TRACKID_TEST_DATABASE_URL` is set (P1).
- No direct tests for `person`, `graph`, `embedding`, or the frontend.

## Synthetic data {#synthetic-data}

`trackid-sim` (separate repo, private): population, onomastics, face and fingerprint
images, loaded through `identity.Ingest` / `cases.Ingest`; trackid's own pipeline makes the
SYSTEM decisions and students make the human ones; `eval` scores both against ground truth.
Full task list: `trackid-sim/docs/PLAN.md` (research phases) and `docs/ROADMAP.md`
(implementation tasks).

Phases: 0 scaffold (done) → 1 real IBGE name frequencies (done) → ~~2 simulator validation~~ (removed 2026-10-20: the world need not
emulate reality precisely; its parameters are the user's, stated as a limitation) → 3 synthetic face images (done; calibration and per-image
quality open) → 4 synthetic fingerprint images (done; harder rolled impressions open) →
5 review exercise with students (built; awaiting a real class) → 6 public release (Zenodo DOI).

Boundary to keep in every paper: synthetic data validates *system behavior*
(stability, auditability, workflow), not absolute biometric error rates or bias —
those need real images/benchmarks or authorized operational data.

## trackid core issues

- ~~Pair ordering vs collation, missing reverse index~~ — fixed, now part of migration
  005 after the squash (`COLLATE "C"` check, `feature_b_id` index, `biometric_decision_sides` view).
- ~~`cluster.Identify` count~~ — fixed: dedupes by `(clusterID, personID)`, returns
  distinct edges written.
- ~~SYSTEM-only POSITIVE auto-confirms with no human review~~ — addressed by design
  (2026-10-19, migration 019): the rule stays, but clusters come in two layers. AUTOMATIC
  (every CONFIRMED pair) serves intelligence views; REVIEWED (pairs people confirmed) is the
  only layer the case and person pages read. Worth a discussion section in P2: review policy
  vs false merges, and the sim can now score both layers (`trackid-sim eval`).

## Open questions

- Real-data replication: pre-register analysis plan now, execute if/when PF
  authorization is granted (P4, P5).
- University affiliation for OA funding — pending.
