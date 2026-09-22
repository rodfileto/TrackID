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
| P0 | Synthetic multimodal forensic corpus + simulator | FSI or Science & Justice (paper) + JOSS (software) | trackid-sim Phase 1–2 |
| P1 | TrackID: extensible open core for forensic biometric case management | SoftwareX | license, tests, demo dataset |
| P2 | ACE-V as an auditable decision graph (blind verification, recorded disagreement) | Science & Justice | blind verification implemented |
| P3 | Citable cluster identity in a continuously revised intelligence database | FSI / Science & Justice (short/technical note) | none — buildable now |
| P4 | Trace-to-trace linkage and unknown-subject clusters (simulation study) | FSI (alt: Crime Science, Policing) | trackid-sim Phase 5 |
| P5 | Human-in-the-loop face triage: thresholds, calibration, demographic differentials | IET Biometrics / FSI | trackid-sim Phase 3 (real images) |
| P6 | Design Science Research: breaking forensic data silos | DESRIST → Government Information Quarterly | Quarto technical-report as base |
| P7 | Auditability as an LGPD safeguard in biometric intelligence systems | FSI: Synergy / Brazilian public-law journal | decision log (done) |
| P8 (optional) | Neo4j schema as a forensic ontology aligned to CASE/UCO | FSI: Digital Investigation | — |

### Venue notes

- No-APC options: FSI, Science & Justice, JFS (AAFS) — all hybrid, free via subscription track.
- JOSS: no APC, reviews the repo itself (license, docs, tests) — pairs with P1/P0.
- OA alternatives if APC is ever funded: SoftwareX, FSI: Synergy.
- Preprint (arXiv/SSRN) + Zenodo DOI regardless of venue, for free-to-read access.
- Check CAPES Portal de Periódicos read-and-publish deals if a university co-author is added.

## Known gaps to close before submission

- No validation / calibration / score→LR mapping (blocks P5).
- Verification is not blind; no linear sequential unmasking (blocks P2 as a methodological paper).
- Fingerprint pipeline is manual-only, no automated matcher (limits P4/P5 fingerprint claims).
- Migration chain has gaps (012→016, bridge migrations) — squash before P1.
- Test coverage thin outside `cluster` package.

## Synthetic data {#synthetic-data}

`trackid-sim` (separate repo, private): population + onomastic + decision simulator,
loads through `identity.Ingest` / `cases.Ingest`, scores clusters against ground truth.
Full task list: `trackid-sim/docs/PLAN.md` (research phases) and `docs/ROADMAP.md`
(implementation tasks).

Phases: 0 scaffold (done) → 1 real IBGE name frequencies → 2 simulator validation
(examiner panel) → 3 synthetic face images → 4 synthetic fingerprint images →
5 policy/revocation scenarios → 6 public release (Zenodo DOI).

Boundary to keep in every paper: synthetic data validates *system behavior*
(stability, auditability, workflow), not absolute biometric error rates or bias —
those need real images/benchmarks or authorized operational data.

## trackid core issues

- ~~Pair ordering vs collation, missing reverse index~~ — fixed in migration 024
  (`COLLATE "C"` check, `feature_b_id` index, `biometric_decision_sides` view).
- ~~`cluster.Identify` count~~ — fixed: dedupes by `(clusterID, personID)`, returns
  distinct edges written.
- SYSTEM-only POSITIVE auto-confirms with no human review (intentional, but worth
  a discussion section in P2).

## Open questions

- Real-data replication: pre-register analysis plan now, execute if/when PF
  authorization is granted (P4, P5).
- University affiliation for OA funding — pending.
