# Paper 2: Strategic / Intelligence (Macro)

## Title (Working)
**Revealing Hidden Organizational Structures in Criminal Networks via Spatio-Temporal Graph Analysis on Video-Derived Face Identities**

(or shorter: **Complex Network Analysis of Video-Derived Social Networks: Uncovering Organizational Hierarchy and Operational Patterns**)

## Abstract Skeleton

Criminal organizations exhibit hidden structural patterns not visible in flat databases: choke points, operational cells, and temporal dynamics. This paper presents a spatio-temporal heterogeneous graph constructed from surveillance video faces and applies complex network analysis to reveal organizational structure. Building upon a validated vector-based identity resolution system (prior work), we demonstrate that (1) community detection identifies operational cells, (2) betweenness centrality uncovers key persons, and (3) temporal decay modeling reveals relationship strength evolution. Evaluation on [multi-event dataset] shows that graph-derived centrality rankings align with known organizational hierarchy (Spearman ρ = 0.87) and community detection recovers known cell structure (NMI = 0.79). The approach enables strategic intelligence generation from raw surveillance data.

## Key Contributions

1. **Spatio-Temporal Graph Construction**: A heterogeneous graph model encoding identity (face embeddings), space (location), and time (co-occurrence timestamps).
   - Nodes: unique suspects (resolved via vector matching).
   - Edges: spatio-temporal co-occurrence (same place, overlapping time).
   - Edge attributes: frequency, temporal proximity, location category.

2. **Community Detection in Dynamic Networks**: Discovering operational cells via modularity optimization.
   - Louvain algorithm applied to time-windowed subgraphs.
   - Temporal stability: communities persisting across time windows indicate structured organization.
   - Modularity (Q) and Normalized Mutual Information (NMI) against ground truth.

3. **Centrality Ranking for Strategic Value**: Identifying key persons via multiple centrality measures.
   - Betweenness centrality: bottlenecks and bridges.
   - Eigenvector centrality: connection to influential others.
   - Temporal evolution: how centrality changes as organization adapts.

4. **Spatio-Temporal Decay Modeling**: Quantifying how relationships weaken over time and distance.
   - Exponential decay: λ_t (temporal), λ_d (spatial).
   - Predicting future co-occurrence and link persistence.

## Methodology

### Problem Formulation

**Input**: 
- Video surveillance (faces, locations, timestamps).
- Identity resolution (from Paper 1): known matches per suspect.

**Output**: 
- Graph of suspects + relationships.
- Ranked suspects by centrality.
- Detected operational communities.

**Challenge**: Revealing organizational structure from raw observations requires integrating space, time, and identity simultaneously.

### Technical Approach

#### 1. Graph Construction

**Nodes**:
- Each unique suspect = 1 node.
- Identified via face vector matching (Paper 1 methodology).

**Edges**:
- Suspects in same location + overlapping time → edge.
- Edge weight: frequency of co-occurrence or inverse temporal distance.

**Formalism**:
```
G = (V, E, A)
where:
  V = {v_1, v_2, ..., v_n}  (suspects)
  E ⊆ V × V  (spatio-temporal co-occurrence)
  A[i,j] = w_ij  (edge weight; frequency or temporal proximity)
```

**Edge Weight Function**:
$$w_{ij} = \sum_{t=1}^{T} \exp(-\lambda_t \cdot |t_i - t_j|) \cdot \exp(-\lambda_d \cdot d_{ij})$$

where:
- λ_t: temporal decay rate.
- λ_d: spatial decay rate.
- |t_i - t_j|: time difference between co-occurrences.
- d_ij: spatial distance (e.g., Euclidean or location category difference).

#### 2. Community Detection

**Algorithm**: Louvain method (modularity optimization).
- Iterative greedy algorithm.
- Multi-level resolution: detect communities at different scales.

**Temporal Dynamics**:
- Apply Louvain to time-windowed subgraphs (e.g., weekly windows).
- Track community evolution: stable communities vs. transient clusters.
- Stability metric: Adjusted Rand Index (ARI) between consecutive time windows.

**Quality Metrics**:
- **Modularity (Q)**: Q ∈ [-1, 1]; Q > 0.3 indicates strong community structure.
- **Normalized Mutual Information (NMI)**: Agreement with ground truth (if available).
  - NMI ∈ [0, 1]; NMI = 1 is perfect agreement.

#### 3. Centrality Analysis

**Betweenness Centrality**:
$$C_B(v) = \sum_{s \neq v \neq t} \frac{\sigma_{st}(v)}{\sigma_{st}}$$

where σ_st(v) = # shortest paths through v; σ_st = total # shortest paths.
- High betweenness = bottleneck (removal would disconnect network).

**Eigenvector Centrality**:
$$C_E(v) \propto \sum_{t \in N(v)} C_E(t)$$

- High eigenvector = connected to other high-centrality nodes (hubs).

**Temporal Centrality**:
- Compute centrality on time-windowed subgraphs.
- Rank centrality evolution: rising (increasing influence) vs. stable vs. declining.

#### 4. Spatio-Temporal Decay Modeling

**Hypothesis**: Relationships decay over time and distance.

**Model**:
$$P(\text{future co-occurrence} | i, j) = w_{ij} = A \exp(-\lambda_t t - \lambda_d d)$$

**Parameter Estimation**:
- Fit λ_t and λ_d via maximum likelihood on historical data.
- Predict future links: high-score pairs likely to co-occur again.

**Application**:
- Link prediction: who will appear together next?
- Operational forecasting: when/where will organization reassemble?

### Evaluation Design

#### Datasets

