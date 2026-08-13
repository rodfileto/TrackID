# TrackID Research & Publication Strategy

## Overview

TrackID is built to solve three complementary but distinct research problems, staged as three separate papers:

1. **Paper 1 (Tactical Case Linking)**: The problem of asynchronous entity resolution across fragmented investigative silos — fast, DSS-focused, no human-subjects testing required.
2. **Paper 2 (Strategic Intelligence)**: The macro-level challenge of revealing hidden relationships and organizational structures once cases are linked.
3. **Paper 3 (Forensic Evidentiary Validation)**: The slow, court-admissible identification workflow — ACE-VR, FISWG compliance, automation-bias mitigation — requiring a human-subjects examiner study.

A single software platform supports all three papers, but they are published as separate, focused contributions to avoid the "Kitchen Sink Paper" trap.

## Why Separate Papers?

Combining these problems in a single manuscript fails peer review because:

- **Schizophrenic Literature Review**: defending entity resolution, network science, and forensic/legal compliance simultaneously requires coverage of information retrieval, complex networks, cognitive psychology, and forensic protocol standards — reviewers flag as unfocused.
- **Conflicting Evaluation Strategies**: Paper 1's claims are provable via simulation on public benchmarks (no ethics review needed). Paper 3's claims are behavioral and require a human-subjects examiner study (ethics review required). Paper 2's claims are graph-topological. A single methodology section cannot serve all three.
- **Reviewer Mismatch**: a DSS/IS reviewer will ask you to cut the FISWG legal detail; a forensic-science reviewer will ask you to cut the graph math; a network-science reviewer will ask you to cut the UI. You cannot win.

**Solution**: Split into three papers, each with surgical depth in its domain, published in sequence.

## Paper 1: Tactical Case Linking (Asynchronous Entity Resolution)

**Scope**: A Decision Support architecture that continuously and asynchronously resolves identities across fragmented, siloed investigative cases — without requiring any human-subjects evaluation.

**Core Problem**
- Distinct incidents investigated in isolated case silos (different districts, different analysts, different days) never get cross-referenced unless a human happens to notice the connection.
- Manually cross-referencing every suspect across every open case is an intractable N×(N−1)/2 combinatorial problem.
- Vector similarity alone is blind to physical plausibility (an 85% match implying an impossible travel speed between two cameras is still nonsense).

**The Solution**
- Treat faces as 512-dimensional vectors indexed continuously as they arrive (InsightFace + pgvector/HNSW — commodity components, not the novelty).
- Apply a spatio-temporal plausibility gate on top of vector similarity to reject physically impossible links.
- Route every candidate pair through a **tripartite state machine**: Auto-Merge (high confidence, no human), Tactical Queue (uncertain, one-click human validation), Auto-Reject (low confidence, discarded silently).

**Key Contributions** (DSS-Focused, Not CV-Focused)
- The tripartite routing architecture itself — minimizing human involvement to only genuinely ambiguous candidates.
- Spatio-temporal plausibility filtering as a physically-grounded complement to similarity thresholding.
- **Silo-Breaking Rate**: how much of the true cross-case identity structure is recovered without any analyst looking for it.
- **Workload Compression**: collapsing an O(N²) manual cross-referencing problem into a short, linear Tactical Queue (target: >99% reduction).

**Evaluation Strategy — No Human Testing Required**
Simulated fragmentation of a standard public multi-camera person re-identification benchmark (Market-1501, MSMT17): artificially partition known identities into isolated "cases" by camera/time block, discard the ground-truth linkage, then measure how well the Linkage Engine reconstructs it. This sidesteps any need for an ethics-reviewed human-subjects study, since the paper is proving a routing/entity-resolution architecture, not a forensic identification method.

**Target Journals**
- Decision Support Systems
- Expert Systems with Applications
- Information Systems Frontiers
- IEEE Transactions on Human-Machine Systems (secondary)

**What This Paper Includes**
- Face detection & embedding extraction (InsightFace, treated as a black box).
- Vector similarity and HNSW indexing (pgvector).
- Spatio-temporal plausibility gating.
- The Tactical Queue interface and tripartite routing state machine.

**What This Paper Explicitly Excludes**
- "While TrackID includes a slow, evidentiary-grade verification tier (ACE-VR/FISWG) and downstream graph intelligence capabilities, the scope of this paper is strictly limited to the tactical case-linking architecture and its no-human-testing simulation evaluation."

---

## Paper 2: Strategic Intelligence & Complex Networks

**Scope**: Revealing hidden structures and relationships in criminal networks via complex graph analysis over the cases and identities linked by Paper 1's engine.

