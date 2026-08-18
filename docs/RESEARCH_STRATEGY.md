# DTID Research & Publication Strategy

## DTID Architecture Overview

**DTID** (Distributed Target-Centric Identity Platform) is a unified, event-sourced graph system that treats identity, networks, and evidentiary records as distinct recursive instances of the **target-centric approach to intelligence analysis** (Clark). The architecture applies target-centric thinking at three levels:

1. **Identity Tier**: resolving *who* — a single individual identity, continuously constructed from fragmented biometric and documentary observations via face-based entity resolution, spatio-temporal plausibility filtering, and a confidence-gated workflow that auto-commits, queues, or discards matches.
2. **Network Tier**: resolving *how identities relate* — criminal series and organizational structures, composed from resolved identities via spatio-temporal network analysis (community detection, centrality, link prediction).
3. **Evidentiary Tier**: resolving *whether an identification can bear legal weight* — Person-Target Profiles escalated from operational intelligence to court-admissible evidence through structured, protocol-enforced verification.

The platform uses a single event-sourced graph (Memgraph/Cypher) + relational store (PostgreSQL + pgvector) architecture, with append-only immutability, confidence-gated routing, dual-mode delivery (live alerts + static dossier snapshots), and governance baked into the schema (retention policies, cryptographic signing, jurisdictional compliance). See [`docs/DTID_ARCHITECTURE.md`](DTID_ARCHITECTURE.md) for full technical details.

All three tiers run in a single codebase; they are published and evaluated separately to avoid the "kitchen sink paper" trap.

## Evaluation Registers: Why Separate Papers?

DTID's three tiers require fundamentally different evaluation methodologies. Combining them in a single manuscript fails peer review:

- **Schizophrenic literature review & methodology**: Entity resolution, network science, and forensic/legal compliance require defending information retrieval, complex networks, cognitive psychology, and forensic protocol standards simultaneously — incompatible in a single literature review and methodology section.
- **Conflicting evaluation criteria**: 
  - Identity tier claims are provable via **DSS/IS metrics** on public benchmarks (silo-breaking rate, routing accuracy, workload compression); no ethics review needed.
  - Network tier claims require **graph-topological validation** (community detection quality, centrality ranking, link-prediction metrics).
  - Evidentiary tier claims require **behavioral human-subjects study** (examiner automation-bias reduction, inter-rater agreement, false-positive error rates); IRB/ethics approval required.
  - A single paper cannot credibly serve all three registers.
- **Reviewer mismatch**: a DSS/IS reviewer asks to cut legal/forensic detail; a forensic-science reviewer asks to cut the graph math; a network-science reviewer asks to cut the workflow discussion.

**Solution**: three focused papers, each with surgical depth in its domain, sharing one theoretical anchor (target-centric recursion). All three tiers run in a single platform; publication separation is strategic, not architectural.

## Paper Scopes & Evaluation Registers

Each paper evaluates one tier of the DTID architecture with its own, domain-appropriate evaluation methodology:

### Paper 1: Identity Tier — Person-Target Profiles

**Register**: Decision Support System (DSS) / Information Systems (IS) evaluation.

Evaluates the identity-resolution tier: does a confidence-gated entity-resolution architecture reduce an intractable cross-case search problem to a tractable analyst validation queue? Evaluation is simulation-based (public re-ID benchmark + synthetic case overlay), no human-subjects testing required. Metrics: silo-breaking rate (precision/recall), routing accuracy (how well τ_high/τ_low separate candidates), workload compression (reduction in pairwise comparisons). See [`docs/PAPER1_IDENTITY_TIER_EVALUATION.md`](PAPER1_TACTICAL_LINKING.md).

### Paper 2: Network Tier — Situation/Network-Target Profiles

**Register**: Graph-topological (network-science / complex-systems) evaluation.

Evaluates the network-analysis tier: does composing identity-resolved profiles into spatio-temporal networks reveal organizational structure invisible in individual case files? Evaluation is graph-topological — validating community detection, centrality measures, and link prediction against synthetic planted communities and known organizational structure. Metrics: modularity, centrality ranking agreement, link-prediction precision/recall. See [`docs/PAPER2_NETWORK_TIER_EVALUATION.md`](PAPER2_INTELLIGENCE.md).

### Paper 3: Evidentiary Tier — Evidentiary-Grade Verification

**Register**: Behavioral / Human-Computer Interaction (HCI) / Forensic-Science evaluation.

