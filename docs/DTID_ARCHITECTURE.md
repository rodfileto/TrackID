# DTID: Distributed Target-Centric Identity Platform — Architecture

## Overview

**DTID** is a target-centric facial-recognition and case-linking platform that treats identity, networks, and evidentiary records as distinct, composable **targets** in the intelligence sense. Rather than a linear pipeline, DTID builds an event-sourced graph model where observations accumulate continuously, confidence gates route findings to appropriate decision-makers (automated commit, analyst queue, or discard), and the same immutable fact-set supports both rapid operational intelligence and rigorous forensic/legal verification.

The architecture is single-server deployable by default; federation and multi-agency capability are optional extensions, not day-one requirements.

## Deployment Tiers

DTID composes three operational tiers, each a recursive instance of the target-centric model applied at different scales:

### 1. Identity Tier (Tactical)
**Target**: a single individual's identity, continuously resolved from fragmented observations.

Observations (face images from cameras, field notes, documentary evidence) arrive asynchronously from disconnected investigations. The system applies face-based entity resolution + spatio-temporal plausibility filtering to decide whether an observation belongs to an existing Person-Target Profile or creates a new one. A tripartite confidence gate routes the decision:
- **τ_high** → auto-commit to the profile (operational lead confirmed)
- **τ_low–τ_high** → enqueue for one-click analyst validation
- **< τ_low** → discard (log only, no action)

Output: Person-Target Profiles (continuously updated identity records), each with a multi-source evidence ledger (biometric auto-match, analyst validation, field-officer document confirmation) and full provenance chain.

### 2. Network Tier (Strategic)
**Target**: a situation, criminal series, or organizational structure, composed from resolved identities.

Once Person-Target Profiles exist, the network tier applies spatio-temporal graph analysis to reveal structure invisible in individual case files: operational cells (via community detection), key persons (via centrality), relationship evolution (via temporal decay), and link prediction for forecasting future co-occurrence. Nodes in this graph are the Person-Target Profiles from the identity tier; edges represent co-occurrence and its confidence.

