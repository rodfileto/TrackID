// ============================================================================
// MIGRATION: 004_identity_chain.cypher (Neo4j Edition)
// DESCRIPTION: Constrains the enrollment-side identity chain that
//              graph.SyncIdentity materializes from Postgres:
//
//                (:Person {personId})
//                  -[:HAS_IDENTITY]->
//                (:Object:Identification {documentId})
//                  -[:HAS_REGISTRATION]->
//                (:Object:IdentityRegister {registerId})
//                  -[:HAS_FEATURE]->
//                (:Object:BiometricFeature {featureId, featureType, provenance: "KNOWN"})
//
//              The Person and BiometricFeature nodes are already constrained
//              (unique_person_id from 001_init.cypher, unique_biometric_feature_id
//              from 002_forensic_evidence_capture.cypher). This migration pins the
//              two middle links, keyed on their Postgres business identifiers.
//
//              As elsewhere, constraints bind to the single most specific label
//              (:Identification, :IdentityRegister) rather than the full
//              multi-label pattern (:Object:Identification, :Object:IdentityRegister),
//              since Neo4j constraints cannot target more than one label.
// ============================================================================

CREATE CONSTRAINT unique_identification_document_id IF NOT EXISTS
FOR (d:Identification) REQUIRE d.documentId IS UNIQUE;

CREATE CONSTRAINT unique_identity_register_id IF NOT EXISTS
FOR (r:IdentityRegister) REQUIRE r.registerId IS UNIQUE;
