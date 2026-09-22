// Package cases provides the core domain types for biometric cases — the
// evidence records organization import commands write into the biometric_cases
// table — and the generic case-ingestion extension contract (Ingest). It has
// no knowledge of any organization's source format and no HTTP concerns.
package cases

// Case is one biometric-case (evidence) record as stored in biometric_cases.
// CaseType is the case's legal nature (CRIMINAL or CIVIL -- e.g. disaster victim
// identification or an unidentified body); Modality is the biometric it works
// with (FACIAL or FINGERPRINT).
type Case struct {
	CaseID      string `json:"caseId"`
	CaseType    string `json:"caseType"`
	Modality    string `json:"modality"`
	Description string `json:"description"`
}

// Case types (biometric_cases.case_type).
const (
	CaseTypeCriminal = "CRIMINAL"
	CaseTypeCivil    = "CIVIL"
)

// validCaseType reports whether caseType is one of the biometric_cases.case_type values.
func validCaseType(caseType string) bool {
	return caseType == CaseTypeCriminal || caseType == CaseTypeCivil
}
