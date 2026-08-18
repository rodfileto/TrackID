# Paper 2: Network Analysis — Surfacing a Target's Model of Functioning

**This document describes the evaluation strategy for Paper 2: analysis of a Target's network of entities.** For full system context and architecture, see [`docs/DTID_ARCHITECTURE.md`](DTID_ARCHITECTURE.md). This paper is built on the `Person` entity resolution output from Paper 1; it evaluates the claim that analyzing the network of entities for a Target reveals that Target's **Model of Functioning** via graph-topological methods (community detection, centrality analysis, link prediction).

## Title (Working)
**Surfacing a Target's Model of Functioning: Network Analysis over Resolved Entities via Spatio-Temporal Graph Methods**

## Scope & Positioning

Paper 1 resolves individual `Person` entities (Person Entity Profiles) from fragmented Event-level observations. This paper picks up where Paper 1 leaves off: given a Target's fully populated network of entities — the raw graph of resolved `Person` entities and their co-occurrence relationships across the Target's Situations and Events — what does analyzing that network reveal about *how the Target operates*? The paper assumes entity resolution has already happened and focuses entirely on the second of DTID's two derived representations: the **Model of Functioning**, built on top of the (already-existing) network of entities.

**Core problem**: a raw network of entities — nodes and co-occurrence edges — doesn't by itself reveal hidden structure: choke points, operational cells, key persons, how relationships evolve over time. That structure only becomes visible once network-analysis methods are applied on top of the raw graph, transforming "who was observed with whom" into "how does this operation actually function."

## Approach

A heterogeneous spatio-temporal graph is built over the Person Entity Profiles that Paper 1 produces for a given Target: nodes are resolved `Person` entities, edges are spatio-temporal co-occurrence (derived from shared participation in the Target's Situations and Events), and edge weight reflects frequency and temporal/spatial proximity. Standard network-analysis techniques are applied on top of this graph to infer the Model of Functioning:

- **Community detection** (e.g. Louvain-style modularity optimization) to surface operational cells, including how stable those cells are across time windows.
- **Centrality measures** (betweenness, eigenvector) to identify key persons — bottlenecks, bridges, and hubs.
- **Temporal decay modeling** of edge weight, to characterize how relationship strength evolves and to support link prediction — forecasting future co-occurrence.

The novelty is not any individual network-science technique, but applying them specifically over a graph whose nodes are already-resolved, provenance-tracked Person Entity Profiles rather than raw, unlinked observations — i.e., treating the Model of Functioning as a distinct analytical artifact derived from, but not identical to, the raw network of entities.

## Evaluation Register

This paper's evaluation register is graph-topological rather than IS/DSS or forensic — distinct from both Paper 1 and Paper 3. Community-detection quality is assessed against known or ground-truth cell structure (e.g. modularity, agreement with known grouping); centrality rankings are validated against known organizational hierarchy where available; temporal-decay/link-prediction quality is assessed via standard link-prediction metrics. Where real ground truth isn't available, a synthetic stress test (e.g. planted community structure within a simulated Target such as "a bank-robbery series in region X," per Paper 1's evaluation design) validates that the methods recover known structure before applying them to messier real data.

## Discussion Points

- **Associational, not causal**: co-occurrence is treated as evidence of association, not proof of collaboration; causal claims are out of scope.
- **Dependence on Paper 1's output quality**: this paper assumes Person Entity Profiles are already resolved; errors in entity resolution propagate into the network, which motivates confidence-weighted edges rather than treating all resolved entities as equally certain.
- **Privacy and legal admissibility**: the Model of Functioning is discussed as an intelligence product, not evidence — that distinction is exactly the gap Paper 3 addresses for individual identifications, and applies analogously here.

## Limitations & Future Work

Assumes reasonably complete entity resolution and reasonably clean co-occurrence data; does not address false negatives upstream in entity resolution, real-time/incremental graph updates, or generalization of the Model of Functioning across different types of Targets. Cross-domain and cross-dataset validation of the decay/link-prediction model is left to future work.

## Conclusion

This paper demonstrates that analyzing the network of entities Paper 1 produces for a Target — via community detection, centrality, and temporal-decay methods — surfaces that Target's Model of Functioning, revealing organizational structure (operational cells, key persons, evolving relationships) that no individual case file exposes. This is the second of DTID's two derived representations per Target, distinct from and built on top of the raw network of entities.

## Target Journals & Keywords

Network-science and applied-AI venues whose audience expects graph-topological evaluation (community detection, centrality, link prediction) rather than DSS/IS or forensic framing.

**Keywords**: target-centric intelligence, model of functioning, criminal networks, community detection, centrality, spatio-temporal graphs, link prediction, organizational structure.
