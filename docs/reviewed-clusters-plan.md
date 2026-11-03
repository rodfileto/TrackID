# Reviewed clusters: forensic views show only human-reviewed links

Decided 2026-10-19. Automatic matching stays for fast analysis and intelligence; the case page,
which is where forensic analysis happens, shows only links that people have reviewed.

## Decisions

1. **Reviewed link.** A pair whose deciding round (`cluster.DecidingRound`) is CONFIRMED and was
   settled by people: VERIFICATOR and REVIEWER agreeing, or INCONSISTENCE. A SYSTEM confirmation
   stays automatic even while a human round is pending.
2. **Indirect links** are shown only when every link of the path is reviewed, marked as indirect
   with the path ("via ..."), and with a "request direct comparison" action where a comparison can
   be opened (a face of this case against an enrolled person; cross-case pairs are not
   distributed yet).
3. **Two persisted cluster layers**: `clusters.basis` = `AUTOMATIC` | `REVIEWED`, each reconciled
   by the same `reconcile` step with its own stable ids and merge history. Every reviewed cluster
   sits inside one automatic cluster.
4. **No trace of automatic links to other cases on the case page**, not even a count (contextual
   information that could bias the examiner). Clusters with no reviewed pair are not shown, and
   the panel is hidden when none remain. *Amended 2026-10-19:* the case page also lists every
   enrolled identity each face reached at the review threshold as a decision of the case
   (awaiting review, confirmed, excluded, inconclusive, disputed), hidden from the case's blind
   verifier.
5. **Intelligence views** (Clusters page, overview graph) stay on the AUTOMATIC layer.
6. **Identified persons** on the case page come only from the REVIEWED layer; `Identify` writes
   IDENTIFIED_AS for both layers.
7. **Person profile, person cases and person search badges** read the REVIEWED layer.
8. **Enforced in the API**: the case and person endpoints only ever read REVIEWED clusters.
9. **No automatic toggle for admins** on the case page for now.
10. **Docs**: MODEL.md section 4; book chapters 10 and 11. Closes core issue #3 (SYSTEM-only
    POSITIVE is CONFIRMED) by separating the two views instead of changing the rule.

## Phases

1. `cluster`: `Reviewed(chain)` next to `DeriveEdgeStatus`, with table tests (including SYSTEM
   CONFIRMED in round 0 plus a pending human round).
2. Migration 019: `basis` on `clusters`, `cluster_members` (unique per basis, composite FK) and
   `cluster_merges`. Queries take a basis. `cluster.Run` reconciles both layers; Neo4j nodes carry
   `basis`; `Identify` per layer.
3. `cases.ListCaseClusters` reads the REVIEWED layer; each link is direct (the reviewed decision
   that links it) or indirect (a path of reviewed links). Person reads switch to REVIEWED.
4. Frontend: `CaseClusters` shows reviewed links, indirect paths and the comparison action; the
   Clusters page says it includes automatic matches.
5. Tests (unit + DB-backed with `TRACKID_TEST_DATABASE_URL`), MODEL.md, book.

## Out of scope

- Cross-case comparison requests (needs distribution of cross-case pairs).
- Fingerprint analyses (not modelled yet); fingerprint links reach the REVIEWED layer only through
  human decisions recorded outside analyses.
