# Paper 1: Tactical Case Linking (Asynchronous Entity Resolution)

## Title (Working)
**Breaking Investigative Silos: A Decision Support Architecture for Asynchronous Tactical Case Linking**

(or shorter: **Tactical Case Linking: Continuous Entity Resolution Across Fragmented Spatio-Temporal Events**)

## Abstract Skeleton

In public security operations, distinct incidents — a robbery in District A on Monday, a car theft in District B on Wednesday — are investigated in isolated case silos. Relational case-management systems have no mechanism to recognize that an unidentified suspect present in one case is the same person present in another. This paper presents TrackID's **Linkage Engine**, a Decision Support System (DSS) that treats faces as 512-dimensional vectors and continuously, asynchronously evaluates incoming surveillance data to propose linkages between otherwise disconnected cases. The system applies spatio-temporal plausibility constraints on top of vector similarity — rejecting matches that would require physically impossible travel — and routes candidate pairs through a **tripartite state machine**: Auto-Merge (high-confidence identity fusion into a Global Trajectory), Tactical Queue (uncertain candidates requiring a one-click human validation), and Auto-Reject (discarded before reaching a human). Because this paper targets an information-systems contribution rather than a forensic-evidence one, evaluation requires no human subjects: we simulate fragmented investigations by artificially partitioning a standard multi-camera person re-identification dataset (e.g., Market-1501 or MSMT17) into isolated "incidents" by camera and time block, then measure how well the Linkage Engine reconstructs the underlying identities. Results show (1) **Silo-Breaking Rate**: X% of true cross-case identity links are recovered via Auto-Merge and Tactical Queue routing combined, at Y% precision; (2) **Workload Compression**: the engine collapses an O(N²) manual cross-referencing problem across 100 simulated cases into a linear sequence of Z tactical proposals, a >99% reduction in required human comparisons. The contribution is a DSS architecture for continuous, asynchronous entity resolution across fragmented data — not a forensic identification method.

## Key Contributions

This paper contributes to **Information Systems / Decision Support Systems**, not computer vision. The novelty is the linkage architecture and routing logic that sits on top of a commodity embedding model (InsightFace/ArcFace), not the embedding model itself.

1. **The Tripartite Routing State Machine** (Core DSS Contribution):
   - **Auto-Merge** (high confidence): candidate pairs above the upper threshold and spatio-temporally plausible are fused automatically into a single Global Trajectory — no human in the loop.
   - **Tactical Queue** (uncertainty band): candidate pairs in the ambiguous zone are surfaced as a **Proposed Link** between two cases, requiring only a one-second human click to confirm or dismiss.
   - **Auto-Reject**: candidate pairs below the lower threshold, or that fail spatio-temporal plausibility regardless of vector similarity, are discarded before ever reaching a human — preventing database bloat and alert fatigue.
   - **Novelty**: most entity-resolution systems expose a flat "is this a match?" decision. The tripartite design explicitly separates *machine-confident* fusion from *machine-uncertain* triage from *machine-confident* rejection, minimizing human involvement to only the genuinely ambiguous cases.

2. **Spatio-Temporal Plausibility as a Filter on Vector Similarity** (Academic/Methodological Contribution):
   - Problem: cosine similarity alone is blind to physical reality — two embeddings can be an 85% match yet correspond to sightings 500 km apart, 5 minutes apart, which is physically impossible.
   - Solution: a spatio-temporal decay/bounding term that discounts or hard-rejects candidate links implying implausible travel speed between camera locations.
   - **Metric**: reduction in false-positive Auto-Merges attributable to spatio-temporal filtering vs. vector similarity alone.
   - **Novelty**: combines a standard IR/vector-retrieval technique with a lightweight physical plausibility model, without requiring any additional sensing modality.

3. **Asynchronous Silo-Breaking Without Administrative Overhead** (Organizational Problem Solved):
   - Problem: analysts in District 1, District 2, and District 3 investigate independently; a shared suspect across their cases stays invisible unless someone happens to notice.
   - Solution: the Linkage Engine continuously indexes every embedding as it arrives and proposes cross-case links automatically — no meetings, no manual cross-referencing, no shared awareness required between analysts.
   - **Metric**: Silo-Breaking Rate — the fraction of true cross-case identity links recovered by the system without any analyst having looked for them.
   - **Novelty**: reframes entity resolution as a continuous background process over a fragmented case database, rather than a query-time lookup a human must initiate.

4. **Workload Compression via Combinatorial Reduction** (Quantified DSS Value):
   - Problem: manually cross-referencing every suspect across every open case is an N×(N−1)/2 combinatorial explosion, intractable at scale.
   - Solution: thresholded routing collapses this into a small, linear set of Tactical Queue proposals that require only rapid validation, not search.
   - **Metric**: Workload Compression Ratio — pairwise comparisons implied by brute-force manual review vs. Tactical Queue proposals actually generated.
   - **Novelty**: quantifies the DSS's triage value independent of its raw matching accuracy — a system can be valuable even before considering how *accurate* its proposals are, simply because of what it removes from human attention.

