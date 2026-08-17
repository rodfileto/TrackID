# Paper 1: Person-Target Profiles (Face-Based Entity Resolution)

## Title (Working)
**Person-Target Profiles: A Target-Centric Decision Support Architecture for Continuous Entity Resolution Across Fragmented Investigative Data**

## 1. Introduction

**Problem**: In public security operations, distinct incidents — a robbery in District A on Monday, a car theft in District B on Wednesday — are investigated in isolated case silos. Relational case-management systems have no mechanism to recognize that an unidentified suspect present in one case is the same person present in another. Manually cross-referencing every suspect across every open case is an intractable combinatorial problem, and vector similarity alone is blind to physical plausibility.

**Positioning**: This paper presents TrackID's approach to building a **Person-Target Profile** — a continuously and collaboratively assembled representation of a single individual identity, fused from fragmented biometric and documentary observations arriving asynchronously across otherwise disconnected cases. It is the identity-level instance of a target-centric platform whose target hierarchy extends beyond individuals to situations/networks and evidentiary records (see `docs/RESEARCH_STRATEGY.md`); this paper is scoped to the identity level alone.

**Scoping statement**: This paper covers **face-based entity resolution and Person-Target Profile construction only** — the layer that decides which fragmented observations belong to the same identity and with what confidence. It does not address network/situation-level composition (Paper 2) or court-admissible evidentiary verification (Paper 3).

## 2. Theoretical Positioning

The paper's theoretical anchor is Clark's **target-centric approach** to intelligence analysis: rather than a linear collection → analysis → dissemination pipeline, analysts collaboratively build and continuously refine a networked model of a target. The paper's central theoretical move is a **recursion argument**: a target in this sense need not be a large strategic object — it can recurse down to a single identity. A Person-Target Profile is exactly that: a target-centric model built at the smallest unit of the hierarchy.

This is briefly grounded in two supporting frames rather than developed as separate literatures:

- **JDL data fusion (Level 1 — object refinement)**: the identity-resolution problem this paper solves is a Level 1 fusion problem — combining observations of an object (a person) into a single, refined estimate of that object's identity and attributes.
- **Forensic intelligence**: the broader field concerned with using investigative-grade information to link cases and generate leads, as distinct from courtroom-grade evidence. This paper operates entirely in that forensic-intelligence register; the evidentiary register is the explicit gap this paper defers to Paper 3.

Together, these frame the paper's contribution: a target-centric system for the identity level of analysis, with Level 1 fusion as its mechanism and forensic intelligence (not forensic evidence) as its register.

## 3. Related Work

**Primary**: entity resolution and record linkage (probabilistic record linkage; data-matching surveys; entity resolution at scale) and Decision Support System / Design Science Research methodology (DSS design frameworks; DSR methodology for building and evaluating information-systems artifacts) — these are covered in depth, since they are the paper's actual methodological home.

**Secondary**: a short pass over existing systems — commercial OSINT/link-analysis platforms and investigative case-management tools — establishing that, while tools exist for manual link analysis, no open, theoretically-grounded system extends a target-centric model down to the identity level or is designed to be extensible to higher levels of the target hierarchy. This is a positioning pass, not a competing deep-dive literature track.

## 4. System Design: Person-Target Profile Construction

The system builds a Person-Target Profile through a pipeline that is deliberately generic about its inputs, so that future entity-resolution modalities (gait, license plates, other biometrics) could plug into the same architecture without redesign:

1. **Face-based Entity Resolution Module (ERM)**: facial embeddings (currently InsightFace/ArcFace, treated as a commodity component) are extracted from incoming observations and indexed for similarity search as they arrive.
2. **Spatio-temporal plausibility**: candidate matches are filtered against a physical plausibility constraint — a match implying an impossible travel speed between two observation sites is discounted or rejected regardless of embedding similarity.
3. **Tripartite routing**: surviving candidates are routed by confidence — auto-merged into the profile at high confidence, queued for a one-click analyst validation at intermediate confidence, or silently discarded at low confidence.
4. **Multi-source evidence ledger**: a Person-Target Profile is not the output of a single match type but an accumulating ledger of evidence from distinct sources — biometric auto-match (system-generated), analyst validation (human-confirmed tactical queue resolution), and field-officer document confirmation (an officer confirming identity against a physical document at the point of contact). Each entry is provenance-tagged; the profile's overall confidence reflects the combination of evidence it has accumulated, not a single score.
5. **Resulting Person-Target Profile**: the continuously-updated identity record that other layers of the platform (situation/network composition, evidentiary escalation) consume as an input.

