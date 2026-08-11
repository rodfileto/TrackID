# TrackID Research & Publication Strategy

## Overview

TrackID is built to solve two complementary but distinct research problems at different scales:

1. **Paper 1 (Tactical/Forensic)**: The micro-level challenge of forensic video review and cognitive load in law enforcement
2. **Paper 2 (Strategic/Intelligence)**: The macro-level challenge of revealing hidden relationships and organizational structures

A single software platform supports both papers, but they are published as separate, focused contributions to avoid the "Kitchen Sink Paper" trap.

## Why Separate Papers?

Combining both problems in a single 8,000–10,000 word manuscript fails peer review because:

- **Schizophrenic Literature Review**: Defending both requires coverage of cognitive psychology (memory decay), HCI (interface design), computer vision (vector spaces), and network science (graph topology) — reviewers flag as unfocused.
- **Conflicting Evaluation Metrics**: Forensic evaluation proves cognitive load reduction and candidate compression. Intelligence evaluation proves modularity, centrality, and strategic value. A single methodology cannot serve both.
- **Reviewer Mismatch**: An HCI reviewer will ask you to cut the graph math; a network science reviewer will ask you to cut the UI. You cannot win.

**Solution**: Split into two papers, each with surgical depth in its domain.

## Paper 1: Tactical / Forensic (The "Micro" Focus)

**Scope**: Evaluating the forensic HITL triage architecture for law enforcement video review.

**Core Problem**
- Biological memory cannot scale with CCTV data volume.
- Automation bias and cognitive bottlenecks in face-matching workflows.
- FISWG (Facial Identification Scientific Working Group) compliance for legal defensibility.

**The Solution**
- A Decision Support System combining 512-dimensional face embeddings with HNSW vector indexing.
- A FISWG-compliant HITL interface reducing cognitive load and enforcing proper comparison protocols.

**Key Contributions**
- Vector similarity thresholding (τ_low, τ_high) optimized for forensic recall/precision.
- Candidate-reduction model: quantifying how many manual comparisons are eliminated.
- Cognitive load reduction: timed UI walkthrough proof that HITL structure reduces decision time and error rate.

**Evaluation Metrics**
- Recall rate at different similarity thresholds.
- Number of false positives eliminated before HITL.
- Time-per-decision reduction (FISWG-compliant vs. baseline).
- Error rates (mismatches that HITL caught).

**Target Journals**
- Decision Support Systems
- Forensic Science International: Digital Investigation
- IEEE Transactions on Human-Machine Systems
- Computers & Security

**What This Paper Includes**
- Face detection & embedding extraction (InsightFace).
- Vector similarity and HNSW indexing (pgvector).
- HITL interface and state machine (React frontend).
- Forensic protocol validation (FISWG compliance).

**What This Paper Explicitly Excludes**
- "While TrackID includes downstream graph intelligence capabilities, the scope of this paper is strictly limited to evaluating its forensic HITL triage architecture."

---

## Paper 2: Strategic / Intelligence (The "Macro" Focus)

**Scope**: Revealing hidden structures and relationships in criminal networks via complex graph analysis over resolved identities.

**Core Problem**
- Flat databases of faces and videos do not reveal hidden connections.
- Traditional approaches cannot identify choke points, key persons, or community formations.
- Spatio-temporal relationships (where and when people co-occur) reveal organizational hierarchy and operational patterns.

**The Solution**
- A heterogeneous spatio-temporal graph built on top of vector-resolved face identities.
- Complex network analysis (betweenness centrality, Louvain community detection, temporal decay).

**Key Contributions**
- Spatio-temporal graph construction from video surveillance (space + time + identity).
- Community detection in dynamic networks: discovering operational cells.
- Centrality measures revealing high-value targets (bottlenecks, bridges, hubs).
- Temporal decay analysis: how relationships strengthen/weaken over time.

**Evaluation Metrics**
- Modularity scores (Q) before and after community detection.
- Betweenness centrality ranking: do high-centrality nodes match known key persons?
- Hidden link discovery: can the graph reveal relationships not obvious from raw data?
- Spatio-temporal decay: does temporal weighting improve prediction accuracy?

**Target Journals**
- Expert Systems with Applications
- Knowledge-Based Systems
- Network Science (or applied networks section of major journals)
- Computational Social Science

**What This Paper Includes**
- Spatio-temporal graph construction and storage.
- Community detection algorithms (Louvain, spectral clustering).
- Centrality & importance measures.
- Temporal dynamics and decay modeling.

**What This Paper Explicitly Excludes**
- "This paper assumes resolved face identities (validated in [Paper 1]) and focuses on the macro-level graph intelligence they enable."

---

## The Single Monorepo Strategy

**You do not split the open-source code.**

TrackID remains a single, powerful unified repository. The separation is **conceptual and publication-driven**, not architectural.

### Citation Chain

**In Paper 1**, cite TrackID but scope explicitly:
> "While TrackID includes downstream graph intelligence capabilities, the scope of this paper is strictly limited to evaluating its forensic HITL triage architecture."

**In Paper 2**, cite Paper 1:
> "Building upon the foundational vector-triage architecture validated in [Paper 1], this paper explores the macro-level strategic intelligence generated by applying complex network analysis to the resolved identities."

### Code Coverage

Both papers are embedded in the same codebase:

| Component | Paper 1 | Paper 2 |
|-----------|---------|---------|
| Face Detection (InsightFace) | ✓ Core | ✓ Input |
| Vector Indexing (pgvector + HNSW) | ✓ Core | ✓ Used |
| HITL Interface & State Machine | ✓ Core | ✗ Excluded |
| Spatio-Temporal Graph | ✗ Excluded | ✓ Core |
| Community Detection (Louvain) | ✗ Excluded | ✓ Core |
| Centrality Analysis | ✗ Excluded | ✓ Core |

---

## Publication Roadmap

### Phase 1: Paper 1 (Tactical / Forensic)
- **Timeline**: Months 1–6
- **Focus**: HITL triage, vector indexing, forensic validation
- **Deliverables**: Manuscript + Dataset (sanitized video subsets, anonymized faces)
- **Target**: Decision Support Systems or similar (8–12 week review cycle)

### Phase 2: Paper 2 (Strategic / Intelligence)
- **Timeline**: Months 4–12 (overlapping with Paper 1 review)
- **Focus**: Graph construction, community detection, temporal analysis
- **Deliverables**: Manuscript + Code (Louvain implementation, graph queries)
- **Dependency**: Cite Paper 1 once accepted/published
- **Target**: Expert Systems with Applications or similar (8–12 week review cycle)

### Phase 3: Follow-up / Extension Papers (Optional)
- Robustness of vector similarity under demographic variations
- Real-time graph updates (incremental community detection)
- Privacy-preserving graph compression for federated deployment

---

## Key Messaging

### For Reviewers (Paper 1)
"This paper advances forensic HITL design by proving that vector-based candidate reduction + protocol-compliant interface design reduces cognitive load and improves accuracy. The graph capabilities are orthogonal and explicitly out of scope."

### For Reviewers (Paper 2)
"This paper demonstrates that complex network analysis applied to video-derived identity graphs can reveal operational structure in criminal networks. The underlying vector triage (validated separately) enables scalable identity resolution; this work focuses on what those resolved identities reveal."

### For Contributors & Users
"TrackID solves two hard problems with a single, integrated platform. Read the papers to understand the research depth; the code shows how forensic triage feeds intelligent network analysis."

---

## References & Related Work

See `docs/PAPER1_FORENSIC.md` and `docs/PAPER2_INTELLIGENCE.md` for detailed literature reviews, methodology, and metrics for each paper.
