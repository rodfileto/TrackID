# TrackID Research & Publication Strategy

## Overview

TrackID's research program is organized around a single theoretical spine — the **target-centric approach to intelligence analysis** (Clark) — applied recursively across a hierarchy of targets. The central move is a recursion argument: a "target" in the target-centric sense need not be a large strategic object (an organization, a network); it can recurse all the way down to a single identity. Each paper in the arc operationalizes that recursion at a different level of the hierarchy, with its own evaluation register:

1. **Paper 1 (Person-Target Profiles)**: the target is an individual identity. Built from face-based entity resolution, spatio-temporal plausibility, tripartite routing, and a multi-source evidence ledger. Fast, DSS-focused, no human-subjects testing required.
2. **Paper 2 (Situation/Network-Target Profiles)**: the target is a criminal series or organizational structure, composed from the Person-Target Profiles Paper 1 produces by applying network/topology analysis over them. Introduces "situations" as first-class entities.
3. **Paper 3 (Evidentiary-Grade Verification)**: addresses the forensic/legal gap explicitly deferred by Paper 1 — chain-of-custody, court-admissible identification, and examiner protocols for confirming a Person-Target Profile as legal evidence rather than a tactical lead.

A single software platform supports all three papers, but they are published as separate, focused contributions to avoid the "kitchen sink paper" trap.

## Why Separate Papers?

Combining these problems in a single manuscript fails peer review because:

- **Schizophrenic literature review**: defending entity resolution, network science, and forensic/legal compliance simultaneously requires covering information retrieval, complex networks, cognitive psychology, and forensic protocol standards all at once — reviewers flag this as unfocused.
- **Conflicting evaluation strategies**: Paper 1's claims are provable via simulation on public benchmarks (no ethics review needed). Paper 2's claims are graph-topological. Paper 3's claims are behavioral and require a human-subjects examiner study (ethics review required). A single methodology section can't serve all three registers — IS/DSS metrics, network/graph metrics, and forensic/legal validity criteria are genuinely different kinds of evidence.
- **Reviewer mismatch**: a DSS/IS reviewer will ask you to cut the legal/forensic detail; a forensic-science reviewer will ask you to cut the graph math; a network-science reviewer will ask you to cut the workflow/UI discussion.

**Solution**: split into three papers, each with surgical depth in its domain, published in sequence, sharing one theoretical anchor.

## The Target Hierarchy

Target-centric analysis (Clark) reframes intelligence work as the collaborative, continuous construction of a networked model of a target, rather than a linear collection → analysis → dissemination pipeline. This project's contribution is applying that same pattern recursively at three levels, where each level's resolved target becomes an input node for the next:

- **Identity level** (Paper 1): resolving *who* — fragmented biometric and documentary observations are fused into a single Person-Target Profile.
- **Situation/network level** (Paper 2): resolving *how the identities relate* — Person-Target Profiles are composed, via co-occurrence and topology, into a Network-Target Profile describing a criminal series or organizational structure.
- **Evidentiary level** (Paper 3): resolving *whether a specific identification can bear legal weight* — a Person-Target Profile is escalated from an operational lead to a court-admissible identification through a structured verification protocol.

## Paper Summaries

### Paper 1: Person-Target Profiles

Individual identity as the target. A Decision Support System that continuously and asynchronously resolves identities across fragmented, siloed investigative data via face-based entity resolution, spatio-temporal plausibility filtering, tripartite confidence routing, and a multi-source evidence ledger (biometric auto-match, analyst validation, field-officer document confirmation). Scope is strictly the entity-resolution layer — evaluated via simulation, no human-subjects testing required. See `docs/PAPER1_TACTICAL_LINKING.md`.

### Paper 2: Situation/Network-Target Profiles

A criminal series or organizational structure as the target. Applies network/topology analysis (community detection, centrality, temporal dynamics) over the Person-Target Profiles Paper 1 produces, treating "situations" — case clusters, series, operational structures — as first-class entities that compose from person-level targets. See `docs/PAPER2_INTELLIGENCE.md`.

### Paper 3: Evidentiary-Grade Verification

Whether a Person-Target Profile can stand as legal evidence, not merely a tactical lead, as the target. Addresses chain-of-custody, court-admissible identification, and structured examiner protocols, requiring a human-subjects examiner study. See `docs/PAPER3_FORENSIC_EVIDENTIARY.md`.

## Single Platform Strategy

**The code is not split.** The platform stays a single, unified codebase; the separation across papers is conceptual and publication-driven, not architectural.

### Citation Chain

Each paper scopes itself explicitly and cites the others rather than re-litigating their contributions:

- **Paper 1** scopes itself to identity-level entity resolution and notes the platform also supports network-level analysis and evidentiary verification, without claiming results for either.
- **Paper 2** cites Paper 1 for the entity-resolution layer that produces its input Person-Target Profiles, and focuses on what composing them into networks reveals.
- **Paper 3** cites Paper 1 for the tactical-lead escalation path, and addresses the complementary question of what verification a lead needs before it can become evidence.

### Code Coverage

| Component | Paper 1 | Paper 2 | Paper 3 |
|-----------|---------|---------|---------|
| Face-based entity resolution | Core | Input | Input |
| Spatio-temporal plausibility gate | Core | Excluded | Excluded |
| Tripartite routing / evidence ledger | Core | Excluded | Input (leads) |
| Network/topology analysis over Person-Target Profiles | Excluded | Core | Excluded |
| Situation composition & community/centrality analysis | Excluded | Core | Excluded |
| Chain-of-custody & examiner verification workflow | Excluded | Excluded | Core |

## Publication Roadmap

Roughly sequential with overlap: Paper 1 first (simulation-only evaluation, shortest path to submission), Paper 2 in parallel once Paper 1's entity-resolution output is stable (its input depends on Paper 1's method, not its publication status), Paper 3 last, gated on IRB/ethics approval for the examiner study. Target venues differ by evaluation register — DSS/IS venues for Paper 1, network-science/applied-AI venues for Paper 2, forensic-science/HCI venues for Paper 3 — see each paper's own document for specifics.

Follow-up work beyond the three-paper arc (robustness under demographic variation, real-time graph updates, privacy-preserving deployment, cross-jurisdictional calibration) is left open rather than pre-scoped.

## Key Messaging

- **Paper 1**: proves that spatio-temporal-constrained entity resolution plus tripartite uncertainty routing turns an intractable cross-case search problem into a short validation queue, evaluated entirely by simulation. Network-level and evidentiary-level capabilities are orthogonal and out of scope.
- **Paper 2**: demonstrates that network analysis applied to the identities Paper 1 resolves can reveal organizational structure that no individual case file exposes, treating situations as targets composed from person-level targets.
- **Paper 3**: demonstrates that structured, protocol-enforced verification mitigates automation bias relative to unstructured review, addressing what happens when a Person-Target Profile must become court-admissible evidence.
- **For contributors**: one platform, one theoretical spine (target-centric recursion), three levels of target — identity, situation/network, evidentiary record. Read the papers for research depth; the code shows how each level's output feeds the next.

## References & Related Work

See `docs/PAPER1_TACTICAL_LINKING.md`, `docs/PAPER2_INTELLIGENCE.md`, and `docs/PAPER3_FORENSIC_EVIDENTIARY.md` for detailed literature reviews, methodology, and metrics for each paper.
