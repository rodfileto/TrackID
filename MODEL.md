# TrackID — Forensic Data Model

This document explains the core data model: how forensic biometric data (fingerprints,
faces) and forensic intelligence are structured across Postgres and Neo4j. It is the
specification to implement against — the evidence hierarchy and the biometric-feature
concept are the heart of the package.

## 1. The unifying concept: `BiometricFeature`

Everything in the system reduces to one atomic thing that gets compared: a **biometric
feature**. A feature is *one* biometric sample — a fingerprint template, an enrolled face
photo, a latent print lifted from a scene, a face detected in evidence footage.

Every feature has:

- `feature_type` — the modality and granularity:
  - `FINGERPRINT_TEMPLATE` — an enrolled ten-print (a full set), **KNOWN**
  - `FACE_RECORD` — an enrolled face photo, **KNOWN**
  - `FINGERPRINT_LIFT` — a latent print from a scene, **QUESTIONED**
  - `FACE_CAPTURE` — a face detected in evidence, **QUESTIONED**
- `provenance` — `KNOWN` (came from an enrollment) vs `QUESTIONED` (came from crime-scene
  evidence). Provenance is derivable from the feature's source, but stored explicitly.

Two mirrored hierarchies produce features. A **decision** (match/no-match) is always made
between *two features*. A **cluster** is a group of same-modality features believed to
belong to one person.

## 2. The two hierarchies

### 2.1 Enrollment hierarchy (KNOWN)

```
person ──< identity_document ──< identity_register ──< identity_file ── biometricfeature
```

| Table | Role |
| --- | --- |
| `person` | the real individual (stable business key `person_id`) |
| `identity_document` | an identity document (e.g. an ID card, an ABIS enrollment record); optionally carries the holder's government-issued fiscal/taxpayer number |
| `identity_register` | one enrollment event under a document (biographic data: name, parents, birthdate) |
| `identity_file` | a raw file produced by that enrollment: `photo` (face), `nist` (ten-print), `pdf`, … |
| `biometricfeature` | the typed feature(s) extracted from a file (a NIST file → one `FINGERPRINT_TEMPLATE`; a photo → one `FACE_RECORD`) |

A person can hold many documents; a document can hold many registers; a register can hold
many files; each file yields one or more features.

### 2.2 Evidence hierarchy (QUESTIONED)

```
biometric_cases ──< case_files
biometric_cases ──< case_evidences ──< case_traces ──< case_codifications
case_traces ── biometricfeature
```

| Table | Role |
| --- | --- |
| `biometric_cases` | the base case (`case_id`; `case_type` = `CRIMINAL` \| `CIVIL`; `modality` = `FACIAL` \| `FINGERPRINT`) |
| `case_files` | raw files attached to a case (`evidence`, `documento`, `forensic_report`, `face_crop`, `codification_image`) |
| `case_evidences` | one evidence item within a case (e.g. a lift card, an image) |
| `case_traces` | one biometric trace within an evidence (one face in an image, one lift on a card) |
| `case_codifications` | one processed encoding of a trace (e.g. a minutiae set, an embedding) |
| `biometricfeature` | the typed QUESTIONED feature for a trace |

A biometric case is any case with questioned biometric material to identify. `case_type`
is its legal nature: `CRIMINAL` (e.g. latent prints from a crime scene) or `CIVIL`
(non-criminal identification such as disaster victim identification or unidentified dead
bodies). `modality` is the biometric the case works with. The two are independent, and
the rest of the hierarchy is the same for both case types.

A codification is a processing artifact, not a comparison record — the pairwise comparison
outcome lives entirely in `biometric_decisions` (section 3), keyed by the two features'
graph ids rather than by codification.

## 3. Decisions (`biometric_decisions`)

An append-only event log of every decision made about a pair of features. This is the
**source of truth** for a pair's confirmation state; cluster membership/status in Neo4j is
a *derived view*, never written authoritatively to the graph.

| Column | Meaning |
| --- | --- |
| `feature_a_id`, `feature_b_id` | the two feature ids (always stored `a < b`) |
| `modality` | `FINGERPRINT` \| `FACE` |
| `role` | `SYSTEM` (automatic), `VERIFICATOR`, `REVIEWER`, `INCONSISTENCE` |
| `decision` | `POSITIVE` \| `NEGATIVE` \| `INCONCLUSIVE` |
| `system_source` / `username` | who/what decided (exactly one set, per role) |
| `confidence`, `threshold` | score and the cutoff it was classified against (SYSTEM only) |
| `notes` | examiner rationale (esp. on INCONSISTENCE) |
| `decided_at` | when |

A pair progresses through roles (SYSTEM → VERIFICATOR → REVIEWER, with INCONSISTENCE only
on disagreement); the full chain matters for audit, not just the outcome.

## 4. Clustering

`clusters` / `cluster_members` / `cluster_merges` persist stable, citable cluster numbers:

- A cluster is created once and its id is never reused or renumbered.
- Two features join the same cluster when a confirmed (POSITIVE) decision links them.
- A cluster **merges** when a new decision links two existing clusters (oldest id survives,
  recorded in `cluster_merges`).
