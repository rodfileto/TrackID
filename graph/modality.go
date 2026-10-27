// Package graph holds the feature conventions the rest of TrackID shares: the feature types
// (FACE_RECORD, FACE_CAPTURE, FINGERPRINT_TEMPLATE, FINGERPRINT_LIFT), how a file, trace or
// modality maps onto them, and the stable feature ids biometric_decisions and the clusters key on
// (a bare identity_file id for a KNOWN feature, "TRACE:<case_trace_id>#feature" for a QUESTIONED
// one). The name is historical: the features once also lived in a graph database.
package graph

import (
	"fmt"
	"strconv"
	"strings"
)

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
// feature type it yields. A trace type is that feature type, so this only
// checks that it is one.
func FeatureTypeForTraceType(traceType string) (string, bool) {
	switch traceType {
	case FeatureTypeFaceCapture, FeatureTypeFingerprintLift:
		return traceType, true
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
	case FeatureTypeFaceCapture:
		return CodificationTypeFaceEmbedding, true
	case FeatureTypeFingerprintLift:
		return CodificationTypeMinutiae, true
	default:
		return "", false
	}
}

// TraceTypeForModality maps a biometric_cases.modality to the
// case_traces.trace_type value a trace marked on evidence of that modality
// gets: one face in an image for FACIAL, one fingerprint lift on a card for
// FINGERPRINT. Both are just a bounding box on the evidence image -- how the
// box is produced (drawn by hand, or -- FACIAL only -- auto-detected) doesn't
// change the trace_type.
func TraceTypeForModality(modality string) (string, bool) {
	switch modality {
	case "FACIAL":
		return FeatureTypeFaceCapture, true
	case "FINGERPRINT":
		return FeatureTypeFingerprintLift, true
	default:
		return "", false
	}
}

// ModalityForFeatureType maps a feature type to its biometric_cases.modality
// (the modality vocabulary used by the clusters table).
func ModalityForFeatureType(featureType string) (string, bool) {
	switch featureType {
	case FeatureTypeFaceRecord, FeatureTypeFaceCapture:
		return "FACIAL", true
	case FeatureTypeFingerprintTemplate, FeatureTypeFingerprintLift:
		return "FINGERPRINT", true
	default:
		return "", false
	}
}

// KnownFeatureID returns the feature id of an enrolled (KNOWN) feature: the
// bare identity_file.id.
func KnownFeatureID(identityFileID int64) string {
	return fmt.Sprintf("%d", identityFileID)
}

// QuestionedFeatureID returns the feature id of a case_trace's QUESTIONED
// biometric feature. The "TRACE:" prefix keeps case_trace ids (a distinct
// Postgres bigserial sequence) from colliding with identity_file ids.
func QuestionedFeatureID(caseTraceID int64) string {
	return fmt.Sprintf("TRACE:%d#feature", caseTraceID)
}

// ParseQuestionedFeatureID is the inverse of QuestionedFeatureID: it recovers
// the case_trace_id from a feature id, so a caller that only has
// cluster_members.feature_id (e.g. after resolving a cluster's membership)
// can join back to case_traces. ok is
// false for a KNOWN member (a bare identity_file id, with no "TRACE:" prefix)
// or a malformed string.
func ParseQuestionedFeatureID(featureID string) (caseTraceID int64, ok bool) {
	rest, ok := strings.CutPrefix(featureID, "TRACE:")
	if !ok {
		return 0, false
	}
	rest, ok = strings.CutSuffix(rest, "#feature")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
