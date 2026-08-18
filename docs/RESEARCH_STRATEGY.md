# DTID Research & Publication Strategy

## DTID Architecture Overview

**DTID** is a target-centric intelligence platform built on Clark's **target-centric approach to intelligence analysis**: a **Target System** (Clark's macro target) is simply the object of interest — a crime series, an organization, a scenario — represented as an instance of a small ontology (**Target → Situation → Event → Target Entity**) stored in a knowledge graph. A **Target Entity** (micro target) is a specific node within that graph that field officers interact with operationally — `TargetPerson`, `Vehicle`, `PhoneNumber`. Keeping these two senses of "Target" distinct matters for reviewers: the platform is the software bridge between them, ingesting observations about discrete Target Entities and automatically constructing the broader Target System. Two distinct analytical representations are built on top of any given Target System: a **network of entities** (the raw structure of who/what was involved and how they relate) and a **model of functioning** (a higher-level account of roles and operational pattern). Facial recognition is the mechanism the platform uses to resolve `TargetPerson` entities across the Events and Situations that make up a Target System — which is what allows a suspect observed in one incident to be recognized as the same person observed in another.

The platform uses a single knowledge graph (Memgraph/Cypher) + relational store (PostgreSQL + pgvector) architecture, with append-only immutability, confidence-gated entity resolution, dual-mode delivery (live alerts + static dossier snapshots), and governance baked into the schema (retention policies, cryptographic signing, jurisdictional compliance). See [`docs/DTID_ARCHITECTURE.md`](DTID_ARCHITECTURE.md) for full technical details.

The platform runs as a single codebase; three papers evaluate different parts of it separately to avoid the "kitchen sink paper" trap.

## Evaluation Registers: Why Separate Papers?

The three papers require fundamentally different evaluation methodologies. Combining them in a single manuscript fails peer review:

- **Schizophrenic literature review & methodology**: Entity resolution, network science, and forensic/legal compliance require defending information retrieval, complex networks, cognitive psychology, and forensic protocol standards simultaneously — incompatible in a single literature review and methodology section.
- **Conflicting evaluation criteria**:
  - Paper 1's architecture claims are provable via **DSS/IS metrics** on public benchmarks (silo-breaking rate, routing accuracy, workload compression); no ethics review needed.
  - Paper 2's network-analysis claims require **graph-topological validation** (community detection quality, centrality ranking, link-prediction metrics).
  - Paper 3's verification claims require a **behavioral human-subjects study** (examiner automation-bias reduction, inter-rater agreement, false-positive error rates); IRB/ethics approval required.
  - A single paper cannot credibly serve all three registers.
- **Reviewer mismatch**: a DSS/IS reviewer asks to cut legal/forensic detail; a forensic-science reviewer asks to cut the graph math; a network-science reviewer asks to cut the workflow discussion.

**Solution**: three focused papers, each with surgical depth in its domain, sharing one theoretical anchor (the target-centric ontology). All three evaluate one platform; publication separation is strategic, not architectural.

## Paper Scopes & Evaluation Registers

### Paper 1: General Architecture — Person Entity Resolution & Case-Linking

**Register**: Decision Support System (DSS) / Information Systems (IS) evaluation.

Introduces the platform's general architecture: the Target/Situation/Event/Target Entity ontology (including the Target System vs. Target Entity distinction), the knowledge-graph + relational storage design, and the confidence-gated entity-resolution workflow. The paper is **not** scoped narrowly to case-linking — case-linking is the demonstrated capability used to evaluate the architecture, via face-based `TargetPerson` resolution within a simulated Target System (a synthetic bank-robbery series). Evaluation is simulation-based (public re-ID benchmark + synthetic Situation/Event overlay), no human-subjects testing required. Metrics: silo-breaking rate (precision/recall), routing accuracy (how well τ_high/τ_low separate candidates), workload compression (reduction in pairwise comparisons). See [`docs/PAPER1_TACTICAL_LINKING.md`](PAPER1_TACTICAL_LINKING.md).

### Paper 2: Network Analysis — Surfacing a Target's Model of Functioning

**Register**: Graph-topological (network-science / complex-systems) evaluation.

Takes the network of Target Entities that Paper 1's architecture produces for a Target System and analyzes it (community detection, centrality, temporal decay, link prediction) to surface that Target System's **Model of Functioning** — operational cells, key persons, and how the operation evolves over time. Evaluation is graph-topological — validating community detection, centrality measures, and link prediction against synthetic planted communities and known organizational structure. Metrics: modularity, centrality ranking agreement, link-prediction precision/recall. See [`docs/PAPER2_INTELLIGENCE.md`](PAPER2_INTELLIGENCE.md).

### Paper 3: Evidentiary Verification

**Register**: Behavioral / Human-Computer Interaction (HCI) / Forensic-Science evaluation.

Addresses when a resolved `TargetPerson`'s identification must be escalated from an operational lead to court-admissible evidence, via structured, protocol-enforced verification. Evaluation is behavioral — a human-subjects examiner study comparing structured review (checklist-driven, blind dual-expert) against an unstructured baseline. Metrics: automation-bias reduction (system suggestion acceptance shift), inter-examiner agreement (Cohen's kappa), false-positive error rate, decision time. Requires IRB/ethics approval. See [`docs/PAPER3_FORENSIC_EVIDENTIARY.md`](PAPER3_FORENSIC_EVIDENTIARY.md).

## Unified Platform, Separate Evaluation Registers