- A cluster **splits** when a decision is revoked (the component containing the smallest
  feature id keeps the id; the rest get new ids).
- A singleton (one feature, no match) is still an unknown-subject cluster.

## 5. Neo4j mapping

```
(:Person {personId})
  -[:HAS_IDENTITY]->
(:Object:Identification {documentId})
  -[:HAS_REGISTRATION]->
(:Object:IdentityRegister {registerId})
  -[:HAS_FEATURE]->
(:Object:BiometricFeature {featureId, featureType, provenance: "KNOWN"})

(:Object:Evidence {evidenceId, evidenceType})
  -[:HAS_FEATURE]->
(:Object:BiometricFeature {featureId, featureType, provenance: "QUESTIONED"})

(:Object:BiometricFeature) -[:IN_CLUSTER]-> (:Object:BiometricCluster {clusterId})
(:Object:BiometricCluster) -[:IDENTIFIED_AS {confidence, identifiedBy, identifiedAt}]-> (:Person)
(:Person) -[:SAME_AS {method, status, confidence}]-> (:Person)   // person↔person merge (later)
```

Feature id conventions (stable, so callers never round-trip through Neo4j to discover one):

- enrolled (KNOWN): the bare `identity_file.id`
- questioned (evidence): `"TRACE:<case_trace_id>#feature"`

## 6. Flow

```
organization import cmd ──► Postgres (the tables above)
                               │
    sync-graph          ──► Evidence + BiometricFeature          (Neo4j)
    sync-identity       ──► Person / Identification / IdentityRegister / KNOWN features
    cluster-biometrics  ──► BiometricCluster + IN_CLUSTER       (Neo4j)
    identify            ──► IDENTIFIED_AS cluster → person       (Neo4j)
```

Every logic entry point reads Postgres — never an external file or a third-party system.

File bytes live in object storage (MinIO/S3), referenced by `storage_ref`. The layout is
trackid's, content-addressed by sha256: `identity/<register>/<file_type>/<sha256>`,
`evidence/<case>/<sha256>` and `codification/<case>/<trace>/<sha256>`. An import cmd holding
raw bytes uploads them with `identity.UploadFile` / `cases.UploadEvidenceFile`, which return the
`FileInput` to pass to `Ingest`; `Ingest` itself never touches storage.

## 7. Implementation status

Done (core), end to end:

- **Schema** (`db/migrations/001`–`008`): `users`; `person`, `identity_document`,
  `identity_register`, `identity_file` (the enrollment hierarchy); `biometric_cases`,
  `case_files`, `case_evidences`, `case_traces`, `case_codifications` (+
  `case_fragment_codes` view) (the evidence hierarchy); `biometricfeature` (unified
  KNOWN+QUESTIONED, exactly-one-source constraint against `identity_file`/`case_traces`);
  `biometric_decisions` (append-only decision log); `clusters`/`cluster_members`/
  `cluster_merges`. This is the model's target shape written directly — since nothing had
  shipped yet, there was no reason to carry the intermediate tables (`comparisons`,
  `codifications`, `identifications`, a `case_codifications.comparison_*` set of columns)
  that an incremental build-up would otherwise leave behind.
- **Sync** (`graph` package): `graph.Sync` materializes Evidence + QUESTIONED
  `BiometricFeature` nodes from the evidence hierarchy; `graph.SyncIdentity` materializes
  Person + the Identification/IdentityRegister/KNOWN-`BiometricFeature` chain from
  `biometricfeature`/`identity_file` (`graph/migrations/004_identity_chain.cypher` pins the
  constraints).
- **Decisions and clustering** (`cluster` package): `cluster.DeriveEdgeStatus`
  (`cluster/decision.go`, unit-tested) turns a pair's `biometric_decisions` chain into
  PENDING_REVIEW/CONFIRMED/DISPUTED/REJECTED; `cluster.Run` derives clusters from CONFIRMED
  pairs; `cluster.Identify` derives `IDENTIFIED_AS` from a CONFIRMED decision linking a
  QUESTIONED feature to a KNOWN one.
- **Commands**: `sync-graph`, `sync-identity`, `cluster-biometrics`, `identify` (all
  dry-run by default, `-commit` to write).

Verified against a live Postgres: the full migration chain applies cleanly (goose version 6
reached), and a smoke insert through
`identity_register → identity_file → biometricfeature (KNOWN)` and
`case_evidences → case_traces → biometricfeature (QUESTIONED)`, joined by one
`biometric_decisions` row, reproduces the expected cluster (`cluster.Run`) and identification
(`cluster.Identify`).

**Not in core** (by design — core is the shared model/library; ingestion is a downstream
concern): an organization import command that populates these tables from source files.
Downstream apps (e.g. trackid-pf) call the `db` package's generated queries
(`UpsertIdentityFile`, `UpsertBiometricFeatureFromIdentityFile`/`FromCaseTrace`,
`UpsertCaseFile`/`CaseEvidence`/`CaseTrace`/`CaseCodification`, `InsertBiometricDecision`, …)
directly, the same way `auth` calls `CreateUser`.