## Methodology

### Problem Formulation

**Input**: A stream of face embeddings extracted from surveillance video, each tagged with a case ID, camera ID, location, and timestamp. Cases are treated as isolated silos — no shared identifiers exist across them a priori.

**Output**: (a) A set of Auto-Merged Global Trajectories linking embeddings across cases; (b) a ranked Tactical Queue of Proposed Links between cases, awaiting one-click human validation; (c) an implicit Auto-Reject set, never surfaced.

**Challenge**: No analyst can be assumed to know, or to check, whether a suspect in their case also appears in someone else's. The system must perform this resolution continuously and asynchronously, without waiting for a human query.

### Technical Approach

#### Face Embedding as a Commodity Component

Facial embeddings are generated using **InsightFace (ArcFace)** [Cite: Deng et al.], a well-established convolutional network whose accuracy has been exhaustively benchmarked in the computer vision literature. **Validating the embedding model itself is out of scope.** This paper treats the 512-dimensional embedding ($\vec{v}$) as a standardized input and focuses entirely on the downstream Decision Support System: how it indexes vectors, filters them against physical plausibility, and routes uncertainty to minimize human workload.

**Note on Evaluation Scope**: This paper does not measure mAP, Rank-1 accuracy, or pose/lighting invariance of the underlying embedding model. Evaluation is confined to the information-systems layer — routing correctness, silo-breaking recall/precision, and workload compression.

#### The Linkage Engine Architecture

1. **Vector Ingestion & Indexing**
   - InsightFace produces a 512-d normalized embedding per detected face.
   - Embeddings are indexed with HNSW (pgvector in PostgreSQL) for O(log N) approximate nearest-neighbor retrieval as new observations arrive.
   - Each embedding is tagged with its originating case, camera, and timestamp — the silo boundary is metadata, not a separate database.

2. **Spatio-Temporal Heuristics**
   - Raw candidate similarity: $s_{ij} = \cos(e_i, e_j)$.
   - Physical plausibility check: given camera locations and timestamps for observations $i$ and $j$, compute the implied minimum travel speed $v_{ij} = d_{ij} / |t_i - t_j|$, where $d_{ij}$ is the great-circle (or road-network) distance between camera sites.
   - **Plausibility gate**: if $v_{ij} > v_{\max}$ (a configurable maximum plausible travel speed), the pair is rejected regardless of $s_{ij}$.
   - **Soft decay** (alternative to a hard cutoff): $w_{ij} = \exp(-\lambda \cdot \max(0,\, v_{ij} - v_{\max}))$, applied as a multiplicative discount on similarity: $s'_{ij} = s_{ij} \cdot w_{ij}$.
   - This is a simple bounding/decay model, not a full trajectory model — it deliberately trades sophistication for auditability and speed.

3. **The Tripartite Routing State Machine**
   - $s'_{ij} \geq \tau_{\text{high}}$: **Auto-Merge** — the two observations are fused into a single Global Trajectory; the underlying cases are automatically linked and logged for audit.
   - $\tau_{\text{low}} \leq s'_{ij} < \tau_{\text{high}}$: **Tactical Queue** — a Proposed Link is surfaced between the two cases; any analyst may resolve it with a one-second accept/reject click, and the resolution merges or discards the candidate.
   - $s'_{ij} < \tau_{\text{low}}$: **Auto-Reject** — discarded silently; never presented to a human, preventing queue bloat from false candidates.

4. **Global Trajectory / Cluster Graph Management**
   - The system maintains an in-database cluster graph: embedding → identity cluster → linked case IDs.
   - Auto-Merges and Tactical Queue confirmations both update this graph, so the Global Trajectory grows monotonically as more evidence arrives — this is the mechanism of asynchronous silo-breaking.

### Evaluation Design (No Human Testing Required)

#### Simulating Fragmented Investigations

Because this paper evaluates a DSS routing architecture rather than a piece of forensic evidence, it does not require an examiner study or IRB-governed human subjects. Instead, we construct a controlled simulation from a standard public multi-camera person re-identification benchmark (e.g., **Market-1501** or **MSMT17**), which already provides multiple identities observed across multiple cameras and time periods with ground-truth identity labels.

