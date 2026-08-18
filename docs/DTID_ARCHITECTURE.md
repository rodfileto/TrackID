# DTID: Target-Centric Identity Platform — Architecture

## Overview

DTID is a target-centric intelligence platform built around Clark's **target-centric approach to intelligence analysis**. In this approach, a **Target** is simply the object of interest — it can be as broad as a criminal organization, a drug-trafficking network, or an assessment of a government's stability, or as narrow as a localized crime series. A Target is not an individual identity, and it is not a fixed hierarchy of "levels" — it is whatever scenario an investigation or analytical effort is organized around.

DTID represents each Target as an instance of a small, explicit **ontology** — Target, Situation, Event, Target Entity — stored as a knowledge graph. Two distinct analytical representations are then built on top of that graph for any given Target: a **network of entities** (the raw structure of who/what was involved and how they relate) and a **model of functioning** (a higher-level account of how the target actually operates). Facial recognition is the platform's mechanism for resolving one specific Target Entity type — `TargetPerson` — across the Events and Situations that make up a Target, which is what allows a suspect observed in one incident to be recognized as the same person observed in another.

The architecture is single-server deployable by default; federation and multi-agency capability are optional extensions, not day-one requirements.

## Terminology: Target System vs. Target Entity

The word "Target" is used deliberately at two different levels of abstraction in this architecture. Academic reviewers can easily conflate them, so the distinction is stated explicitly here and should be preserved wherever "Target" appears in project documentation or papers:

