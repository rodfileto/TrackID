// ============================================================================
// MIGRATION: 002_forensic_evidence_capture.cypher (Neo4j Edition)
// DESCRIPTION: DRAFT — not applied yet. Models forensic evidence collection,
//              separate from (but linkable to) the identity-enrollment chain
//              in 001_init.cypher:
//
//                (:Object:Evidence)
//                    -[:COLLECTED_AT]->
//                (:Event:CriminalOccurrence)
//                    -[:OCCURRED_AT]->
//                (:Location)
//
//                (:Object:Evidence)
//                    -[:HAS_FEATURE]->
//                (:Object:BiometricFeature:<FingerprintLift|FaceCapture>)
//                    -[:COMPARED {confidence, examinerId, comparedAt, result}]->
//                (:Object:BiometricTrait)
//
//              COMPARED is not exclusive to Feature->Trait: it is a general
//              comparison relationship, so it also connects same-type nodes:
//
//                (:Object:BiometricTrait) -[:COMPARED]-> (:Object:BiometricTrait)
//                  // e.g. de-duplication: checking whether two enrolled
//                  // identities are actually the same person
//
//                (:Object:Evidence) -[:COMPARED]-> (:Object:Evidence)
//                  // e.g. crime linkage across occurrences via a forensic
//                  // discipline that never goes through BiometricFeature at
//                  // all (ballistics, tool marks, MO/signature analysis)
//
//              Not covered yet, but the same reasoning would extend to it:
//              BiometricFeature-to-BiometricFeature (e.g. two latent prints
//              from different scenes linking two occurrences) — flagging in
//              case that's also wanted.
//
//              A comparison can also be made accountable to the person who
//              made the call: :User (a system/examiner account, distinct
//              from :Person, which models criminal-case subjects) and
//              :Event:Decision (the comparison determination itself, as a
//              node rather than flat relationship properties) — see the
//              Decision-node relationship pattern below.
//
//              FingerprintLift covers a latent print lifted from a surface
//              or a photograph of one; FaceCapture covers a face pulled from
//              a scene photo or a video frame. Named FaceCapture rather than
//              FaceRecord (the enrollment-side label in 001_init.cypher) on
//              purpose — a constraint on a shared label would apply to both
//              populations at once, which we want to avoid (see NOTE below).
//
//              Option C (chosen over reusing :BiometricTrait directly):
//              a sample lifted from a crime scene is a distinct concept from
//              a controlled enrollment capture — different confidence,
//              chain-of-custody, and lifecycle, so it gets its own label,
//              :Object:BiometricFeature, instead of reusing
//              :Object:BiometricTrait from 001_init.cypher. The COMPARED
//              relationship is where the two chains meet: it is itself the
//              match determination (confidence/examiner/date), not just a
//              link, which is why the two samples are separate nodes rather
//              than one.
//
//              COLLECTED_AT / OCCURRED_AT / HAS_FEATURE are placeholder
//              names — pick final ones before applying.
//
//              NOTE: as in 001_init.cypher, constraints below bind to the
//              single most specific label (:Evidence, :CriminalOccurrence,
//              :BiometricFeature) rather than the full multi-label pattern
//              the nodes carry (:Object:Evidence, :Event:CriminalOccurrence,
//              :Object:BiometricFeature), since Neo4j constraints cannot
//              target more than one label. :CriminalOccurrence nodes also
//              carry :Event, so the base unique_event_id/event_id_exists/
//              event_timestamp_exists constraints from 001_init.cypher
//              already apply to them — not repeated here.
// ============================================================================

// ----------------------------------------------------------------------------
// 0. RELATIONSHIP SHAPE (documentation only — see 001_init.cypher section 0
//    for why this isn't enforceable as real Neo4j DDL)
// ----------------------------------------------------------------------------

// (:Object:Evidence {evidenceId, evidenceType, description, collectedAt, collectedBy})
//   -[:COLLECTED_AT]->
// (:Event:CriminalOccurrence {eventId, occurrenceNumber, timestamp, occurrenceType})
//   -[:OCCURRED_AT]->
// (:Location {locationId, ...})
//
// (:Object:Evidence)
//   -[:HAS_FEATURE]->
// (:Object:BiometricFeature:FingerprintLift {featureId, featureType, liftMethod, substrate, exhibitNumber, extractedAt})
//   -[:COMPARED {confidence, examinerId, comparedAt, result}]->
// (:Object:BiometricTrait)   // the enrolled trait from 001_init.cypher being matched against
//
// (:Object:Evidence)
//   -[:HAS_FEATURE]->
// (:Object:BiometricFeature:FaceCapture {featureId, featureType, sourceMedia, capturedAt, frameTimestamp, exhibitNumber})
//   -[:COMPARED {confidence, examinerId, comparedAt, result}]->
// (:Object:BiometricTrait)
//
// (:Object:BiometricTrait) -[:COMPARED {confidence, examinerId, comparedAt, result}]-> (:Object:BiometricTrait)
//
// (:Object:Evidence) -[:COMPARED {confidence, examinerId, comparedAt, result}]-> (:Object:Evidence)
//
// When the examiner needs to be a first-class, traversable node (e.g. "every
// decision examiner X made") rather than just an examinerId string, use this
// Decision-node form instead of flat COMPARED properties:
//
// (:Object:Evidence) -[:COMPARED]-> (:Event:Decision {decisionId, matchType, source})
// (:Event:Decision) -[:COMPARED]-> (:Object:Evidence)
// (:User {username}) -[:DECIDED]-> (:Event:Decision)

