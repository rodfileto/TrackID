// Ingest is the generic case-evidence-ingestion extension contract, the QUESTIONED-side
// counterpart to identity.Ingest's KNOWN-side. Organizations normalize their own case-management
// source (a CSV export, a case-management API, ...) into a CaseInput and call Ingest, which
// upserts the full criminal_cases -> case_evidences -> case_traces -> case_codifications chain,
// plus each trace's QUESTIONED biometricfeature, in one transaction. See MODEL.md section 2.2.
// This package has no knowledge of any organization's source format or case-identifier
// vocabulary.
package cases

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// CodificationInput is one processed encoding of a trace (e.g. a minutiae set, an embedding
// placeholder). The pairwise comparison outcome belongs in biometric_decisions, not here.
type CodificationInput struct {
	Sequence         int16
	CodificationType string
}

// TraceInput is one trace found within an Evidence item (e.g. one fingerprint lift on a card,
// one detected face in a photo). TraceType determines the QUESTIONED biometricfeature Ingest
// upserts for it, via graph.FeatureTypeForTraceType -- a trace type with no mapping (ok=false)
// still gets its case_traces row, just no feature.
type TraceInput struct {
	Sequence      int16
	TraceType     string
	Codifications []CodificationInput
}

// EvidenceInput is one evidence item (a lift card, a photo, ...) within a case, and every trace
// found on it.
type EvidenceInput struct {
	Sequence    int16
	Description string
	Traces      []TraceInput
}

// CaseInput is one normalized criminal case. Organizations translate their own case-management
// source into this shape and call Ingest; this package never sees the source's own identifiers
// or vocabulary (e.g. an organization-specific match-type code), only the generic shape.
type CaseInput struct {
	CaseID      string
	CaseType    string
	Description string
	Evidences   []EvidenceInput
}

// CodificationResult is one case_codifications row Ingest wrote.
type CodificationResult struct {
	Sequence       int16
	CodificationID int64
}

// TraceResult is one case_traces row Ingest wrote, and the QUESTIONED biometricfeature it
// produced, if TraceInput.TraceType mapped to one.
type TraceResult struct {
	Sequence      int16
	TraceID       int64
	FeatureID     int64 // 0 when TraceType has no QUESTIONED feature mapping (graph.FeatureTypeForTraceType)
	Codifications []CodificationResult
}

// EvidenceResult is one case_evidences row Ingest wrote, and every trace found on it.
type EvidenceResult struct {
	Sequence   int16
	EvidenceID int64
	Traces     []TraceResult
}

// Result is the set of rows Ingest wrote for one CaseInput.
type Result struct {
	CriminalCaseID int64
	// Inserted is false when CaseID already existed and was updated instead.
	Inserted  bool
	Evidences []EvidenceResult
}

// Ingest upserts one CaseInput's full chain -- criminal_cases, case_evidences, case_traces,
// case_codifications, and each trace's QUESTIONED biometricfeature -- in a single transaction.
func Ingest(ctx context.Context, sqlDB *sql.DB, c CaseInput) (Result, error) {
	if sqlDB == nil {
		return Result{}, fmt.Errorf("cases: nil db")
	}
	if c.CaseID == "" {
		return Result{}, fmt.Errorf("cases: CaseID is required")
	}
	if c.CaseType == "" {
		return Result{}, fmt.Errorf("cases: CaseType is required")
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()

	q := db.New(tx)

	upserted, err := q.UpsertCriminalCase(ctx, db.UpsertCriminalCaseParams{
		CaseID:      c.CaseID,
		CaseType:    c.CaseType,
		Description: c.Description,
	})
	if err != nil {
		return Result{}, fmt.Errorf("cases: upsert criminal_cases: %w", err)
	}

	result := Result{CriminalCaseID: upserted.ID, Inserted: upserted.Inserted}

	for _, evidence := range c.Evidences {
		evidenceID, err := q.UpsertCaseEvidence(ctx, db.UpsertCaseEvidenceParams{
			CriminalCaseID: upserted.ID,
			Sequence:       evidence.Sequence,
			Description:    nullString(evidence.Description),
		})
		if err != nil {
			return Result{}, fmt.Errorf("cases: upsert case_evidences (case %s, evidence %d): %w", c.CaseID, evidence.Sequence, err)
		}
		evidenceResult := EvidenceResult{Sequence: evidence.Sequence, EvidenceID: evidenceID}

		for _, trace := range evidence.Traces {
			traceID, err := q.UpsertCaseTrace(ctx, db.UpsertCaseTraceParams{
				EvidenceID: evidenceID,
				Sequence:   trace.Sequence,
				TraceType:  trace.TraceType,
			})
			if err != nil {
				return Result{}, fmt.Errorf("cases: upsert case_traces (case %s, evidence %d, trace %d): %w", c.CaseID, evidence.Sequence, trace.Sequence, err)
			}
			traceResult := TraceResult{Sequence: trace.Sequence, TraceID: traceID}

			if featureType, ok := graph.FeatureTypeForTraceType(trace.TraceType); ok {
				featureID, err := q.UpsertBiometricFeatureFromCaseTrace(ctx, db.UpsertBiometricFeatureFromCaseTraceParams{
					FeatureType: featureType,
					Provenance:  "QUESTIONED",
					CaseTraceID: sql.NullInt64{Int64: traceID, Valid: true},
				})
				if err != nil {
					return Result{}, fmt.Errorf("cases: upsert biometricfeature (case %s, evidence %d, trace %d): %w", c.CaseID, evidence.Sequence, trace.Sequence, err)
				}
				traceResult.FeatureID = featureID
			}

			for _, codification := range trace.Codifications {
				codificationID, err := q.UpsertCaseCodification(ctx, db.UpsertCaseCodificationParams{
					TraceID:          traceID,
					Sequence:         codification.Sequence,
					CodificationType: codification.CodificationType,
				})
				if err != nil {
					return Result{}, fmt.Errorf("cases: upsert case_codifications (case %s, evidence %d, trace %d, codification %d): %w",
						c.CaseID, evidence.Sequence, trace.Sequence, codification.Sequence, err)
				}
				traceResult.Codifications = append(traceResult.Codifications, CodificationResult{Sequence: codification.Sequence, CodificationID: codificationID})
			}

			evidenceResult.Traces = append(evidenceResult.Traces, traceResult)
		}

		result.Evidences = append(result.Evidences, evidenceResult)
	}

	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
