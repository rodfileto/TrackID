# TrackID Ontology: BFO + Common Core Ontologies (CCO)

Status: **implemented** — upper (BFO) + mid (CCO) layers imported, validated, and
persisted to Memgraph at startup. The `tid:` domain layer is deferred.

## Decision summary

TrackID's ontology is grounded in a standard three-level stack:

```
BFO 2.0          (upper)   — Continuant/Occurrent, MaterialEntity, InformationContentEntity, Process, …
  └─ CCO          (mid)     — Person, Organization, Event, Location, Artifact, Identifier, …
       └─ tid:    (domain)  — TrackID-specific types + resolution annotations (DEFERRED)
```

- **BFO 2.0** (ISO/IEC 21838) supplies the most general categories and the reasoning backbone (continuant/occurrent split, mereology, participation).
- **CCO (Common Core Ontologies)** supplies the mid-level classes we actually talk about. BSD 3-Clause (CUBRC, Inc.), and directed as the *baseline standard* for formal ontology across the US DoD and Intelligence Community — a direct fit for TrackID's target-centric intelligence positioning.
- **`tid:`** is a thin domain extension for what only TrackID needs (the `TargetEntity` biometric-cluster semantics, confidence-gate annotations) — not built yet.
- **No Pole+O layer** (see "Analyst vocabulary" below).

Semantic reasoning is structural, not inferential: the graph's schema *is* the semantics, with no OWL reasoner and no LLM at query time.

## Why CCO (not hand-rolled)

The earlier plan proposed a pruned BFO subset plus hand-written domain classes. That was the right shape under *bare BFO* (whose classes are too abstract for analysts). CCO changes the calculus:

1. CCO already defines the classes we need (`Person`, `Organization`, `Event`, `Location`, `Artifact`, `Identifier`, `Financial Account`), with definitions and axioms — we avoid re-inventing mid-level terms and inherit a peer-reviewed taxonomy.
2. It extends BFO + the Relation Ontology (RO), so the relation set (`has_part`, `participates_in`, `occurs_in`, …) also comes from an established source rather than our own.
3. It carries citation and interoperability value for the papers, and aligns with an emerging government standard.

**Adopted**: BFO (upper) + RO (relations) + CCO (mid-level) + `tid:` (domain, deferred).

## Vendored ontologies

Pinned under `backend/app/ontology/vendor/`:

- `bfo/bfo-core.ttl` — BFO 2.0 (ISO 21838-2), official `purl.obolibrary.org/obo/BFO_*` IRIs.
- `cco/v2.2/*.ttl` — the full CCO suite (all 11 modules), release `v2.2`, namespace `https://www.commoncoreontologies.org/`.

CCO classes use opaque numeric IRIs (`…/ont00001262`) with human-readable `rdfs:label`s, so identity is the IRI and the label is a display/`name` property. Imports total **1437 classes** (37 BFO + 1400 CCO) and **177 relations** (40 BFO + 137 CCO).

## Type mapping (reference for the deferred `tid:` layer)

| TrackID type (today) | CCO class | BFO anchor |
|---|---|---|
| `TargetPerson` | `Person` | `bfo:Object` (via `Animal → Organism`) |
| `Organization` | `Organization` | `bfo:ObjectAggregate` (via `Group of Agents`) |
| `Place` | `Geospatial Location` | `bfo:Site` (immaterial) |
| `Vehicle` | `Vehicle`/`Artifact` | `bfo:Object` |
| `PhoneNumber` | `Identifier` (contact info) | `bfo:InformationContentEntity` |
| `FinancialAccount` | `Financial Account`/instrument | `bfo:InformationContentEntity` |
| `Situation` | `Event` | `bfo:Process` |
| `Event` | `Event` (instantaneous → process boundary) | `bfo:Process` |
| `Concept` (Reconnaissance, Logistics, …) | `Act`/`Process` subclasses | `bfo:Process` |

Confirmed against CCO v2.2: `Person` sits under `Animal → Organism → bfo:Object` (not `Agent`), and `Organization` sits under `Group of Agents → bfo:ObjectAggregate`. `Agent` is a distinct class (a material entity that *bears* an agent capability), related to persons/organizations by role rather than subclass. We defer to CCO's classification rather than hand-rolling it.

