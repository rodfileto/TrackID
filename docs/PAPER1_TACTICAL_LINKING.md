# Paper 1: General Architecture — Person Entity Resolution & Case-Linking

**This document describes the evaluation strategy for Paper 1: the platform's general architecture.** For full system context, see [`docs/DTID_ARCHITECTURE.md`](DTID_ARCHITECTURE.md). Paper 1 introduces the Target/Situation/Event/Entity ontology and the confidence-gated entity-resolution workflow that the rest of the platform builds on; its evaluated capability is face-based `Person` entity resolution and cross-Situation case-linking, which serves as the demonstration vehicle for the architecture rather than the paper's scope limit.

## Title (Working)
**Target-Centric Architecture for Continuous Entity Resolution Across Fragmented Investigative Data**

## 1. Introduction

**Problem**: In public security operations, distinct incidents — a robbery in District A on Monday, a car theft in District B on Wednesday — are investigated in isolated case silos. Relational case-management systems have no mechanism to recognize that an unidentified suspect present in one case is the same person present in another. Manually cross-referencing every suspect across every open case is an intractable combinatorial problem, and vector similarity alone is blind to physical plausibility.

**Positioning**: This paper presents TrackID's general architecture for target-centric intelligence: a knowledge graph built from an explicit ontology — **Target** (the object of interest, e.g. a crime series), **Situation** (a descriptive account of one incident within it), **Event** (a fine-grained occurrence within a Situation), and **Entity** (a typed participant, e.g. `Person`). The paper's evaluated capability is a **Person Entity Profile** — a continuously and collaboratively assembled representation of a single individual identity, fused from fragmented biometric and documentary observations arriving asynchronously as Events across otherwise disconnected Situations. This demonstrates the architecture's core mechanism — confidence-gated entity resolution — without requiring the rest of the platform's capabilities (network analysis in Paper 2, evidentiary verification in Paper 3) to be evaluated here.

**Scoping statement**: This paper covers **the general architecture, plus face-based entity resolution and Person Entity Profile construction** — the ontology, storage design, and the layer that decides which fragmented Event-level observations belong to the same `Person` entity and with what confidence. It does not address network-of-entities analysis or model-of-functioning inference (Paper 2) or court-admissible evidentiary verification (Paper 3).

## 2. Theoretical Positioning

The paper's theoretical anchor is Clark's **target-centric approach** to intelligence analysis: rather than a linear collection → analysis → dissemination pipeline, analysts collaboratively build and continuously refine a networked model of a target. A **Target** is the object of interest — a crime series or scenario, not an individual identity — and this paper's architecture represents it as a knowledge graph of Situations, Events, and Entities. Person entity resolution operates at the Entity level of that ontology: a Person Entity Profile is the continuously-refined record of one `Person` entity as it recurs across a Target's Situations and Events.

This is briefly grounded in two supporting frames rather than developed as separate literatures:

- **JDL data fusion (Level 1 — object refinement)**: the entity-resolution problem this paper solves is a Level 1 fusion problem — combining Event-level observations of an object (a person) into a single, refined estimate of that object's identity and attributes.
- **Forensic intelligence**: the broader field concerned with using investigative-grade information to link cases and generate leads, as distinct from courtroom-grade evidence. This paper operates entirely in that forensic-intelligence register; the evidentiary register is the explicit gap this paper defers to Paper 3.

Together, these frame the paper's contribution: a target-centric architecture with Level 1 fusion as its entity-resolution mechanism and forensic intelligence (not forensic evidence) as its register.

## 3. Related Work

**Primary**: entity resolution and record linkage (probabilistic record linkage; data-matching surveys; entity resolution at scale) and Decision Support System / Design Science Research methodology (DSS design frameworks; DSR methodology for building and evaluating information-systems artifacts) — these are covered in depth, since they are the paper's actual methodological home.

**Secondary**: a short pass over existing systems — commercial OSINT/link-analysis platforms and investigative case-management tools — establishing that, while tools exist for manual link analysis, no open, theoretically-grounded system represents investigative data as an explicit target-centric ontology (Target/Situation/Event/Entity) or is designed to support analysis beyond entity resolution alone. This is a positioning pass, not a competing deep-dive literature track.

## 4. System Design: Person Entity Profile Construction

The system builds a Person Entity Profile through a pipeline that is deliberately generic about its inputs, so that future entity-resolution modalities (gait, license plates, other biometrics) — and future Entity types beyond `Person` — could plug into the same architecture without redesign:

