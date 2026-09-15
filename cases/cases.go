// Package cases provides the core domain types for criminal cases — the
// evidence records organization import commands write into the criminal_cases
// table — and the generic case-ingestion extension contract (Ingest). It has
// no knowledge of any organization's source format and no HTTP concerns.
package cases

// Case is one criminal-case (evidence) record as stored in criminal_cases.
type Case struct {
	CaseID      string `json:"caseId"`
	CaseType    string `json:"caseType"`
	Description string `json:"description"`
}