**Simulation procedure**:
1. Select $K$ unique identities and their observations across the dataset's cameras/timestamps.
2. Artificially partition observations into $N$ "isolated cases" by grouping on camera ID and time block, discarding the ground-truth identity linkage between groups (simulating independent investigations that don't share information).
3. Feed all case observations into the Linkage Engine as if arriving asynchronously over time.
4. Compare the engine's Auto-Merge/Tactical Queue/Auto-Reject output against the withheld ground truth to measure whether the true cross-case identity links were recovered.

#### Metric 1: Silo-Breaking Rate (Entity Resolution Accuracy)

- **Setup**: 100 simulated isolated incidents constructed from 20 true unique identities.
- **Measurement**: 
  - **Auto-Merge Precision/Recall**: of the true cross-case links, what fraction were correctly Auto-Merged, and of all Auto-Merges, what fraction were correct?
  - **Tactical Queue Recall**: of the true cross-case links *not* Auto-Merged, what fraction were correctly routed to the Tactical Queue (i.e., not silently Auto-Rejected)?
  - **Auto-Reject Leakage**: what fraction of true links were incorrectly Auto-Rejected and permanently lost?
- **Result target**: high Auto-Merge precision (>95%) with near-zero Auto-Reject leakage of true links (<2%), demonstrating that uncertainty is pushed to the Tactical Queue rather than silently discarded.

#### Metric 2: Workload Compression (The Triage Metric)

- **Setup**: the same 100-case simulation. A traditional analyst would need to manually compare suspects across all 100 cases — $\binom{100}{2} = 4{,}950$ pairwise comparisons in the worst case (more, if multiple suspects per case).
- **Measurement**: count the number of Tactical Queue proposals actually generated (e.g., 45) and compute the **Workload Compression Ratio**: 
$$\text{WCR} = 1 - \frac{|\text{Tactical Queue proposals}|}{\binom{N_{\text{total observations}}}{2}}$$
- **Result target**: WCR > 99%, demonstrating that the engine converts an intractable combinatorial search into a short, linear list of high-probability proposals requiring only rapid validation.

#### Threshold Sensitivity

- Repeat both metrics across conservative/moderate/aggressive $(\tau_{\text{low}}, \tau_{\text{high}})$ settings and $v_{\max}$ values, to show threshold choice is a tunable policy trade-off (recall vs. queue size), not a fixed optimum.

### Results Structure

**Table 1**: Routing Distribution Across Threshold Settings
| Threshold Set | Auto-Reject % | Auto-Merge % | Tactical Queue % | True-Link Recall | Notes |
|---|---|---|---|---|---|
| Conservative | 55% | 10% | 35% | 99.5% | Larger queue, safer |
| Moderate | 78% | 30% | 12% | 97.8% | Recommended balance |
| Aggressive | 90% | 55% | 5% | 92.0% | Small queue, risk of silent loss |

**Table 2**: Silo-Breaking Performance (Entity Resolution Accuracy)
| Metric | Result | Target |
|---|---|---|
| Auto-Merge Precision | — | >95% |
| Auto-Merge Recall | — | >85% |
| Tactical Queue Recall (of remaining true links) | — | >95% |
| Auto-Reject Leakage (true links lost) | — | <2% |

**Table 3**: Workload Compression
| Scenario | Cases | Total Observations | Brute-Force Comparisons | Tactical Queue Proposals | WCR |
|---|---|---|---|---|---|
| Simulated Investigation | 100 | ~500 | ~124,750 | 45 | >99.9% |

**Figure 1**: The Tripartite Routing State Machine (diagram: Auto-Merge / Tactical Queue / Auto-Reject decision flow).

**Figure 2**: Spatio-temporal plausibility gate — scatter plot of implied travel speed vs. vector similarity, showing rejected vs. accepted candidate pairs.

**Figure 3**: Workload Compression — brute-force comparisons vs. Tactical Queue proposals as case count scales (log-scale bar/line chart).

**Figure 4**: Screenshot of the Tactical Queue interface (one-click Proposed Link validation).

## Literature Review (Outline)

**Primary Focus: Decision Support Systems, Entity Resolution, Information Systems**

- **Entity Resolution / Record Linkage**: Fellegi & Sunter, 1969 (foundational probabilistic record linkage); Christen, 2012 (data matching survey); Getoor & Machanavajjhala, 2012 (entity resolution in big data).
- **Decision Support System Design**: Sprague & Watson, 1993 (DSS frameworks); Arnott & Pervan, 2014 (DSS in organizations); Shim et al., 2002 (past, present, future of DSS).
- **Human-in-the-Loop Triage / Uncertainty Routing**: Amershi et al., 2019 (guidelines for human-AI interaction); Green & Chen, 2019 (human-AI decision-making in the loop).
- **Organizational Silos & Information Sharing in Public Safety**: Bharosa et al., 2010 (information sharing in emergency management); Zheng et al., 2014 (inter-organizational information sharing barriers).

**Secondary Focus: Technical Implementation (Commodity, Not Novel)**

- **Facial Embeddings**: Deng et al., 2019 (ArcFace — treated as black box).
- **Person Re-Identification Benchmarks**: Zheng et al., 2015 (Market-1501); Wei et al., 2018 (MSMT17).
- **HNSW / Approximate Nearest Neighbor Search**: Malkov & Yashunin, 2018; pgvector documentation.
- **Spatio-Temporal Constraint Modeling**: Yuan et al., 2011 (trajectory pattern mining); Zheng, 2015 (trajectory data mining survey).

## Discussion Points (DSS-Centric)

1. **Threshold Calibration as Policy, Not Optimization**: $\tau_{\text{low}}$, $\tau_{\text{high}}$, and $v_{\max}$ jointly define a trade-off between queue size and recall of true links. There is no single "correct" setting — an agency prioritizing thoroughness over analyst time will tune differently than one prioritizing speed.

2. **Why Spatio-Temporal Filtering Matters More Than Threshold Tuning**: raising $\tau_{\text{high}}$ alone to reduce false Auto-Merges trades away true-positive recall; the spatio-temporal gate removes a specific, physically-groundable class of false positives without that trade-off, making it a more efficient lever than similarity-threshold tuning alone.

3. **From Combinatorial Search to Linear Validation**: the paper's central practical claim is not that the system is a highly accurate matcher — it is that it changes the *shape* of the analyst's task, from an intractable search problem to a short validation list. This value exists even under conservative thresholds that produce larger queues.

4. **Asynchronous Collaboration Without Coordination**: because Auto-Merge and Tactical Queue proposals are generated purely from data as it arrives, two analysts who never communicate can have their cases linked automatically — the DSS substitutes for organizational coordination overhead that would otherwise require deliberate cross-referencing.

5. **Generalization Beyond Faces**: the tripartite routing + spatio-temporal plausibility architecture is agnostic to the embedding source; it would apply equally to license plates, gait signatures, or other biometric/behavioral vectors, though this paper scopes evaluation to face embeddings only.

## Limitations & Future Work

- Evaluation uses a public person re-identification benchmark as a proxy for real fragmented investigations; camera geolocations and time blocks are simulated, not drawn from an operational deployment.
- The spatio-temporal plausibility model uses a simple bounding/decay function on straight-line or road-network distance; it does not model real-world travel constraints (traffic, transit schedules, terrain).
- The Tactical Queue's "one-second validation click" is not evaluated with real analysts in this paper — usability and actual validation latency are deferred to a future human-subjects study, since this paper's contribution is architectural, not behavioral.
- Threshold values are dataset-specific; cross-agency and cross-dataset calibration is left to future work.
- This paper explicitly does not address court-admissible identification, chain-of-custody, or forensic examiner protocols — those are the subject of a planned follow-up (Paper 3).

## Conclusion

This paper demonstrates that a lightweight Decision Support architecture — combining vector similarity, spatio-temporal plausibility, and tripartite uncertainty routing — can perform continuous, asynchronous entity resolution across fragmented investigative silos without requiring any change to how individual cases are managed, and without requiring human-subjects evaluation. By simulating fragmented investigations on a standard public re-identification benchmark, we show the Linkage Engine recovers a high fraction of true cross-case identity links while compressing the analyst's workload from an intractable combinatorial search into a short, linear validation queue.

**Not About AI, But About Routing**: the novelty is not the embedding model (a commodity), but the architecture that decides, automatically, which candidate pairs need a human at all.

**Quantified Impact** (illustrative targets, to be replaced with measured results):
- **Silo-Breaking Rate**: >95% Auto-Merge precision with <2% true-link leakage into Auto-Reject.
- **Workload Compression**: >99% reduction in required pairwise comparisons vs. brute-force manual cross-referencing.

Future work extends this architecture in two directions: applying network topology over the resulting linked cases to surface organizational structure (Paper 2), and layering a slow, evidentiary-grade verification tier on top of Tactical Queue confirmations for court-admissible identification (Paper 3).

---

## Target Journals & Keywords

**Primary Targets** (Ranked by fit):
1. **Decision Support Systems** — Highest fit. Explicitly seeks DSS architecture, uncertainty routing, and organizational workflow contributions.
2. **Expert Systems with Applications** — Strong fit. Applied entity resolution / triage systems with quantified workload metrics.
3. **Information Systems Frontiers** — Possible fit. Organizational information-sharing and silo-breaking framing.
4. **IEEE Transactions on Human-Machine Systems** — Secondary fit if reframed around the human-in-the-loop triage design.

**Keywords**: 
- **DSS/Systems**: decision support systems, entity resolution, record linkage, information systems, uncertainty routing, human-in-the-loop.
- **Domain**: tactical intelligence, case linking, investigative silos, public security, criminal investigation support.
- **Technical (Commodity)**: vector indexing, embeddings, HNSW, spatio-temporal constraints, candidate reduction, threshold policy.

**Avoid Framing As**: face recognition validation, facial matching accuracy, forensic identification, legal/evidentiary compliance, deep learning optimization.