1. **Face-based Entity Resolution Module (ERM)**: facial embeddings (currently InsightFace/ArcFace, treated as a commodity component) are extracted from incoming Events (face-capture observations) and indexed for similarity search as they arrive.
2. **Spatio-temporal plausibility**: candidate matches are filtered against a physical plausibility constraint — a match implying an impossible travel speed between two Event locations is discounted or rejected regardless of embedding similarity.
3. **Tripartite routing**: surviving candidates are routed by confidence — auto-merged into the Person Entity Profile at high confidence, queued for a one-click analyst validation at intermediate confidence, or silently discarded at low confidence.
4. **Multi-source evidence ledger**: a Person Entity Profile is not the output of a single match type but an accumulating ledger of evidence from distinct sources — biometric auto-match (system-generated), analyst validation (human-confirmed tactical queue resolution), and field-officer document confirmation (an officer confirming identity against a physical document at the point of contact). Each entry is provenance-tagged; the profile's overall confidence reflects the combination of evidence it has accumulated, not a single score.
5. **Resulting Person Entity Profile**: the continuously-updated record for one `Person` entity, referenced by every Event and Situation it participates in, that other analyses of the platform (network-of-entities analysis, evidentiary escalation) consume as an input.

## 5. Simulation & Evaluation Design

Because this paper's claims are about a routing/entity-resolution architecture rather than a piece of forensic evidence, evaluation is simulation-based and requires no human subjects or ethics review.

**Layered synthetic scenario**: the simulation instantiates the ontology directly — a synthetic **Target** ("a bank-robbery series in region X"), composed of synthetic **Situations** (individual robberies), each with several **Events** (camera captures, field notes) referencing **Entities** (`Person` suspects, `Bank` locations). This is built in three layers to stress-test entity resolution and routing specifically — it does not attempt to validate the resulting entity network's structure, which is left to Paper 2:

- **Entity layer**: real identities and observations drawn from a standard public multi-camera person re-identification benchmark (Market-1501 or MSMT17), providing ground-truth identity labels for the `Person` entities.
- **Situation layer**: a synthetic robbery series (the Target) is overlaid on the entity layer, with individual robberies as Situations — used to structure the simulation into realistic case groupings, not to claim any network- or functioning-level result.
- **Event / spatial-temporal layer**: camera locations and timestamps are synthesized as Events to drive the spatio-temporal plausibility gate.

Observations are fragmented into isolated Situations and fed to the system asynchronously, as if arriving from independent investigations; the system's Auto-Merge / Tactical Queue / Auto-Reject output and resulting evidence ledger are compared against the withheld ground truth.

## 6. Results

Reported along three axes, all identity-level:

- **Silo-breaking rate**: the fraction of true cross-case identity links recovered (via auto-merge and tactical-queue routing combined), with precision/recall broken out by routing tier.
- **Routing accuracy**: how well the tripartite routing separates genuinely ambiguous candidates from confident merges and confident rejections.
- **Workload compression**: the reduction from an intractable pairwise comparison problem to a short, linear queue of proposals requiring human validation.
- **Threshold sensitivity**: all of the above repeated across conservative/moderate/aggressive threshold settings, showing threshold choice is a tunable policy trade-off rather than a fixed optimum.

## 7. Discussion

**DSS-centric interpretation**: the paper's central claim is architectural, not about raw matching accuracy — it changes the shape of the analyst's task from an intractable search problem into a short validation queue, and does so by fusing evidence from multiple sources rather than trusting any single similarity score.

**Toward an Integrated Platform**: this section is explicitly bounded to preview, not claim, the rest of the platform's capability. The Person Entity Profiles this paper produces populate the entity network for a Target — the input that Paper 2 analyzes via network/topology methods to surface a Model of Functioning — and are the object that a separate, deliberately slower verification tier can escalate into court-admissible evidence (Paper 3). Neither claim is evaluated here.

## 8. Limitations & Future Work

- Evaluation uses a public re-identification benchmark plus a synthetic Target/Situation/Event overlay as a proxy for real fragmented investigations, not an operational deployment.
- The multi-source evidence ledger currently combines biometric auto-match, analyst validation, and field-officer document confirmation; other evidence sources are left for future extension.
- This paper does not evaluate whether the resulting network of entities supports accurate model-of-functioning inference — that hand-off is explicitly Paper 2's contribution.
- This paper does not address chain-of-custody, court-admissible identification, or examiner protocols for treating a Person Entity Profile as legal evidence — that hand-off is explicitly Paper 3's contribution.

## 9. Conclusion

This paper demonstrates that a target-centric architecture — an explicit ontology of Target, Situation, Event, and Entity, stored as a knowledge graph — can continuously and asynchronously construct Person Entity Profiles from fragmented, multi-source investigative data via confidence-gated entity resolution, evaluated entirely by simulation. The Person Entity Profile is positioned as the foundational building block the rest of the platform uses: its entity network is analyzed for a Model of Functioning in Paper 2, and its identifications are escalated to evidentiary-grade status in Paper 3.