## Analyst vocabulary: CCO directly, not Pole+O

Earlier iterations proposed a Pole+O layer (`Person, Organization, Location, Event, Object`) as the analyst-facing vocabulary, shielding analysts from BFO's abstract classes.

With CCO adopted, Pole+O is **redundant and dropped**:

- CCO's mid-level classes are *already* the plain-English vocabulary (`Person`, `Organization`, `Event`, `Location`, `Artifact`), so a separate 5-bucket layer duplicates them.
- Pole+O's "Object" bucket was ambiguous (it blurred physical objects like `Vehicle` with information entities like `PhoneNumber`). CCO's precise classes (`Artifact` vs `Identifier` vs `Financial Account`) resolve that ambiguity, not reintroduce it.

What survives is a **curated analyst surface**: a config-level whitelist of *which* CCO classes are offered in the UI/API for analysts to assign, so the full CCO taxonomy stays a system concern while analysts see only the handful they need. This is a presentation/filter concern, not a semantic layer — it lives in the `tid:`/API layer, not as an ontology.

## Physical object vs. digital representation

BFO's most valuable distinction for TrackID is cleanly formalized here:

| Kind | BFO/CCO class |
|---|---|
| The suspect, their face, a vehicle | `bfo:MaterialEntity` / `bfo:Object` |
| A photo, a video frame, a 512-d embedding, a DB row | `bfo:InformationContentEntity` (CCO: `Image` / `Measurement Information Content Entity` / `Identifier`) |
| The disk/camera the bytes live on | `bfo:MaterialEntity` (the bearer) |

The "aboutness" relation (`is_about` / `depicts`, from IAO, and `concretizes`) links a digital representation to its physical referent. Two subtleties matter:

1. **Face vs. person** — the face is `continuant_part_of` the person (still material); a *photo of the face* is information *about* that part.
2. **Embedding vs. photo** — the 512-d vector is a *derived* representation (information about a photo about a face), two steps from the physical object. This is exactly why Paper 3 (evidentiary) ranks an embedding match below a pixel-level comparison.

TrackID's architecture already embodies this: Postgres holds artifacts (embeddings, media blobs) while Memgraph holds entities, linked by UUID (`Event.embedding_id`). CCO's Information Entity Ontology gives that split its formal name — the `:Person` node is the referent; the `ResolutionEmbedding` row is the digital representation.

## Representation in Memgraph (hybrid)

Persisted at startup (`seed_taxonomy`, see below):

- **Class nodes** — `(:OntologyClass {iri, name, definition, source})`, one per BFO/CCO class, connected by `SUBCLASS_OF` edges. A unique constraint on `iri` makes `MERGE` genuinely unique and enables O(1) lookup.
- **Relation nodes** — `(:ObjectProperty {iri, name, definition, transitive, source})`, connected to their domain/range classes by `DOMAIN`/`RANGE` edges.
- **Multi-label stamping** — a helper (`Taxonomy.lineage_labels`) computes a type's full label lineage for stamping onto instance nodes, e.g. a `:Person` instance also carries `:Animal :Organism :Object :MaterialEntity :IndependentContinuant :Continuant :Entity`, so `MATCH (n:MaterialEntity)` is O(1) with no traversal. Implemented but not yet *applied* — there is no instance layer yet.

## Import pipeline

1. `app/ontology/importer.py` parses the vendored TTL into an in-memory `Taxonomy`: classes (`owl:Class` + `rdfs:subClassOf` + label + definition) and object properties (`owl:ObjectProperty` + label + domain/range + transitivity + `subPropertyOf`/`inverseOf`). Domain/range class expressions (`unionOf`/`intersectionOf`/restrictions) are flattened to the named classes they include; `complementOf` is skipped.
2. `app/ontology/validation.py` guards structure before it reaches Memgraph: `validate_edge` (domain/range via subclass closure), `validate_lineage` (unknown class/parent, cycles), plus `is_occurrent`/`is_continuant`/`is_material_entity`/`is_information_content_entity` conveniences.
3. `app/ontology/persistence.py` writes the taxonomy to Memgraph: `write_taxonomy` (idempotent `MERGE`, `UNWIND`-batched) and `clear_taxonomy` (drop + rebuild).
4. `app/core/ontology.py` wires it into startup: cached `load_taxonomy()`/`get_taxonomy()`, `ensure_constraints()`, and `seed_taxonomy()` — invoked from `main.py`'s `lifespan` after Memgraph connectivity is verified (non-fatal: the app still boots without Memgraph).