**Real-world Multi-Event Scenario**:
- [Dataset descriptor]: e.g., "3 related criminal events over 6 months; 150 unique suspects; 10,000+ video frames with face detections."
- Ground truth: investigator-confirmed cell structure and key persons.

**Synthetic Stress-Test** (optional):
- Erdős–Rényi random graphs + planted communities.
- Validates that algorithms recover known structure.

#### Metrics

**Community Detection**:
| Metric | Definition | Target |
|--------|-----------|--------|
| Modularity (Q) | Strength of community structure | Q > 0.3 |
| NMI | Agreement with ground truth | NMI > 0.7 |
| Stability (ARI) | Consistency across time windows | ARI > 0.6 |

**Centrality Validation**:
| Metric | Definition | Target |
|--------|-----------|--------|
| Spearman ρ | Correlation with known hierarchy | ρ > 0.8 |
| Precision @ K | Fraction of top-K betweenness nodes known in organization | P@10 > 80% |
| Kendall τ | Rank-order agreement | τ > 0.7 |

**Link Prediction**:
| Metric | Definition | Target |
|--------|-----------|--------|
| AUC-ROC | Discriminating true links from false | AUC > 0.85 |
| Precision @ K | Fraction of predicted links that occur | P@5 > 60% |

**Temporal Robustness**:
- Re-compute graph weekly; assess consistency of top-ranked suspects.
- Measure: Spearman ρ between consecutive rankings.

#### Study Design
- **Longitudinal**: multi-month surveillance dataset.
- **Blinded Validation**: compare algorithmic rankings against investigator-identified hierarchy without revealing algorithm scores initially.
- **Qualitative Interviews**: investigator feedback on discovered patterns (were they actionable?).

### Results Structure

**Table 1**: Community Detection Performance
| Dataset | Events | Suspects | Modularity (Q) | NMI | Stability (ARI) |
|---------|--------|----------|----------------|-----|-----------------|
| Multi-Event A | 3 | 150 | 0.52 | 0.79 | 0.68 |
| Multi-Event B | 5 | 320 | 0.48 | 0.74 | 0.65 |
| Synthetic | 1 | 100 | 0.71 | 0.95 | 0.92 |

**Table 2**: Centrality Ranking Validation
| Measure | Spearman ρ | Kendall τ | P@10 |
|---------|-----------|-----------|------|
| Betweenness Centrality | 0.87 | 0.75 | 0.85 |
| Eigenvector Centrality | 0.79 | 0.68 | 0.72 |
| Temporal Betweenness | 0.83 | 0.71 | 0.80 |

**Table 3**: Link Prediction Performance
| Method | AUC-ROC | Precision@5 | Recall@10 |
|--------|---------|-------------|-----------|
| Louvain + Decay (λ_t, λ_d) | 0.88 | 0.64 | 0.72 |
| Baseline (Random Forest on features) | 0.75 | 0.48 | 0.55 |

**Figure 1**: Spatio-temporal graph visualization (time-windowed snapshots).
**Figure 2**: Community detection over time (Sankey diagram showing cell evolution).
**Figure 3**: Centrality ranking evolution (line plot; top suspects over time).
**Figure 4**: Decay model fit (λ_t, λ_d estimated from data vs. prediction accuracy).

## Literature Review (Outline)

- **Complex Networks & Criminal Networks**: Xu & Ling, 2013 (criminal network analysis); Morselli, 2009 (structural patterns in organized crime).
- **Community Detection**: Blondel et al., 2008 (Louvain algorithm); Lancichinetti et al., 2015 (evaluation).
- **Centrality Measures**: Freeman, 1977 (betweenness); Bonacich, 1987 (eigenvector).
- **Temporal Networks**: Holme & Ghoshal, 2016 (temporal network review); Interdonato et al., 2020 (temporal community detection).
- **Link Prediction**: Lü & Zhou, 2011; Menon & Elkan, 2011.
- **Video Surveillance + Intelligence**: Jain et al., 2015 (video analysis for intelligence); Vizzini et al., 2022 (face recognition in investigative contexts).

## Discussion Points

1. **Causal vs. Associational**: Are co-occurrences truly indicative of collaboration, or coincidence?
   - Answer: We treat as associational; ground truth validation required for causal claims.

2. **Temporal Decay Parameters**: How sensitive are results to λ_t and λ_d?
   - Answer: Sensitivity analysis; robustness tested across parameter ranges.

3. **Privacy & Legal Admissibility**: Can graph discoveries be presented in court without compromising surveillance methods?
   - Answer: Out of scope but discussed as policy implication.

4. **False Identities**: Paper 1 assumes resolved identities; what if vector matching fails?
   - Answer: Propagated error discussed; recommends confidence weighting.

## Limitations & Future Work

- Assumes high-quality video with visible faces (no occlusion, extreme angles).
- Does not account for false negatives in face detection (missed people).
- Community detection validated on single organization type; generalization to other criminal structures unclear.
- Temporal decay parameters estimated from single dataset; cross-domain validation needed.
- Real-time graph updates not addressed (computational cost of incremental Louvain).

## Conclusion

This paper demonstrates that complex network analysis applied to video-derived identity graphs can reveal organizational structures invisible in flat databases. By integrating space, time, and identity, investigators can prioritize targets, identify operational cells, and forecast organizational behavior. The work opens a new frontier in intelligence-led law enforcement.

---

## Target Journals & Keywords

**Primary Targets**:
- Expert Systems with Applications
- Knowledge-Based Systems
- Network Science (or applied networks)
- Computational Social Science

**Keywords**: criminal networks, complex networks, community detection, centrality, spatio-temporal graphs, surveillance analysis, link prediction, organizational structure, Louvain, betweenness centrality.
