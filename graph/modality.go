package graph

import "fmt"

// ModalityMapping describes how a criminal_cases.case_type maps onto Neo4j
// labels and type values. It is the single place that connects the relational
// modality to the graph schema (see migrations/001_init.cypher,
// migrations/002_forensic_evidence_capture.cypher, and
// migrations/003_biometric_cluster.cypher).
type ModalityMapping struct {
	// EvidenceType is the value stored on Object:Evidence.evidenceType.
	EvidenceType string
	// FeatureType is the value stored on BiometricFeature.featureType for a
	// QUESTIONED feature of this modality.
	FeatureType string
	// ClusterLabel is the BiometricCluster sub-label (e.g. "Fingerprint").
	ClusterLabel string
}

// Modalities maps each supported criminal_cases.case_type to its graph
// representation.
var Modalities = map[string]ModalityMapping{
	"FACIAL": {
		EvidenceType: "FACE_CAPTURE",
		FeatureType:  FeatureTypeFaceCapture,
		ClusterLabel: "Face",
	},
	"FINGERPRINT": {
		EvidenceType: "LATENT_FINGERPRINT",
		FeatureType:  FeatureTypeFingerprintLift,
		ClusterLabel: "Fingerprint",
	},
}

// Feature types, per MODEL.md: the modality and provenance of every feature.
const (
	// FeatureTypeFingerprintTemplate is an enrolled ten-print, KNOWN.
	FeatureTypeFingerprintTemplate = "FINGERPRINT_TEMPLATE"
	// FeatureTypeFaceRecord is an enrolled face photo, KNOWN.
	FeatureTypeFaceRecord = "FACE_RECORD"
	// FeatureTypeFingerprintLift is a latent print lifted from a scene, QUESTIONED.
	FeatureTypeFingerprintLift = "FINGERPRINT_LIFT"
	// FeatureTypeFaceCapture is a face detected in evidence, QUESTIONED.
	FeatureTypeFaceCapture = "FACE_CAPTURE"
)

// FeatureTypeForFileType maps an identity_file.file_type to the KNOWN feature
// type it yields. ok is false for file types that don't produce a biometric
// feature (e.g. a pdf).
func FeatureTypeForFileType(fileType string) (string, bool) {
	switch fileType {
	case "photo":
		return FeatureTypeFaceRecord, true
	case "nist":
		return FeatureTypeFingerprintTemplate, true
	default:
		return "", false
	}
}

// FeatureTypeForTraceType maps a case_traces.trace_type to the QUESTIONED
// feature type it yields.
func FeatureTypeForTraceType(traceType string) (string, bool) {
	switch traceType {
	case "FACE_RECORD":
		return FeatureTypeFaceCapture, true
	case "FINGERPRINT_LIFT":
		return FeatureTypeFingerprintLift, true
	default:
		return "", false
	}
}

// Codification types, per MODEL.md: the processing artifact a trace's
// case_codifications row records.
const (
	// CodificationTypeFaceEmbedding is a face trace's embedding artifact.
	CodificationTypeFaceEmbedding = "FACE_EMBEDDING"
	// CodificationTypeMinutiae is a fingerprint lift's minutiae artifact.
	CodificationTypeMinutiae = "MINUTIAE"
)

// CodificationTypeForTraceType maps a case_traces.trace_type to the
// case_codifications.codification_type its processing artifact gets.
func CodificationTypeForTraceType(traceType string) (string, bool) {
	switch traceType {
	case "FACE_RECORD":
		return CodificationTypeFaceEmbedding, true
	case "FINGERPRINT_LIFT":
		return CodificationTypeMinutiae, true
	default:
		return "", false
	}
}

// TraceTypeForCaseType maps a criminal_cases.case_type to the
// case_traces.trace_type value a trace marked on evidence of that modality
// gets: one face in an image for FACIAL, one fingerprint lift on a card for
// FINGERPRINT. Both are just a bounding box on the evidence image -- how the
// box is produced (drawn by hand, or -- FACIAL only -- auto-detected) doesn't
// change the trace_type.
func TraceTypeForCaseType(caseType string) (string, bool) {
	switch caseType {
	case "FACIAL":
		return "FACE_RECORD", true
	case "FINGERPRINT":
		return "FINGERPRINT_LIFT", true
	default:
		return "", false
	}
}

// CaseTypeForFeatureType maps a feature type to its criminal_cases.case_type
// (the modality vocabulary used by the clusters table and graph.Modalities).
func CaseTypeForFeatureType(featureType string) (string, bool) {
	switch featureType {
	case FeatureTypeFaceRecord, FeatureTypeFaceCapture:
		return "FACIAL", true
	case FeatureTypeFingerprintTemplate, FeatureTypeFingerprintLift:
		return "FINGERPRINT", true
	default:
		return "", false
	}
}

// KnownFeatureID returns the graph node id for an enrolled (KNOWN) feature: the
// bare identity_file.id. Callers never round-trip through Neo4j to discover it.
func KnownFeatureID(identityFileID int64) string {
	return fmt.Sprintf("%d", identityFileID)
}

// QuestionedFeatureID returns the graph node id for a case_trace's QUESTIONED
// biometric feature. The "TRACE:" prefix keeps case_trace ids (a distinct
// Postgres bigserial sequence) from colliding with identity_file ids.
func QuestionedFeatureID(caseTraceID int64) string {
	return fmt.Sprintf("TRACE:%d#feature", caseTraceID)
}
