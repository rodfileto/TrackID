# Paper 2: Situation/Network-Target Profiles

## Title (Working)
**Situation/Network-Target Profiles: Composing Person-Target Identities into Organizational Structure via Spatio-Temporal Network Analysis**

## Scope & Positioning

Where Paper 1 recurses the target-centric model down to a single identity (a Person-Target Profile), this paper recurses it back up — one level of the target hierarchy above the individual. Here the target is a **situation**: a criminal series or an organizational structure, treated as a first-class entity composed from the Person-Target Profiles that Paper 1 produces. The paper assumes identity resolution has already happened and focuses entirely on what emerges when those resolved identities are related to one another in space and time.

**Core problem**: flat databases of individually-linked identities don't reveal hidden structure — choke points, operational cells, key persons, how relationships evolve over time. That structure only becomes visible once person-level targets are composed into a network.

## Approach

A heterogeneous spatio-temporal graph is built over the Person-Target Profiles from Paper 1: nodes are resolved identities, edges are spatio-temporal co-occurrence, and edge weight reflects frequency and temporal/spatial proximity. Standard network-analysis techniques are applied on top of this graph:

- **Community detection** (e.g. Louvain-style modularity optimization) to surface operational cells, including how stable those cells are across time windows.
- **Centrality measures** (betweenness, eigenvector) to identify key persons — bottlenecks, bridges, and hubs.
- **Temporal decay modeling** of edge weight, to characterize how relationship strength evolves and to support link prediction — forecasting future co-occurrence.

The novelty is not any individual network-science technique, but applying them specifically over a graph whose nodes are already-resolved, provenance-tracked Person-Target Profiles rather than raw, unlinked observations — i.e., treating the *situation* itself, not just the individual, as a first-class target that composes from person-level targets.

## Evaluation Register

This paper's evaluation register is graph-topological rather than IS/DSS or forensic — distinct from both Paper 1 and Paper 3. Community-detection quality is assessed against known or ground-truth cell structure (e.g. modularity, agreement with known grouping); centrality rankings are validated against known organizational hierarchy where available; temporal-decay/link-prediction quality is assessed via standard link-prediction metrics. Where real ground truth isn't available, a synthetic stress test (e.g. planted community structure) validates that the methods recover known structure before applying them to messier real data.

## Discussion Points

- **Associational, not causal**: co-occurrence is treated as evidence of association, not proof of collaboration; causal claims are out of scope.
- **Dependence on Paper 1's output quality**: this paper assumes Person-Target Profiles are already resolved; errors in identity resolution propagate into the network, which motivates confidence-weighted edges rather than treating all resolved identities as equally certain.
- **Privacy and legal admissibility**: network-level findings are discussed as intelligence products, not evidence — that distinction is exactly the gap Paper 3 addresses at the identity level, and applies analogously here.

## Limitations & Future Work

Assumes reasonably complete identity resolution and reasonably clean co-occurrence data; does not address false negatives upstream in entity resolution, real-time/incremental graph updates, or generalization of community structure across different types of organizations. Cross-domain and cross-dataset validation of the decay/link-prediction model is left to future work.

## Conclusion

This paper demonstrates that composing individually-resolved Person-Target Profiles into a spatio-temporal network makes organizational structure visible that no individual case file exposes — operationalizing the next level of target-centric recursion above the individual, and treating a "situation" as a target in its own right rather than an incidental grouping of unrelated people.

## Target Journals & Keywords

Network-science and applied-AI venues whose audience expects graph-topological evaluation (community detection, centrality, link prediction) rather than DSS/IS or forensic framing.

**Keywords**: target-centric intelligence, criminal networks, community detection, centrality, spatio-temporal graphs, link prediction, organizational structure.