## 5. Simulation & Evaluation Design

Because this paper's claims are about a routing/entity-resolution architecture rather than a piece of forensic evidence, evaluation is simulation-based and requires no human subjects or ethics review.

**Layered synthetic scenario**: rather than a flat benchmark partition, the simulation constructs a layered scenario to stress-test entity resolution and routing specifically — it does not attempt to validate situation-level structure, which is left to Paper 2.

- **Identity layer**: real identities and observations drawn from a standard public multi-camera person re-identification benchmark (Market-1501 or MSMT17), providing ground-truth identity labels.
- **Situation layer**: a synthetic robbery series is overlaid on the identity layer purely as a narrative/organizational scaffold for grouping observations into cases — used only to structure the simulation, not to claim any situation-level result.
- **Spatial/temporal layer**: camera locations and timestamps are synthesized to drive the spatio-temporal plausibility gate.

Observations are fragmented into isolated "cases" and fed to the system asynchronously, as if arriving from independent investigations; the system's Auto-Merge / Tactical Queue / Auto-Reject output and resulting evidence ledger are compared against the withheld ground truth.

## 6. Results

Reported along three axes, all identity-level:

- **Silo-breaking rate**: the fraction of true cross-case identity links recovered (via auto-merge and tactical-queue routing combined), with precision/recall broken out by routing tier.
- **Routing accuracy**: how well the tripartite routing separates genuinely ambiguous candidates from confident merges and confident rejections.
- **Workload compression**: the reduction from an intractable pairwise comparison problem to a short, linear queue of proposals requiring human validation.
- **Threshold sensitivity**: all of the above repeated across conservative/moderate/aggressive threshold settings, showing threshold choice is a tunable policy trade-off rather than a fixed optimum.

## 7. Discussion

**DSS-centric interpretation**: the paper's central claim is architectural, not about raw matching accuracy — it changes the shape of the analyst's task from an intractable search problem into a short validation queue, and does so by fusing evidence from multiple sources rather than trusting any single similarity score.

**Toward an Integrated Platform**: this section is explicitly bounded to preview, not claim, the rest of the target hierarchy. The Person-Target Profiles this paper produces are designed as the input unit for the next level — composing them via network/topology analysis into Situation/Network-Target Profiles (Paper 2) — and as the object that a separate, deliberately slower verification tier can escalate into court-admissible evidence (Paper 3). Neither claim is evaluated here.

## 8. Limitations & Future Work

- Evaluation uses a public re-identification benchmark plus a synthetic situation/spatial/temporal overlay as a proxy for real fragmented investigations, not an operational deployment.
- The multi-source evidence ledger currently combines biometric auto-match, analyst validation, and field-officer document confirmation; other evidence sources are left for future extension.
- This paper does not evaluate whether composed Person-Target Profiles support accurate situation/network-level inference — that hand-off is explicitly Paper 2's contribution.
- This paper does not address chain-of-custody, court-admissible identification, or examiner protocols for treating a Person-Target Profile as legal evidence — that hand-off is explicitly Paper 3's contribution.

## 9. Conclusion

This paper demonstrates that a target-centric Decision Support architecture — recursing the target-centric model down to the level of a single identity — can continuously and asynchronously construct Person-Target Profiles from fragmented, multi-source investigative data, evaluated entirely by simulation. The Person-Target Profile is positioned as the foundational unit that the rest of the platform's target hierarchy builds on: composed into Situation/Network-Target Profiles in Paper 2, and escalated into evidentiary-grade identifications in Paper 3.