**The code is not split.** The platform is a single, unified codebase; the separation across papers is evaluation-register-driven, not architectural. All three papers evaluate parts of the same knowledge graph + relational infrastructure.

### Citation Chain

Each paper scopes itself to one contribution and cites the others for what it depends on:

- **Paper 1** evaluates the general architecture via `TargetPerson` resolution (confidence-gated routing, multi-source evidence ledger) using DSS metrics and simulation. It notes the platform also supports network analysis and evidentiary verification (Papers 2–3) but does not claim results for either.
- **Paper 2** cites Paper 1 for the Person Entity Profile outputs that populate the entity network; it focuses entirely on the graph-topological evaluation of that network and does not evaluate entity-resolution quality or evidentiary verification.
- **Paper 3** cites Paper 1 for the operational-lead outputs that feed the verification workflow; it focuses on the behavioral/forensic evaluation of structured verification and does not evaluate entity resolution or network analysis.

### Deployment in DTID Architecture

| Component | Paper 1 | Paper 2 | Paper 3 |
|-----------|---------|---------|---------|
| Target/Situation/Event/Target Entity ontology + knowledge graph | Core eval | Underlying | Underlying |
| Face-based TargetPerson resolution + embedding | Core eval | Depends on | Depends on |
| Spatio-temporal plausibility gate | Core eval | Not eval'd | Not eval'd |
| Confidence-gated routing (τ_high, τ_low, queue) | Core eval | Not eval'd | Input (candidates) |
| Multi-source evidence ledger (Person Entity Profile) | Core eval | Depends on | Depends on |
| Memgraph + PostgreSQL architecture | Underlying | Underlying | Underlying |
| Network-of-entities analysis (community/centrality/decay) | Not eval'd | Core eval | Not eval'd |
| Model-of-functioning inference | Not eval'd | Core eval | Not eval'd |
| Structured verification protocol & dual-expert review | Not eval'd | Not eval'd | Core eval |
| Chain-of-custody & evidentiary signing | Not eval'd | Not eval'd | Core eval |

## Publication Roadmap

Submission sequence follows evaluation-register dependencies and ethics gatekeeping:

1. **Paper 1 (General Architecture)** — submitted first (simulation-only, fastest path to completion).
2. **Paper 2 (Network Analysis)** — submitted in parallel with Paper 1's submission (depends on the entity-resolution methodology, not its publication; can be developed concurrently).
3. **Paper 3 (Evidentiary Verification)** — submitted last (gated on IRB/ethics approval for the human-subjects examiner study; significant review timeline).

Target venues by evaluation register:
- **Paper 1**: DSS/IS/MIS conferences and journals (Decision Support Systems, Journal of Information Systems, etc.)
- **Paper 2**: Network-science, complex-systems, and applied-AI venues (Social Network Analysis and Mining, Applied Network Science, etc.)
- **Paper 3**: Forensic-science and HCI venues (Forensic Science International, Journal of Forensic Sciences, ACM CHI, etc.)

**Future directions** (beyond the three-paper arc, not pre-scoped):
- Robustness under demographic variation (fairness, bias mitigation across geographic/demographic groups)
- Real-time graph updates and query optimization for larger deployments
- Privacy-preserving deployment (federated inference, differential privacy)
- Cross-jurisdictional calibration (multi-agency threshold policy harmonization)
- Additional Target Entity types and resolution modalities beyond `TargetPerson` (vehicles, gait, license plates)

## Key Messaging

**For Technical Audiences (Developers, Researchers)**:
- **Paper 1 (General Architecture)**: Introduces the Target/Situation/Event/Target Entity ontology and confidence-gated entity-resolution workflow, evaluated via face-based `TargetPerson` resolution and cross-Situation case-linking. DSS metrics validate the claim; simulation evaluation; no human-subjects testing.
- **Paper 2 (Network Analysis)**: Demonstrates that analyzing a Target System's entity network surfaces its Model of Functioning — operational cells, key persons, relationship evolution — invisible in individual case files. Graph-topological evaluation validates the claim.
- **Paper 3 (Evidentiary Verification)**: Demonstrates that structured, protocol-enforced verification (blind dual-expert review, checklist-driven comparison) mitigates automation bias and reduces false-positive identifications relative to unstructured review. Behavioral study (human-subjects examiner evaluation) validates the claim.

**For Contributors & Operators**:
- One platform (DTID), one ontology (Target/Situation/Event/Target Entity), one Target System / Target Entity distinction, two derived analytical layers per Target System (network of entities; model of functioning).
- Single knowledge graph (Memgraph) + relational store (PostgreSQL); all three papers evaluate parts of the same system.
- Each paper has its own evaluation register, publication venue, and research audience — but they feed into one cohesive architecture.
- Governance, retention policies, cryptographic signing, and jurisdictional compliance are baked into the data model, not bolted on.
- Read the papers for research-register depth; read [`docs/DTID_ARCHITECTURE.md`](DTID_ARCHITECTURE.md) for full system context.

## References & Related Work

For detailed literature reviews, methodology, and evaluation metrics, see:
- [`docs/PAPER1_TACTICAL_LINKING.md`](PAPER1_TACTICAL_LINKING.md) — DSS/IS literature, entity-resolution methods, decision-support evaluation
- [`docs/PAPER2_INTELLIGENCE.md`](PAPER2_INTELLIGENCE.md) — Network-science literature, community detection, centrality, link prediction
- [`docs/PAPER3_FORENSIC_EVIDENTIARY.md`](PAPER3_FORENSIC_EVIDENTIARY.md) — Forensic-science literature, dual-process theory, automation bias, examiner protocols