- **Target System (Clark's macro target)**: the overarching problem domain an investigation is organized around — e.g. "Illicit Trafficking Network in District A," or this project's running example, "Bank robberies in region X." This is what the `Target` ontology class (above) represents, and it is what the *entire* DTID knowledge graph for that investigation constitutes once populated with Situations, Events, and Target Entities.
- **Target Entity (micro target)**: a specific node within that graph that field officers interact with operationally — `TargetPerson`, `Vehicle`, `PhoneNumber`, and other domain-relevant types. A Target Entity is a participant *in* a Target System, not a Target System itself.

Framed this way, DTID is the software bridge between the two levels: it ingests observations about discrete Target Entities (a face capture, a plate read, a phone ping) and automatically constructs the broader Target System — the networked model — required for strategic, intelligence-led policing. Every use of "Target" elsewhere in this document (Target/Situation/Event/Target Entity ontology, "a Target's network of entities," "a Target's Model of Functioning") refers to the Target System sense unless the node type `TargetPerson`, `Vehicle`, etc. is named explicitly.

## Core Ontology

DTID's knowledge graph is built from four classes:

### Target
The Target System (Clark's sense, macro): the object of interest. Example: **"Bank robberies in region X"** — a crime series treated as a single analytical scope. A Target is the top-level container; everything else in the ontology exists in service of understanding one.

### Situation
A descriptive account of one incident within a Target's scope. Example: one specific bank robbery is a Situation. A Target has many Situations.

### Event
A fine-grained occurrence within a Situation. Examples: "suspect entered the bank at 14:02," "camera 3 captured a face at 14:03," "getaway vehicle departed at 14:05." A Situation has many Events. Events are where raw observations — face captures, field notes, document scans — enter the system.

### Target Entity
A typed participant referenced by Events and Situations — the micro-target sense from above: `TargetPerson`, `Vehicle`, `PhoneNumber`, and other domain-relevant types. Target Entities are not owned by a single Situation — the same `TargetPerson` can appear across multiple Situations within a Target (and, in principle, across Targets). Recognizing that recurrence is what breaks case silos: an unidentified suspect in one robbery and an unidentified suspect in another are only linkable once both are resolved to the same `TargetPerson`. Face-based entity resolution (InsightFace/ArcFace embeddings + confidence-gated matching) is how `TargetPerson` entities are resolved from Event-level observations; the platform's ontology is deliberately generic about other Target Entity types and other resolution modalities, so this is one instantiation of the pattern, not the whole of it.

## Two Derived Representations per Target

Once a Target System's Situations, Events, and Target Entities are populated, two distinct analytical layers can be built on top — these are **not the same artifact** and should not be conflated:

### Network of Entities
The raw knowledge graph itself: nodes are Target Entities (and, where useful, Situations/Events), edges are relationships — participation, co-occurrence (weighted by spatio-temporal proximity and frequency), and provenance. This is a structural, largely mechanical representation — it records *what* was observed and *how things relate*, without asserting *why*.

### Model of Functioning
A higher-level, interpretive account of *how the Target System operates* — roles (e.g., driver, lookout, financier), operational workflow (e.g., reconnaissance → robbery → getaway), and how that structure evolves over time. The Model of Functioning is built **on top of** the Network of Entities (via community detection, centrality analysis, and temporal modeling — see Paper Mapping below) rather than being read directly off the raw graph. Two Target Systems can have structurally similar entity networks and very different models of functioning, or vice versa — the two representations answer different questions.

## Storage Architecture

### Storage Model: Knowledge Graph + Relational

Two complementary data stores, logically coupled by UUID pointers:

**Memgraph (Cypher graph database)** — the canonical ontology and relationship store
- Node types: `Target`, `Situation`, `Event`, Target Entity types (`TargetPerson`, `Vehicle`, `PhoneNumber`, ...), `AnalystAction`
- Edge types: `HAS_SITUATION` (Target→Situation), `HAS_EVENT` (Situation→Event), `PARTICIPATES_IN` (Target Entity→Event/Situation), `CO_OCCURS_WITH` (Target Entity↔Target Entity, weighted), `AFFECTS` (AnalystAction→node), `CORRECTS` (node→prior node)

**PostgreSQL (relational + pgvector)** — artifacts and metadata
- **Embeddings**: 512-d ArcFace vectors (cosine-normalized) for `TargetPerson` resolution, indexed for similarity search
- **Media blobs/references**: face images, field documents (as paths or object-storage URIs)
- **Case metadata**: jurisdiction, assigned investigators, Target-level administrative context
- **Retention timers**: purge-after dates and status for compliance
- **Audit blob storage**: signed `AnalystAction` payloads too large for the graph

### Linkage & Immutability

- **UUID pointers only**: no duplication of embeddings, media, or large objects between stores — Postgres holds the artifact, Memgraph references it by UUID.
- **Append-only principle**: `Event` and `AnalystAction` nodes are never updated. Corrections create new nodes with a `CORRECTS` edge back to the node being revised. This yields audit trail, versioning, and frozen forensic snapshots without a separate history system.

## Confidence-Gated Entity Resolution (DR2)

The confidence gate is how `TargetPerson` entities get resolved and linked across Events and Situations — it is the platform's central control loop, not a feature specific to one paper. Every observation-bearing Event enters the gate:

```
Event arrives (e.g. a face capture)
   ↓
[Target Entity Resolution + Plausibility Check]
   ↓
Compute confidence τ (0 to 1) against candidate TargetPerson entities
   ↓
τ >= τ_high?
  YES → Auto-commit: attach Event to the existing TargetPerson
        Log AnalystAction (auto-match)
        If this creates a new cross-Situation link, push WebSocket alert to field devices
  NO → τ >= τ_low?
        YES → Enqueue for analyst review (tactical queue)
        NO → Discard / log only
```

**Policy auditability**: every gate decision is itself logged as an `AnalystAction` node referencing the Event — the gate logic is auditable, not opaque.

**Threshold tuning**: τ_high and τ_low are policy parameters, not fixed constants — they can differ per Target System, per Target Entity type, or per investigative context, and every threshold change is itself an `AnalystAction`.

## Dual-Mode Delivery (DR3)

### Live Mode: WebSocket/SSE to Field
High-confidence auto-commits (τ >= τ_high) push in real time: new cross-Situation links, co-occurrence alerts (two watched Target Entities appearing together), analyst queue resolutions.

### Static Mode: Dossier Snapshots
A **dossier** is a point-in-time Cypher query over a Target's Situations, Events, and Target Entities, rendered to PDF/Markdown:

```
MATCH (t:Target {id: $target_uuid})-[:HAS_SITUATION]->(s:Situation)-[:HAS_EVENT]->(e:Event)
MATCH (p:TargetPerson)-[:PARTICIPATES_IN]->(e)
OPTIONAL MATCH (p)-[co:CO_OCCURS_WITH]-(p2:TargetPerson)
OPTIONAL MATCH (p)<-[:AFFECTS]-(action:AnalystAction)
RETURN t, s, e, p, co, action
```

The dossier is a **generated view**, not a separately maintained document — it captures graph state at a single point in time, including all evidence, actions, and cryptographic signatures needed for legal or intelligence review.

## Ingestion Consistency

Event ingestion is **idempotent**, keyed by Event UUID:

1. An Event arrives with a UUID (client- or server-generated)
2. **Atomically**: write to PostgreSQL (embedding, media reference) AND write to Memgraph (`Event` node + `PARTICIPATES_IN`/`HAS_EVENT` edges) as one logical transaction
   - Partial failure (e.g. Postgres succeeds, Memgraph fails) is marked for retry, not silently dropped
3. Run the confidence gate
4. Log the gate decision as an `AnalystAction`

A retried Event with the same UUID is a no-op, not a duplicate.

## Governance & Compliance

Governance is schema, not policy — baked into the data model:

### Retention & Purge
- `Target`, `Situation`, and Target Entity nodes carry a `retention_policy` field (jurisdiction-specific purge rule, e.g. "GDPR_RIGHT_TO_BE_FORGOTTEN," "BIPA_KEEP_7_YEARS").
- A background purge job scans for expired nodes and removes them (graph node, Postgres record, media blob together); the deletion itself is logged as a system `AnalystAction` with a reason code — no silent deletes.

### Cryptographic Signing
- Every `AnalystAction` is signed with the analyst's key (or an HSM-managed key in high-security deployments); the signature is stored on the node, making tampering detectable.
- Dossiers include the full signature chain for legal review.

### Jurisdictional Rules & Compliance
- At Event ingestion, region-specific rules are checked (GDPR consent, BIPA notice, state/local restrictions). Non-compliant Events are quarantined — stored for audit but excluded from confidence-gate routing and intelligence queries until resolved.
- These rules vary by jurisdiction and change often; this check is a standing requirement, not a one-time gate before prototype sign-off.

## Instrumentation & Metrics

Instrumented from day one, tied directly to evaluation goals:

- **Query latency** (p50/p99) for common Cypher patterns: candidate-match lookup for an incoming Event, `AnalystAction` history for a Target Entity, network analysis over a Target System's entity graph.
- **Confidence-gate metrics**: auto-commit rate, manual-review rate, discard rate, and analyst-reported false-positive rate in the review queue.
- **Time-to-alert**: latency from Event ingestion to WebSocket push for high-confidence cross-Situation links.
- **DSR metrics** (Paper 1): silo-breaking rate, routing accuracy, workload compression.
- **Forensic metrics** (Paper 3): examiner agreement (Cohen's kappa), decision time, false-positive error rate under structured vs. unstructured review.

## Modularity for Small LEAs

- **Single-server default**: one Memgraph instance + one PostgreSQL instance on a single host, suitable for a mid-sized department or regional task force; scales vertically before requiring distribution.
- **Pluggable Re-ID engine**: the embedding model behind `TargetPerson` resolution (currently InsightFace/ArcFace) sits behind a stable interface — swappable without touching the ontology or graph schema.
- **Future federation** (out of scope for now): multiple DTID instances federated via a gateway for privacy-respecting cross-jurisdiction queries; no assumption of shared trust or infrastructure between agencies.

## Paper Mapping

- **Paper 1 — General Architecture**: introduces the ontology (Target/Situation/Event/Target Entity), the knowledge-graph + relational storage architecture, and the confidence-gated entity-resolution workflow. The paper's evaluated capability is `TargetPerson` resolution (face-based) and cross-Situation case-linking within a simulated Target System ("a synthetic bank-robbery series") — this is the demonstration vehicle for the architecture, not a scope limit on the platform.
- **Paper 2 — Network Analysis / Model of Functioning**: takes the network of entities that Paper 1's architecture produces for a Target System and analyzes it (community detection, centrality, temporal decay, link prediction) to surface that Target System's Model of Functioning — operational cells, key persons, and how the operation evolves over time.
- **Paper 3 — Evidentiary Verification**: addresses when a `TargetPerson`'s resolved identification must be escalated from an operational lead to court-admissible evidence, via a structured, protocol-enforced verification workflow.

All three papers evaluate one platform; they are published separately because their evaluation registers (DSS/IS, graph-topological, behavioral/forensic) are incompatible within a single methodology section — see `docs/RESEARCH_STRATEGY.md`.

## References

- Clark, R. M. (2013). *Intelligence Analysis: A Target-Centric Approach*. CQ Press. — theoretical anchor for the Target System / Target Entity distinction and the Target/Situation/Event/Target Entity ontology
- JDL data fusion model (Level 1 object refinement) — fusion methodology for Target Entity-level observations
- Memgraph documentation — graph database schema and Cypher query optimization
- PostgreSQL pgvector extension — embedding storage and similarity search
- FISWG (Facial Identification Scientific Working Group), ACE-VR (Analysis, Comparison, Evaluation, Verification, Review) — forensic facial-comparison protocol standards
