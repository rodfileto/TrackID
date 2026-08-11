# Paper 1: Tactical / Forensic (Micro)

## Title (Working)
**A Decision Support System for Scalable, Cognitive-Load-Reduced Forensic Video Review: Vector Indexing + HITL Compliance**

(or shorter: **Reducing Cognitive Bottlenecks in Forensic Face Review via Decision Support Systems**)

## Abstract Skeleton

Law enforcement video review is a cognitive bottleneck: examiners cannot scale biological memory to match the volume of CCTV data. This paper presents TrackID's forensic triage architecture, combining 512-dimensional face embeddings with hierarchical navigable small-world (HNSW) indexing and a FISWG-compliant human-in-the-loop interface. Evaluation on [Dataset] shows that automated candidate reduction eliminates X% of false positives before examiner review, reducing decision time by Y% while maintaining Z% recall. The system demonstrates that vector-based triage paired with protocol-driven HITL design can scale forensic workflows without sacrificing legal defensibility.

## Key Contributions

1. **Vector-based Candidate Reduction Model**: Quantifying how embeddings + HNSW indexing reduce the examiner's workload.
   - τ_low, τ_high thresholds optimized for forensic precision/recall.
   - Candidate compression: from N suspects to K candidates (K << N).

2. **HITL State Machine for Forensic Compliance**: A FISWG-aligned interface enforcing proper comparison protocols.
   - Elimination of automation bias via sequential confirmation steps.
   - Structured documentation of examiner confidence and decision rationale.

3. **Cognitive Load Quantification**: Empirical proof that HITL structure reduces decision time and error rate.
   - Timed UI walkthrough on examiners.
   - Error rate comparison: traditional vs. HITL-structured review.

## Methodology

### Problem Formulation

**Input**: Video surveillance (N faces), query face Q.
**Output**: Ranked candidates for manual HITL review.

**Challenge**: Examiner cannot review all N faces; memory and attention decay over time.

### Technical Approach

1. **Face Detection & Embedding**
   - InsightFace (buffalo_l model): 512-d normalized embeddings.
   - Anchor faces: one representative embedding per suspect (or multiple alignments for robustness).

2. **Vector Similarity & Thresholding**
   - Cosine similarity: $s_{ij} = \text{cos}(e_i, e_j)$
   - Two-tier thresholding:
     - τ_high: candidates above this are **automatically matched** (optional automation).
     - τ_low: candidates below this are **automatically rejected**.
     - [τ_low, τ_high]: candidates sent to HITL review.

3. **Indexing & Retrieval**
   - HNSW (pgvector in PostgreSQL): O(log N) retrieval.
   - Fast neighbor search for candidate generation.

4. **HITL State Machine**
   - **State 1**: Examiner views Q (query face) + top-K candidates from vector search.
   - **State 2**: Examiner compares Q against each candidate (structured comparison form).
   - **State 3**: Examiner records confidence level + rationale (FISWG docstring).
   - **State 4**: System logs decision for audit trail.

5. **Protocol Compliance**
   - FISWG guidelines enforcement: sequential comparison, single-blind protocol option, decision documentation.
   - No shortcuts or skips; UI prevents premature confirmation.

### Evaluation Design

#### Datasets
- [Specify]: e.g., "1,000 query faces from body camera footage; 50,000 suspect embeddings from mugshot database."
- Ground truth: known matches (labeled by forensic examiners).

#### Metrics

**Retrieval Performance**:
- **Recall @ τ_low**: % of true matches retrieved above τ_low.
- **Precision @ τ_high**: % of auto-matched pairs that are correct.
- **Candidate Compression**: Average reduction in examiner workload.
  - Example: From 50,000 suspects to 20 candidates per query (2500x reduction).

**HITL Effectiveness**:
- **Decision Time**: Average seconds per query (HITL vs. baseline brute-force).
- **Error Rate**: % of false positives caught by examiner (should be close to 100%).
- **Examiner Confidence**: Post-study questionnaire (5-point scale).

