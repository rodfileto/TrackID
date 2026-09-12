// ============================================================================
// MIGRATION: 003_biometric_cluster.cypher (Neo4j Edition)
// DESCRIPTION: DRAFT — not applied yet. Models a biometric cluster: a group of
//              same-modality biometric samples believed to belong to one
//              subject, regardless of which chain each sample came from —
//              001_init.cypher's enrollment BiometricTrait or
//              002_forensic_evidence_capture.cypher's forensic
//              BiometricFeature:
//
//                (:Object:BiometricTrait)
//                    -[:IN_CLUSTER {confidence, addedAt}]->
//                (:Object:BiometricCluster:<Face|Fingerprint>)
//
//                (:Object:BiometricFeature)
//                    -[:IN_CLUSTER {confidence, addedAt}]->
//                (:Object:BiometricCluster:<Face|Fingerprint>)
//
//              A cluster is "known" or "unknown" purely by whether it has
//              been resolved to a real identity — there is no stored
//              status property/label for this. A status flag could drift
//              out of sync with reality (linked but left "unknown", or vice
//              versa); the relationship itself is the source of truth:
//
//                (:Object:BiometricCluster) -[:IDENTIFIED_AS {confidence, identifiedBy, identifiedAt}]-> (:Person)
//
//              "Known" query:   MATCH (c:BiometricCluster)-[:IDENTIFIED_AS]->(:Person)
//              "Unknown" query: MATCH (c:BiometricCluster) WHERE NOT (c)-[:IDENTIFIED_AS]->(:Person)
//
//              This is where the two biometric domains actually meet in
//              practice: a recurring, still-unidentified offender's latent
//              prints (BiometricFeature, from many crime scenes) can cluster
//              with each other long before any enrollment record exists, and
//              the moment a matching enrollment (BiometricTrait) is found or
//              an examiner confirms a name, IDENTIFIED_AS turns the same
//              cluster "known" without moving or re-keying any member.
//
//              NOTE: Neo4j has no way to enforce that a Face cluster only
//              contains FaceRecord/FaceCapture members and a Fingerprint
//              cluster only contains FingerprintTemplate/Finger/
//              FingerprintLift members (or that IN_CLUSTER only connects
//              BiometricTrait/BiometricFeature to BiometricCluster at all) —
//              same limitation as every relationship-shape note so far in
//              this migration set. Modality consistency is an application
//              responsibility.
//
//              NOTE: as in 001_init.cypher and 002_forensic_evidence_capture.cypher,
//              the constraint below binds to the single label :BiometricCluster
//              rather than the full multi-label pattern
//              (:Object:BiometricCluster:Face / :Object:BiometricCluster:Fingerprint),
//              since Neo4j constraints cannot target more than one label —
//              it still applies to nodes carrying either subtype.
// ============================================================================

// ----------------------------------------------------------------------------
// 0. RELATIONSHIP SHAPE (documentation only — see 001_init.cypher section 0
//    for why this isn't enforceable as real Neo4j DDL)
// ----------------------------------------------------------------------------

// (:Object:BiometricTrait) -[:IN_CLUSTER {confidence, addedAt}]-> (:Object:BiometricCluster:Face {clusterId, createdAt})
// (:Object:BiometricFeature) -[:IN_CLUSTER {confidence, addedAt}]-> (:Object:BiometricCluster:Face {clusterId, createdAt})
//
// (:Object:BiometricTrait) -[:IN_CLUSTER {confidence, addedAt}]-> (:Object:BiometricCluster:Fingerprint {clusterId, createdAt})
// (:Object:BiometricFeature) -[:IN_CLUSTER {confidence, addedAt}]-> (:Object:BiometricCluster:Fingerprint {clusterId, createdAt})
//
// (:Object:BiometricCluster) -[:IDENTIFIED_AS {confidence, identifiedBy, identifiedAt}]-> (:Person)

// ----------------------------------------------------------------------------
// 1. UNIQUENESS CONSTRAINTS
// ----------------------------------------------------------------------------

CREATE CONSTRAINT unique_biometric_cluster_id IF NOT EXISTS
FOR (c:BiometricCluster) REQUIRE c.clusterId IS UNIQUE;

// ----------------------------------------------------------------------------
// 2. EXISTENCE CONSTRAINTS
//
// COMMENTED OUT: property existence constraints are a Neo4j Enterprise
// feature, unavailable on the neo4j:5-community image in docker-compose.yml
// (see the matching note in 001_init.cypher). Kept here, disabled, as the
// intended shape in case this ever runs against Enterprise.
// ----------------------------------------------------------------------------

// CREATE CONSTRAINT biometric_cluster_id_exists IF NOT EXISTS
// FOR (c:BiometricCluster) REQUIRE c.clusterId IS NOT NULL;

// ----------------------------------------------------------------------------
// 3. PERFORMANCE INDEXES
// Note: clusterId is already covered by the uniqueness constraint above and
// not re-indexed. Face vs Fingerprint is a label, not a property, so it's
// covered by Neo4j's native label index — no separate modality index needed.
// ----------------------------------------------------------------------------

CREATE INDEX biometric_cluster_created_at_idx IF NOT EXISTS
FOR (c:BiometricCluster) ON (c.createdAt);

CREATE INDEX in_cluster_confidence_idx IF NOT EXISTS
FOR ()-[r:IN_CLUSTER]-() ON (r.confidence);

CREATE INDEX identified_as_confidence_idx IF NOT EXISTS
FOR ()-[r:IDENTIFIED_AS]-() ON (r.confidence);