**Core Problem**
- Flat databases of linked faces and cases do not reveal hidden connections — choke points, operational cells, key persons.
- Spatio-temporal co-occurrence patterns reveal organizational hierarchy and operational dynamics that no individual case file exposes.

**The Solution**
- A heterogeneous spatio-temporal graph built on top of the vector-resolved identities produced by Paper 1.
- Complex network analysis: Louvain community detection, betweenness/eigenvector centrality, temporal decay modeling.

**Key Contributions**
- Spatio-temporal graph construction from linked-case data (space + time + identity).
- Community detection revealing operational cells; centrality measures revealing key persons; temporal decay modeling of relationship strength.

**Evaluation Metrics**
- Modularity (Q), Normalized Mutual Information (NMI) against ground-truth cell structure.
- Centrality ranking correlation (Spearman ρ) against known organizational hierarchy.
- Link prediction (AUC-ROC) via the fitted spatio-temporal decay model.

**Target Journals**
- Expert Systems with Applications
- Knowledge-Based Systems
- Network Science / Computational Social Science venues

**What This Paper Includes**
- Spatio-temporal graph construction and storage.
- Community detection algorithms (Louvain, spectral clustering).
- Centrality & importance measures, temporal dynamics.

**What This Paper Explicitly Excludes**
- "This paper assumes cases have already been linked via the tactical entity-resolution architecture validated in [Paper 1] and focuses on the macro-level graph intelligence those linkages enable."

---

## Paper 3 (Future): Forensic Evidentiary Validation

**Scope**: The slow, court-admissible identification workflow that sits *above* a Tactical Queue confirmation when a lead must become legal evidence — ACE-VR methodology, mandatory FISWG morphological checklist, blind dual-expert verification, and automation-bias mitigation grounded in Dual-Process Theory (Kahneman).

**Core Problem**
- A Tactical Queue confirmation (Paper 1) is fast and low-friction by design — appropriate for operational leads, not for evidence.
- Automated suggestions risk automation bias: examiners rubber-stamping AI-suggested matches (System 1) instead of independently verifying them (System 2).
- Legal defensibility requires FISWG-compliant structured comparison, full audit trails, and independent dual-expert sign-off.

**The Solution**
- A dedicated, deliberately slow verification tier: Analysis → Comparison (FISWG checklist, mandatory) → Evaluation → Verification (blind second expert), producing an immutable, digitally-signed Forensic Identification Report.

**Key Contributions**
- Full ACE-VR workflow enforcement as a DSS/UI contribution, not an algorithmic one.
- Automation bias mitigation via structured friction — quantified via decision-time and false-positive error-rate shifts under FISWG enforcement vs. unstructured review.
- A clean architectural/legal boundary between Paper 1's tactical leads and this tier's evidentiary output.

**Evaluation Strategy — Requires Human-Subjects Testing**
Unlike Papers 1 and 2, this paper's central claims are behavioral (does structured friction reduce automation bias and false-positive identification errors?) and require an IRB-governed examiner study (N ≥ 10 law-enforcement or trained examiners), comparing structured ACE-VR review against an unstructured baseline.

**Target Journals**
- Forensic Science International: Digital Investigation
- IEEE Transactions on Human-Machine Systems
- ACM Transactions on Computer-Human Interaction (ToCHI), if reframed as a behavioral/HCI intervention

**What This Paper Includes**
- The FISWG morphological checklist UI and mandatory-completion gating.
- ACE-VR phase workflow (Analysis, Comparison, Evaluation, Verification).
- Blind dual-expert review, digital sign-off, and immutable report generation.

**What This Paper Explicitly Excludes**
- Tactical Queue mechanics and the entity-resolution/routing architecture (covered by Paper 1); those are cited as the source of leads entering this tier, not re-litigated.

---

## The Single Monorepo Strategy

**You do not split the open-source code.**

TrackID remains a single, powerful unified repository. The separation is **conceptual and publication-driven**, not architectural.

### Citation Chain

**In Paper 1**, cite TrackID but scope explicitly:
> "While TrackID includes a slow, evidentiary-grade verification tier and downstream graph intelligence capabilities, the scope of this paper is strictly limited to the tactical case-linking architecture and its no-human-testing simulation evaluation."

**In Paper 2**, cite Paper 1:
> "This paper assumes cases have already been linked via the tactical entity-resolution architecture validated in [Paper 1] and focuses on the macro-level graph intelligence those linkages enable."