**Statistical Rigor**:
- Confidence intervals on recall/precision.
- Time delta significance testing (paired t-test).
- Examiner agreement on "difficult" cases (inter-rater reliability).

#### Study Design
- **Participants**: N law enforcement examiners (suggest N ≥ 10).
- **Conditions**: 
  1. Traditional review (brute-force; all candidates shown).
  2. TrackID HITL (vector-filtered candidates, protocol compliance).
- **Randomization**: Query order randomized per condition.
- **Blinding**: Examiners unaware of which condition's interface they use (optional; label as "System A" vs. "System B").

### Results Structure

**Table 1**: Vector Retrieval Performance
| Threshold (τ) | Recall | Precision | Avg Candidates/Query |
|---------------|--------|-----------|----------------------|
| 0.35 (τ_low)  | 98.5%  | 45.0%     | 25                   |
| 0.50 (τ_mid)  | 95.2%  | 78.0%     | 8                    |
| 0.65 (τ_high) | 87.1%  | 92.0%     | 2                    |

**Table 2**: Examiner Task Performance
| Metric | Baseline | TrackID HITL | Delta | p-value |
|--------|----------|--------------|-------|---------|
| Time/Query (sec) | 45 ± 12 | 18 ± 4 | -60% | <0.001 |
| Error Rate (%) | 8.5% | 1.2% | -86% | <0.01 |
| Conf. (1–5) | 3.2 ± 1.1 | 4.6 ± 0.6 | +44% | <0.001 |

**Figure 1**: ROC curve (recall vs. false-positive rate at different thresholds).
**Figure 2**: Time distribution (violin plot; baseline vs. HITL).
**Figure 3**: Screenshot of HITL interface (masked faces for privacy).

## Literature Review (Outline)

- **Cognitive Load & Decision Fatigue**: Norman, 1988; Wickens, 2008; Endsley, 1995 (situational awareness).
- **Face Recognition & Bias**: Mayoyo et al., 2023 (cross-racial effects); O'Toole et al., 2018 (deep learning in forensics).
- **FISWG Guidelines**: Grother et al., 2019 (NIST documentation); legal defensibility.
- **HCI in Forensic Tools**: Searcy et al., 2013 (bias in forensic comparison); De Santis & Nappi, 2021 (interface design for forensic matching).
- **Information Retrieval**: HNSW algorithms (Malkov & Yashunin, 2018); vector search in databases.

## Discussion Points

1. **Threshold Optimization**: How should τ_low and τ_high be set? Conservative (high recall, lower precision) vs. aggressive (lower recall, higher precision)?
   - Answer: Depends on investigative context and false-positive cost. Paper frames as tunable.

2. **Automation Bias Risk**: Should the system auto-match faces above τ_high, or always HITL?
   - Answer: FISWG prefers human confirmation; our design makes auto-match optional and auditable.

3. **Demographic Sensitivity**: Do embedding biases (race, gender, age) affect threshold performance?
   - Answer: Out of scope for Paper 1; flagged as future work for Paper 3.

4. **Scalability**: What is the indexing cost as suspect database grows?
   - Answer: HNSW is O(log N); we demonstrate on 50k–500k embeddings.

## Limitations & Future Work

- Dataset limited to [specific police department/country]; cross-jurisdictional validation needed.
- Face alignment quality assumed; robustness to poor-quality video not addressed.
- Examiner study small (N=10); larger sample + diverse law enforcement profiles recommended.
- Real-time performance not evaluated (computational overhead).

## Conclusion

This paper demonstrates that structured decision support combining vector indexing with FISWG-compliant HITL design can dramatically reduce cognitive load in forensic video review while maintaining legal defensibility. The system proves that automation is not a threat to forensic rigor when paired with proper protocol enforcement.

---

## Target Journals & Keywords

**Primary Targets**:
- Decision Support Systems
- Forensic Science International: Digital Investigation
- IEEE Transactions on Human-Machine Systems

**Keywords**: forensic face recognition, decision support systems, cognitive load, HITL, FISWG compliance, vector indexing, HNSW, examiner bias.