// ----------------------------------------------------------------------------
// 1. UNIQUENESS CONSTRAINTS
// ----------------------------------------------------------------------------

CREATE CONSTRAINT unique_evidence_id IF NOT EXISTS
FOR (ev:Evidence) REQUIRE ev.evidenceId IS UNIQUE;

CREATE CONSTRAINT unique_criminal_occurrence_number IF NOT EXISTS
FOR (o:CriminalOccurrence) REQUIRE o.occurrenceNumber IS UNIQUE;

CREATE CONSTRAINT unique_biometric_feature_id IF NOT EXISTS
FOR (f:BiometricFeature) REQUIRE f.featureId IS UNIQUE;

CREATE CONSTRAINT unique_username IF NOT EXISTS
FOR (u:User) REQUIRE u.username IS UNIQUE;

CREATE CONSTRAINT unique_decision_id IF NOT EXISTS
FOR (d:Decision) REQUIRE d.decisionId IS UNIQUE;

// ----------------------------------------------------------------------------
// 2. EXISTENCE CONSTRAINTS
//
// COMMENTED OUT: property existence constraints are a Neo4j Enterprise
// feature, unavailable on the neo4j:5-community image in docker-compose.yml
// (see the matching note in 001_init.cypher). Kept here, disabled, as the
// intended shape in case this ever runs against Enterprise.
// ----------------------------------------------------------------------------

// CREATE CONSTRAINT evidence_id_exists IF NOT EXISTS
// FOR (ev:Evidence) REQUIRE ev.evidenceId IS NOT NULL;

// CREATE CONSTRAINT evidence_type_exists IF NOT EXISTS
// FOR (ev:Evidence) REQUIRE ev.evidenceType IS NOT NULL;

// CREATE CONSTRAINT criminal_occurrence_number_exists IF NOT EXISTS
// FOR (o:CriminalOccurrence) REQUIRE o.occurrenceNumber IS NOT NULL;

// CREATE CONSTRAINT biometric_feature_id_exists IF NOT EXISTS
// FOR (f:BiometricFeature) REQUIRE f.featureId IS NOT NULL;

// CREATE CONSTRAINT biometric_feature_type_exists IF NOT EXISTS
// FOR (f:BiometricFeature) REQUIRE f.featureType IS NOT NULL;

// CREATE CONSTRAINT fingerprint_lift_method_exists IF NOT EXISTS
// FOR (f:FingerprintLift) REQUIRE f.liftMethod IS NOT NULL;

// CREATE CONSTRAINT face_capture_source_media_exists IF NOT EXISTS
// FOR (f:FaceCapture) REQUIRE f.sourceMedia IS NOT NULL;

// CREATE CONSTRAINT username_exists IF NOT EXISTS
// FOR (u:User) REQUIRE u.username IS NOT NULL;

// CREATE CONSTRAINT decision_id_exists IF NOT EXISTS
// FOR (d:Decision) REQUIRE d.decisionId IS NOT NULL;

// CREATE CONSTRAINT decision_match_type_exists IF NOT EXISTS
// FOR (d:Decision) REQUIRE d.matchType IS NOT NULL;

// ----------------------------------------------------------------------------
// 3. PERFORMANCE INDEXES
// Note: as in 001_init.cypher, properties already covered by a uniqueness
// constraint (evidenceId, occurrenceNumber, featureId) are not re-indexed.
// ----------------------------------------------------------------------------

CREATE INDEX evidence_type_idx IF NOT EXISTS
FOR (ev:Evidence) ON (ev.evidenceType);

CREATE INDEX evidence_collected_at_idx IF NOT EXISTS
FOR (ev:Evidence) ON (ev.collectedAt);

CREATE INDEX criminal_occurrence_type_idx IF NOT EXISTS
FOR (o:CriminalOccurrence) ON (o.occurrenceType);

CREATE INDEX biometric_feature_type_idx IF NOT EXISTS
FOR (f:BiometricFeature) ON (f.featureType);

CREATE INDEX biometric_feature_exhibit_number_idx IF NOT EXISTS
FOR (f:BiometricFeature) ON (f.exhibitNumber);

CREATE INDEX fingerprint_lift_method_idx IF NOT EXISTS
FOR (f:FingerprintLift) ON (f.liftMethod);

CREATE INDEX face_capture_source_media_idx IF NOT EXISTS
FOR (f:FaceCapture) ON (f.sourceMedia);

// COMPARED is polymorphic (Feature->Trait, Trait->Trait, Evidence->Evidence),
// so its properties are indexed on the relationship itself rather than
// duplicated per node-label pair.

CREATE INDEX compared_result_idx IF NOT EXISTS
FOR ()-[r:COMPARED]-() ON (r.result);

CREATE INDEX compared_at_idx IF NOT EXISTS
FOR ()-[r:COMPARED]-() ON (r.comparedAt);

CREATE INDEX decision_match_type_idx IF NOT EXISTS
FOR (d:Decision) ON (d.matchType);

CREATE INDEX decision_source_idx IF NOT EXISTS
FOR (d:Decision) ON (d.source);
