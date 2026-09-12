// ============================================================================
// MIGRATION: 001_init.cypher (Neo4j Edition)
// DESCRIPTION: Establishes uniqueness constraints,
//              existence rules, and performance indexes for the POLE+O
//              Identity & Biometric System, plus its identity-document ->
//              registration-event -> biometric-capture extension:
//
//                (:Person)
//                    -[:ASSOCIATED_WITH]->
//                (:Object:Document:Identity)
//                    -[:REGISTERED_BY]->
//                (:Event:IdentityRegister)
//                    -[:CAPTURES]->
//                (:Object:BiometricTrait:<FaceRecord|FingerprintTemplate|Finger>)
//
//              FingerprintTemplate is the 10-print capture as a whole;
//              Finger is a single-finger sample within it:
//
//                (:Object:BiometricTrait:FingerprintTemplate)
//                    -[:HAS_FINGER]->
//                (:Object:BiometricTrait:Finger)
//
//              REGISTERED_BY is a placeholder name — ISSUED_BY or
//              RECORDED_BY would also fit; pick one before applying.
//
//              NOTE: Neo4j constraints bind to exactly one label, so the
//              Identity/IdentityRegister constraints below target the most
//              specific label (:Identity, :IdentityRegister) rather than the
//              full multi-label pattern the nodes actually carry
//              (:Object:Document:Identity, :Event:IdentityRegister) — a
//              constraint on :Identity still applies to any node carrying
//              that label alongside others.
// ============================================================================

// ----------------------------------------------------------------------------
// 0. RELATIONSHIP SHAPE (documentation only — Neo4j has no relationship DDL
//    outside Enterprise property-existence constraints; example patterns
//    below show the shape these migrations expect writers to produce)
// ----------------------------------------------------------------------------

// (:Person {personId})
//   -[:ASSOCIATED_WITH]->
// (:Object:Document:Identity {documentId, documentType, identityNumber, issuedAt, issuingAuthority})
//   -[:REGISTERED_BY]->
// (:Event:IdentityRegister {eventId, registeredAt, station, operatorId})
//   -[:CAPTURES]->
// (:Object:BiometricTrait:FaceRecord {traitId, imageHash, capturedAt, quality})
//
// (:Event:IdentityRegister)
//   -[:CAPTURES]->
// (:Object:BiometricTrait:FingerprintTemplate {traitId, templateHash, format, capturedAt})
//   -[:HAS_FINGER]->
// (:Object:BiometricTrait:Finger {traitId, position, nfiqScore, capturedAt})

// ----------------------------------------------------------------------------
// 1. UNIQUENESS CONSTRAINTS (Prevent duplicate IDs and national identifiers)
// ----------------------------------------------------------------------------

CREATE CONSTRAINT unique_person_id IF NOT EXISTS
FOR (p:Person) REQUIRE p.personId IS UNIQUE;

CREATE CONSTRAINT unique_object_id IF NOT EXISTS
FOR (o:Object) REQUIRE o.objectId IS UNIQUE;

CREATE CONSTRAINT unique_event_id IF NOT EXISTS
FOR (e:Event) REQUIRE e.eventId IS UNIQUE;

CREATE CONSTRAINT unique_location_id IF NOT EXISTS
FOR (l:Location) REQUIRE l.locationId IS UNIQUE;

CREATE CONSTRAINT unique_org_id IF NOT EXISTS
FOR (org:Organization) REQUIRE org.orgId IS UNIQUE;

CREATE CONSTRAINT unique_identity_document_id IF NOT EXISTS
FOR (d:Identity) REQUIRE d.documentId IS UNIQUE;

CREATE CONSTRAINT unique_identity_register_event_id IF NOT EXISTS
FOR (e:IdentityRegister) REQUIRE e.eventId IS UNIQUE;

CREATE CONSTRAINT unique_biometric_trait_id IF NOT EXISTS
FOR (t:BiometricTrait) REQUIRE t.traitId IS UNIQUE;

// ----------------------------------------------------------------------------
// 2. EXISTENCE CONSTRAINTS (Ensure mandatory attributes always exist)
// Note: Neo4j uses 'IS NOT NULL' for property existence rules.
//
// COMMENTED OUT: property existence constraints are a Neo4j Enterprise
// feature — applying this file against the neo4j:5-community image in
// docker-compose.yml fails on the first one ("Property existence constraint
// requires Neo4j Enterprise Edition"). Kept here, disabled, as the intended
// shape in case this ever runs against Enterprise; uncomment if so.
// ----------------------------------------------------------------------------

// CREATE CONSTRAINT person_id_exists IF NOT EXISTS
// FOR (p:Person) REQUIRE p.personId IS NOT NULL;

// CREATE CONSTRAINT object_id_exists IF NOT EXISTS
// FOR (o:Object) REQUIRE o.objectId IS NOT NULL;

// CREATE CONSTRAINT event_id_exists IF NOT EXISTS
// FOR (e:Event) REQUIRE e.eventId IS NOT NULL;

// CREATE CONSTRAINT event_timestamp_exists IF NOT EXISTS
// FOR (e:Event) REQUIRE e.timestamp IS NOT NULL;

// CREATE CONSTRAINT identity_document_id_exists IF NOT EXISTS
// FOR (d:Identity) REQUIRE d.documentId IS NOT NULL;

// CREATE CONSTRAINT identity_document_type_exists IF NOT EXISTS
// FOR (d:Identity) REQUIRE d.documentType IS NOT NULL;

// CREATE CONSTRAINT identity_register_event_id_exists IF NOT EXISTS
// FOR (e:IdentityRegister) REQUIRE e.eventId IS NOT NULL;

// CREATE CONSTRAINT identity_register_timestamp_exists IF NOT EXISTS
// FOR (e:IdentityRegister) REQUIRE e.registeredAt IS NOT NULL;

// CREATE CONSTRAINT biometric_trait_id_exists IF NOT EXISTS
// FOR (t:BiometricTrait) REQUIRE t.traitId IS NOT NULL;

// ----------------------------------------------------------------------------
// 3. PERFORMANCE INDEXES (Optimize temporal queries and fraud checks)
// Note: a uniqueness constraint already creates its own backing index, so
// properties covered by section 1 (personId, documentId, eventId, traitId)
// are not re-indexed here.
// ----------------------------------------------------------------------------

CREATE INDEX event_timestamp_idx IF NOT EXISTS
FOR (e:Event) ON (e.timestamp);

CREATE INDEX event_type_idx IF NOT EXISTS
FOR (e:Event) ON (e.type);

CREATE INDEX object_hash_idx IF NOT EXISTS
FOR (o:Object) ON (o.hash);

CREATE INDEX object_status_idx IF NOT EXISTS
FOR (o:Object) ON (o.status);

CREATE INDEX object_type_idx IF NOT EXISTS
FOR (o:Object) ON (o.type);

CREATE INDEX person_cpf_idx IF NOT EXISTS
FOR (p:Person) ON (p.cpf);

CREATE INDEX identity_document_type_idx IF NOT EXISTS
FOR (d:Identity) ON (d.documentType);

CREATE INDEX identity_document_number_idx IF NOT EXISTS
FOR (d:Identity) ON (d.identityNumber);

CREATE INDEX identity_register_timestamp_idx IF NOT EXISTS
FOR (e:IdentityRegister) ON (e.registeredAt);

CREATE INDEX biometric_trait_captured_at_idx IF NOT EXISTS
FOR (t:BiometricTrait) ON (t.capturedAt);

CREATE INDEX finger_position_idx IF NOT EXISTS
FOR (f:Finger) ON (f.position);