Output: Situation/Network-Target Profiles — intelligence products describing organizational structure, but explicitly not evidence (per the identity tier's legal boundary).

### 3. Evidentiary Tier (Forensic)
**Target**: a Person-Target Profile escalated to court-admissible evidence.

When an identity-tier profile must support a legal proceeding, it enters a separate, deliberately slow verification workflow that mirrors forensic facial-comparison protocol standards (FISWG, ACE-VR). This tier enforces:
- Structured, mandatory comparison checklist (anatomical features)
- Blind dual-expert review (two examiners work independently)
- Immutable, separately-signed output (never shares legal weight with the fast tactical tier)
- Full chain-of-custody documentation

Output: Evidentiary-grade identification reports, cryptographically signed and auditable.

## System Architecture

### Storage Model: Event-Sourced Graph + Relational

The platform uses two complementary data stores, logically coupled by UUID pointers:

**Memgraph (Cypher graph database)**
- **Canonical relationship and provenance model**
- Node types:
  - `TargetPerson` — a resolved individual identity (Person-Target Profile)
  - `ReIDObservation` — a single biometric/documentary observation (face image, field note, etc.)
  - `AnalystAction` — a human decision or system action in the investigation (match confirmation, evidence escalation, etc.)
  - `AnalysisContext` — a case, series, or investigation grouping observations and analysts
- Edge types:
  - Co-occurrence (spatio-temporal, weighted by frequency and proximity)
  - Evidence-of (ReIDObservation → TargetPerson, confidence-tagged)
  - Provenance (action → affected node)
  - Temporal edges (for link prediction, decay modeling)

**PostgreSQL (relational + pgvector)**
- **Embeddings**: 512-d ArcFace vectors (cosine-normalized), indexed for similarity search
- **Media blobs/references**: JPEG/PNG paths or S3 URIs for face images, field documents
- **Case metadata**: investigation context, case number, jurisdiction, assigned investigators
- **Retention timers**: purge-after dates and status for compliance with regional data-retention rules
- **Audit blob storage**: signed AnalystAction payloads that are too large for the graph

### Linkage & Immutability

- **UUID pointers only**: no duplication of embeddings, media, or large objects between stores. PostgreSQL holds the artifact; Memgraph references it by UUID.
- **Append-only principle**: `ReIDObservation` and `AnalystAction` nodes are never updated. Corrections create new nodes with back-references (`corrects` edge) to the node being revised. This yields:
  - Audit trail automatically (every change is a new node)
  - Versioning without a separate history table
  - Forensic snapshots frozen in time (point-in-time Cypher queries)

## Confidence-Gated Workflow (DR2)

The confidence gate is the central control loop, not just Paper 1's feature. Every observation enters the gate; the gate decides the observation's fate:

```
Observation arrives
   ↓
[Entity Resolution + Plausibility Check]
   ↓
Compute confidence τ (0 to 1)
   ↓
τ >= τ_high?
  YES → Auto-commit to Person-Target Profile
        Log AnalystAction (auto-match)
        If identity is new, push WebSocket alert to field devices
  NO → τ >= τ_low?
        YES → Enqueue for analyst review (tactical queue)
              Notify analyst via dashboard
        NO → Discard / log only
              (No alert, no queue entry)
```

**Policy auditability**: every confidence decision is itself logged as an `AnalystAction` node referencing the observation. This makes the gate logic auditable, not opaque — investigators can see exactly why a match was auto-committed or queued.

**Threshold tuning**: τ_high and τ_low are not fixed constants but **policy parameters** that can differ per investigation, target, or time window:
- Conservative settings (τ_high = 0.9, τ_low = 0.7): high precision, longer analyst queues
- Aggressive settings (τ_high = 0.7, τ_low = 0.4): faster tactical response, more false positives for review
- Adaptive settings: thresholds adjusted based on target-profile age, source diversity, or investigative context

This tuning is itself auditable — every threshold-change action is an `AnalystAction` node.

## Dual-Mode Delivery (DR3)

### Live Mode: WebSocket/SSE to Field
High-confidence auto-commits (τ >= τ_high) are pushed in real time to field devices via WebSocket or Server-Sent Events:
- New match alerts (identity found)
- Location/co-occurrence alerts (two watched identities appear together)
- Status updates (analyst finished reviewing a queue item)

No polling, no latency — field teams see new leads as they arrive.

### Static Mode: Dossier Snapshots
At investigation close or on-demand, a point-in-time **dossier** is generated by running a Cypher query over the append-only graph and rendering it to PDF/Markdown:

```
MATCH (p:TargetPerson {id: $target_uuid})
RETURN p, 
  [(p)<-[e:EVIDENCE_OF]-(obs:ReIDObservation) | obs],
  [(p)-[co:COOCCURS_WITH]-(p2:TargetPerson) | {person: p2, confidence: co.weight}],
  [(p)<-[pa:AFFECTS]-(action:AnalystAction) | action]
UNION
... [collect metadata, signatures, chain-of-custody records] ...
```

The dossier is a **generated view**, not a separately maintained document. It captures the graph state at a single point in time and includes all evidence, actions, and metadata needed for legal or intelligence review. The PDF includes cryptographic signatures (from AnalystActions) and retention-policy stamps.

## Ingestion Consistency

All observation ingestion is **idempotent**, keyed by observation UUID:

1. Incoming observation arrives with UUID (generated client-side or server-side, depending on source)
2. **Atomically**: write to PostgreSQL (`embeddings` table, `observations` table) AND write to Memgraph (`ReIDObservation` node) as a single logical transaction
   - If Postgres succeeds but Memgraph fails, mark the Postgres record with a retry flag and re-attempt the Memgraph write asynchronously
   - If both succeed, the observation is canonical in both stores
3. Run confidence gate (happens inside or immediately after step 2)
4. Log the gate decision as an `AnalystAction` node

Idempotency means: if a client retries an observation ingestion (e.g., due to network loss), the UUID matches the one already in the database, and the system skips the write (returns 200 OK with "already present") rather than duplicating it.

## Governance & Compliance

**Governance is schema, not policy** — baked into the data model, not enforced externally:

### Retention & Purge
- Every `TargetPerson` and `ReIDObservation` node has a `retention_policy` field: a jurisdiction-specific purge rule (e.g., "GDPR_RIGHT_TO_BE_FORGOTTEN", "BIPA_KEEP_7_YEARS", "CALIFORNIA_DELETE_AFTER_180_DAYS").
- A background purge job periodically scans for nodes that have exceeded their retention date and marks them for deletion (including their corresponding PostgreSQL records and media blobs).
- The deletion itself is logged as an `AnalystAction` (system action) with a reason code and timestamp — no silent deletes.

### Cryptographic Signing
- Every `AnalystAction` node is signed with the analyst's private key (or, in high-security deployments, an HSM-managed key).
- The signature is stored as a field on the node, making the action immutable — tampering with an action's content breaks the signature.
- Dossiers include the chain of signatures, allowing legal reviewers to audit the full decision chain.

### Jurisdictional Rules & Compliance
- At observation ingestion, the system checks the observation's jurisdiction and enforces region-specific rules:
  - **GDPR**: requires explicit consent for biometric processing; observations without consent are quarantined
  - **BIPA**: Illinois state law requiring notice and consent for facial recognition
  - **State/local rules**: varies by jurisdiction (California AB 701 bans certain uses, etc.)
- Non-compliant observations are logged with a compliance flag but still stored (for audit/legal purposes); they are excluded from confidence-gate routing and intelligence queries until compliance is resolved.
- This checking happens at the API boundary, not retrospectively.

## Instrumentation & Metrics

Metrics are instrumented from day one, tied directly to evaluation goals:

### Query Latency
- Cypher query execution time (p50, p99) for common patterns:
  - Find all co-occurrence candidates for a face embedding: `MATCH (o:ReIDObservation {embedding_uuid: $uuid}) MATCH (p:TargetPerson) WHERE cosine_similarity(o.embedding, p.embedding) > 0.4 RETURN p`
  - List all AnalystActions affecting a Person-Target Profile: `MATCH (p:TargetPerson {id: $uuid})<-[a:AFFECTS]-(action:AnalystAction) RETURN action ORDER BY action.timestamp DESC`
  - Network analysis (community detection over co-occurrence edges) on graphs of varying sizes

### Confidence-Gate Metrics
- **Auto-commit rate**: fraction of observations routed to τ >= τ_high (operational intelligence shipped without human review)
- **Manual-review rate**: fraction of observations in τ_low–τ_high (analyst queue size as a proxy for workload)
- **Discard rate**: fraction < τ_low (tuning for signal-to-noise tradeoff)
- **False-positive rate in queue**: analyst-reported "this match is wrong" / "correct" ratio (feedback loop to retrain thresholds)

### Time-to-Alert
- Latency from observation ingestion to WebSocket push for high-confidence matches (e-to-e wall-clock time)
- Latency from analyst action (validation) to queue-update push

### DSR (Design Science Research) Metrics
- **Silo-breaking rate**: fraction of true cross-case identity links recovered (precision/recall)
- **Routing accuracy**: how well τ_high/τ_low separate genuinely ambiguous candidates from confident decisions
- **Workload compression**: reduction in analyst task complexity (ratio of pre-system to post-system pairwise comparisons)

### Forensic Metrics (Evidentiary Tier)
- **Examiner agreement**: inter-rater reliability (Cohen's kappa) for blind dual-expert review
- **Decision time**: examiner time per verification decision (structured protocol vs. unstructured baseline)
- **False-positive error rate**: number of wrong identifications agreed upon by both examiners

## Modularity for Small LEAs

DTID is designed for single-server deployment; multi-agency federation is a later extension.

### Single-Server Default
- One Memgraph instance + one PostgreSQL instance on a single host (or co-located cluster)
- No distributed consensus, no cross-server replication complexity
- Suitable for a mid-sized city police department or regional task force
- Scales vertically (bigger machines) before requiring horizontal distribution

### Pluggable Re-ID Engine
The embedding model (face detection + ArcFace vectors) is **swappable** behind a stable interface:
- Current: InsightFace/ArcFace (commodity, fast, CPU/GPU flexible)
- Alternative: proprietary model from vendor X, Y, or Z (drop-in replacement)
- Custom: LEA trains its own embedding model (fine-tuned on local suspects or regional demographics)

The graph schema and confidence-gate logic are independent of the embedding model — changing models requires only recomputing embeddings for existing observations, not schema changes.

### Future Federation (Out of Scope)
- Multiple DTID instances in different jurisdictions can be federated via a gateway service
- Gateway performs privacy-respecting cross-jurisdiction queries (e.g., does suspect X appear in any other agency's database?)
- No assumption of trust or shared infrastructure between agencies
- Details deferred to a later architecture doc

## Migration from Papers 1–3

The three research papers describe evaluation strategies for each tier:

- **Paper 1 (Identity Tier Evaluation)**: DSS/IS evaluation register (silo-breaking rate, routing accuracy, workload compression) — validates the identity-level tier's core claim
- **Paper 2 (Network Tier Evaluation)**: Graph-topological evaluation register (community detection, centrality, link prediction) — validates that composing profiles into networks reveals organizational structure
- **Paper 3 (Evidentiary Tier Evaluation)**: Behavioral/HCI evaluation register (examiner study: automation-bias reduction, inter-rater agreement, false-positive errors) — validates that structured verification mitigates legal/forensic risks

All three tiers are production-ready in the codebase; papers evaluate them independently, but they coexist and share the same event-sourced graph.

## References

- Clark, R. M. (2013). *Intelligence Analysis: A Target-Centric Approach*. CQ Press. — theoretical anchor for target-centric recursion
- JDL data fusion model (Level 1 object refinement) — fusion methodology for identity-level observations
- Memgraph documentation — graph database schema and Cypher query optimization
- PostgreSQL pgvector extension — embedding storage and similarity search
- FISWG (Facial Identification Scientific Working Group), ACE-VR (Analysis, Comparison, Evaluation, Verification, Review) — forensic facial-comparison protocol standards
