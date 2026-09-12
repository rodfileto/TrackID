package graph

// ModalityMapping describes how a criminal_cases.case_type maps onto Neo4j
// labels and type values. It is the single place that connects the relational
// modality to the graph schema (see migrations/001_init.cypher,
// migrations/002_forensic_evidence_capture.cypher, and
// migrations/003_biometric_cluster.cypher).
type ModalityMapping struct {
	// EvidenceType is the value stored on Object:Evidence.evidenceType.
	EvidenceType string
	// FeatureLabel is the BiometricFeature sub-label (e.g. "FingerprintLift").
	FeatureLabel string
	// FeatureType is the value stored on BiometricFeature.featureType.
	FeatureType string
	// ClusterLabel is the BiometricCluster sub-label (e.g. "Fingerprint").
	ClusterLabel string
}

// Modalities maps each supported criminal_cases.case_type to its graph
// representation.
var Modalities = map[string]ModalityMapping{
	"FACIAL": {
		EvidenceType: "FACE_CAPTURE",
		FeatureLabel: "FaceCapture",
		FeatureType:  "FACE_CAPTURE",
		ClusterLabel: "Face",
	},
	"FINGERPRINT": {
		EvidenceType: "LATENT_FINGERPRINT",
		FeatureLabel: "FingerprintLift",
		FeatureType:  "FINGERPRINT_LIFT",
		ClusterLabel: "Fingerprint",
	},
}

// FeatureID returns the graph node id for an evidence item's biometric feature.
func FeatureID(evidenceID string) string {
	return evidenceID + "#feature"
}

// DecisionID returns the graph node id for the decision that compared two
// evidence items.
func DecisionID(a, b string) string {
	return a + "#" + b
}