Evaluates the evidentiary-verification tier: does structured, protocol-enforced verification mitigate automation bias and reduce false-positive identifications? Evaluation is behavioral — human-subjects examiner study comparing structured review (checklist-driven, blind dual-expert) against unstructured baseline. Metrics: automation-bias reduction (system suggestion acceptance shift), inter-examiner agreement (Cohen's kappa), false-positive error rate, decision time. Requires IRB/ethics approval. See [`docs/PAPER3_EVIDENTIARY_TIER_EVALUATION.md`](PAPER3_FORENSIC_EVIDENTIARY.md).

## Unified Platform, Separate Evaluation Registers

**The code is not split.** The platform is a single, unified codebase; the separation across papers is evaluation-register-driven, not architectural. All three tiers coexist and share the same event-sourced graph + relational infrastructure.

### Citation Chain

Each paper scopes itself to one tier and cites the others for the tiers it depends on:

- **Paper 1** evaluates identity-level entity resolution (confidence-gated routing, multi-source evidence ledger) via DSS metrics and simulation. It notes the platform also supports network-tier and evidentiary-tier capabilities (Papers 2–3) but does not claim results for either.
- **Paper 2** cites Paper 1 for the Person-Target Profile outputs that become network nodes; it focuses entirely on the graph-topological evaluation of network-tier composition and does not evaluate identity-resolution quality or evidentiary verification.
- **Paper 3** cites Paper 1 for the tactical-lead escalation path (which tiers supply candidates); it focuses on the behavioral/forensic evaluation of structured verification and does not evaluate identity resolution or network analysis.

### Deployment in DTID Architecture

| Component | Paper 1 | Paper 2 | Paper 3 |
|-----------|---------|---------|---------|
| Face-based entity resolution + embedding | Core eval | Depends on | Depends on |
| Spatio-temporal plausibility gate | Core eval | Not eval'd | Not eval'd |
| Confidence-gated routing (τ_high, τ_low, queue) | Core eval | Not eval'd | Input (candidates) |
| Multi-source evidence ledger | Core eval | Depends on | Depends on |
| Memgraph + PostgreSQL event-sourced architecture | Underlying | Underlying | Underlying |
| Network/topology analysis over profiles | Not eval'd | Core eval | Not eval'd |
| Situation composition & community/centrality | Not eval'd | Core eval | Not eval'd |
| Structured verification protocol & dual-expert review | Not eval'd | Not eval'd | Core eval |
| Chain-of-custody & evidentiary signing | Not eval'd | Not eval'd | Core eval |

## Publication Roadmap

Submission sequence follows evaluation-register dependencies and ethics gatekeeping:

1. **Paper 1 (Identity Tier)** — submitted first (simulation-only, fastest path to completion).
2. **Paper 2 (Network Tier)** — submitted in parallel with Paper 1's submission (depends on entity-resolution methodology, not its publication; can be developed concurrently).
3. **Paper 3 (Evidentiary Tier)** — submitted last (gated on IRB/ethics approval for human-subjects examiner study; significant review timeline).

Target venues by evaluation register:
- **Paper 1**: DSS/IS/MIS conferences and journals (Decision Support Systems, Journal of Information Systems, etc.)
- **Paper 2**: Network-science, complex-systems, and applied-AI venues (Social Network Analysis and Mining, Applied Network Science, etc.)
- **Paper 3**: Forensic-science and HCI venues (Forensic Science International, Journal of Forensic Sciences, ACM CHI, etc.)

**Future directions** (beyond the three-paper arc, not pre-scoped):
- Robustness under demographic variation (fairness, bias mitigation across geographic/demographic groups)
- Real-time graph updates and query optimization for larger deployments
- Privacy-preserving deployment (federated inference, differential privacy)
- Cross-jurisdictional calibration (multi-agency threshold policy harmonization)
- Swappable Re-ID models (integration of alternative embedding methods)

## Key Messaging

**For Technical Audiences (Developers, Researchers)**:
- **Paper 1 (Identity Tier)**: Proves that spatio-temporal-constrained entity resolution plus tripartite confidence routing turns an intractable cross-case search problem into a short validation queue. DSS metrics validate the claim; simulation evaluation; no human-subjects testing.
- **Paper 2 (Network Tier)**: Demonstrates that network analysis applied to identity-resolved profiles reveals organizational structure (operational cells, key persons, relationship evolution) invisible in individual case files. Graph-topological evaluation validates the claim.
- **Paper 3 (Evidentiary Tier)**: Demonstrates that structured, protocol-enforced verification (blind dual-expert review, checklist-driven comparison) mitigates automation bias and reduces false-positive identifications relative to unstructured review. Behavioral study (human-subjects examiner evaluation) validates the claim.

**For Contributors & Operators**:
- One platform (DTID), one theoretical spine (target-centric recursion applied at three target levels: identity, situation/network, evidentiary record).
- Single event-sourced graph (Memgraph) + relational store (PostgreSQL); all three tiers coexist.
- Each tier has its own evaluation register, publication venue, and research audience — but they feed into one cohesive architecture.
- Governance, retention policies, cryptographic signing, and jurisdictional compliance are baked into the data model, not bolted on.
- Read the papers for research-register depth; read [`docs/DTID_ARCHITECTURE.md`](DTID_ARCHITECTURE.md) for full system context.

## References & Related Work

For detailed literature reviews, methodology, and evaluation metrics, see:
- [`docs/PAPER1_IDENTITY_TIER_EVALUATION.md`](PAPER1_TACTICAL_LINKING.md) — DSS/IS literature, entity-resolution methods, decision-support evaluation
- [`docs/PAPER2_NETWORK_TIER_EVALUATION.md`](PAPER2_INTELLIGENCE.md) — Network-science literature, community detection, centrality, link prediction
- [`docs/PAPER3_EVIDENTIARY_TIER_EVALUATION.md`](PAPER3_FORENSIC_EVIDENTIARY.md) — Forensic-science literature, dual-process theory, automation bias, examiner protocols