## Ingest validation

`app/ontology/validation.py` is a pure-Python guard (no graph dependency) over the imported taxonomy. `validate_edge(subject, relation, object)` rejects domain/range violations — e.g. `Process -[continuant part of]-> Object` is rejected because `process` is an `occurrent`, not a `continuant` — and `validate_lineage` rejects unknown classes and ancestry cycles. Invalid structure is blocked at the door, not discovered later.

## Verification

Two complementary checks, both run over the merged BFO + CCO + `tid:` set:

- **Conformance** (`python -m app.ontology.validation`) — a pure-Python, reasoner-free check that domain (`tid:`) classes are anchored to the BFO/CCO backbone: each has a parent that exists, its lineage reaches `bfo:Entity`, and it does not (transitively) fall under two disjoint classes (e.g. nothing is both a `continuant` and an `occurrent`). The lighter structural counterpart to the reasoner.
- **Consistency** (`python -m app.ontology.reasoner`) — runs a DL reasoner (HermiT, via ROBOT — see `scripts/fetch_robot.sh`, requires Java) over the merged ontology and reports unsatisfiable classes and the OWL profile (BFO+CCO is OWL 2 DL). This is the logical check: a logically inconsistent `tid:` addition (a class under two disjoint classes) makes the ontology unsatisfiable and is flagged here.

Adding a domain TTL to `vendor/tid/` is picked up by both; run either before persisting.

## Module structure

```
app/ontology/
├── vendor/
│   ├── bfo/bfo-core.ttl          # BFO 2.0
│   └── cco/v2.2/*.ttl            # all 11 CCO modules
├── models.py                     # ClassNode, Relation, Taxonomy (+ sanitize_label, lineage_labels)
├── importer.py                   # TTL -> Taxonomy (classes + object properties)
├── validation.py                 # domain/range + lineage guard; conformance check
├── reasoner.py                   # DL reasoner pass (HermiT via ROBOT) for consistency
└── persistence.py                # write_taxonomy / clear_taxonomy (Memgraph)
app/core/ontology.py              # cached load + startup seed + uniqueness constraints
```

## Clean start (legacy removal)

The prior template/category system was removed before this work began:

- Deleted `app/ontology/templates/`, `app/ontology/entity_types/`, `app/ontology/schemas.py`, `app/api/ontology.py`, `app/schemas/ontology.py`.
- Dropped the `ontology` router from `main.py`; `api/targets.py` and `api/entities.py` now accept free-string `target_type`/`entity_type` until the `tid:` registry is wired back in.

No graph/persistence behavior changed (`entity_resolution_service.py` referenced `TARGET_PERSON` only in a comment).

## Deferred / remaining

- **`tid:` domain layer** — TrackID-specific types (biometrics: face/fingerprint/iris and their digital representations) and per-type confidence-gate defaults, anchored as CCO subclasses. Deliberately deferred.
- **Multi-label stamping on instances** — the helper exists; it will be applied when an instance layer (typed nodes) exists.
- **Curated analyst surface** — the UI/API whitelist of which CCO classes analysts assign.
- `app/graph/service.py` is unchanged; no alembic migration; the instance-layer ontology (Target/Situation/Event/TargetEntity) is untouched.

## Paper implications

`docs/DTID_ARCHITECTURE.md` and `papers/paper1-tactical-linking` already name an extensible `tid:` micro-ontology. Grounding it in BFO + CCO (a DoD/IC baseline standard) materially strengthens the Paper 1 positioning: an explicit upper- and mid-level ontology foundation, with a thin domain extension — a reviewer-recognizable, defensible stack.