**In Paper 3**, cite Paper 1:
> "This paper addresses the complementary problem to [Paper 1]: when a fast tactical lead must be escalated to a court-admissible identification, what verification architecture prevents automation bias while remaining tractable for practitioners?"

### Code Coverage

All three papers are embedded in the same codebase:

| Component | Paper 1 | Paper 2 | Paper 3 |
|-----------|---------|---------|---------|
| Face Detection (InsightFace) | ✓ Core | ✓ Input | ✓ Input |
| Vector Indexing (pgvector + HNSW) | ✓ Core | ✓ Used | ✗ Excluded |
| Spatio-Temporal Plausibility Gate | ✓ Core | ✗ Excluded | ✗ Excluded |
| Tactical Queue / Tripartite Routing | ✓ Core | ✗ Excluded | ✓ Input (leads) |
| Spatio-Temporal Graph | ✗ Excluded | ✓ Core | ✗ Excluded |
| Community Detection (Louvain) | ✗ Excluded | ✓ Core | ✗ Excluded |
| Centrality Analysis | ✗ Excluded | ✓ Core | ✗ Excluded |
| FISWG Checklist / ACE-VR Workflow | ✗ Excluded | ✗ Excluded | ✓ Core |
| Blind Dual-Expert Verification | ✗ Excluded | ✗ Excluded | ✓ Core |

---

## Publication Roadmap

### Phase 1: Paper 1 (Tactical Case Linking)
- **Timeline**: Months 1–6
- **Focus**: Tripartite routing, spatio-temporal plausibility, silo-breaking, workload compression
- **Deliverables**: Manuscript + simulation code/config for public-benchmark evaluation (Market-1501/MSMT17 partitioning scripts)
- **No ethics review required** — evaluation is fully simulated on public data
- **Target**: Decision Support Systems or Expert Systems with Applications (8–12 week review cycle)

### Phase 2: Paper 2 (Strategic Intelligence)
- **Timeline**: Months 4–12 (overlapping with Paper 1 review)
- **Focus**: Graph construction, community detection, temporal analysis
- **Deliverables**: Manuscript + Code (Louvain implementation, graph queries)
- **Dependency**: Cite Paper 1 once accepted/published
- **Target**: Expert Systems with Applications or similar (8–12 week review cycle)

### Phase 3: Paper 3 (Forensic Evidentiary Validation)
- **Timeline**: Months 9–18 (starts once ethics/IRB approval process is underway)
- **Focus**: ACE-VR workflow, FISWG compliance, automation-bias mitigation, dual-expert verification
- **Deliverables**: Manuscript + examiner study data (requires IRB approval, N ≥ 10 examiners)
- **Dependency**: Cite Paper 1 for the Tactical Queue escalation path
- **Target**: Forensic Science International: Digital Investigation (8–12 week review cycle)

### Phase 4: Follow-up / Extension Papers (Optional)
- Robustness of vector similarity under demographic variations
- Real-time graph updates (incremental community detection)
- Privacy-preserving graph compression for federated deployment
- Cross-jurisdictional threshold catalogs for the tripartite routing state machine

---

## Key Messaging

### For Reviewers (Paper 1)
"This paper advances tactical decision support by proving that spatio-temporal-constrained vector matching plus tripartite uncertainty routing collapses an intractable cross-case search problem into a short validation queue — evaluated entirely via simulation on public benchmarks, without any human-subjects testing. The evidentiary/legal verification tier and graph intelligence capabilities are orthogonal and explicitly out of scope."

### For Reviewers (Paper 2)
"This paper demonstrates that complex network analysis applied to video-derived identity graphs can reveal operational structure in criminal networks. The underlying entity resolution (validated separately in Paper 1) enables scalable identity linkage; this work focuses on what those linked cases reveal."

### For Reviewers (Paper 3)
"This paper demonstrates that structured, FISWG-enforced ACE-VR review mitigates automation bias relative to unstructured review, via a human-subjects examiner study. It sits above the fast tactical triage validated in Paper 1, addressing what happens when a lead must become court-admissible evidence."

### For Contributors & Users
"TrackID solves three related but distinct problems with a single, integrated platform: fast tactical case linking, strategic network intelligence, and slow forensic-grade verification. Read the papers to understand the research depth; the code shows how tactical linkage feeds both intelligence analysis and evidentiary review."

---

## References & Related Work

See `docs/PAPER1_TACTICAL_LINKING.md`, `docs/PAPER2_INTELLIGENCE.md`, and `docs/PAPER3_FORENSIC_EVIDENTIARY.md` for detailed literature reviews, methodology, and metrics for each paper.
