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

// FileInput is one already-uploaded file to attach to an EvidenceInput or TraceInput, stored as
// its own case_files row (see MODEL.md section 2.2) -- the evidence image itself, or a trace's
// own face_crop. Like identity.FileInput, Ingest works from a StorageRef a caller already wrote
// with trackid's storage client, not raw bytes: this package stays a DB-only transaction.
//
// HashID is required: it's part of case_files' upsert key (criminal_case_id, category,
// hash_id), so a caller that generates its own images (e.g. trackid-sim's media.Generator)
// supplies its own content hash rather than Ingest computing one from bytes it never sees.
type FileInput struct {
	StorageRef  string
	ContentType string
	SizeBytes   int64
	HashID      string
	Filename    string // optional
}

// Box is one trace's location within its evidence image, in the image's pixel coordinates
// (e.g. cases.DetectFaces' FaceProposal). DetectionScore of exactly 0 is stored as NULL, the
// same simplification EvidenceInput.Description makes for "": no detector kept here scores a
// real detection at exactly 0.
type Box struct {
	X1, Y1, X2, Y2 float64
	DetectionScore float64
}

// TraceInput is one trace found within an Evidence item (e.g. one fingerprint lift on a card,
// one detected face in a photo). TraceType determines the QUESTIONED biometricfeature Ingest
// upserts for it, via graph.FeatureTypeForTraceType -- a trace type with no mapping (ok=false)
// still gets its case_traces row, just no feature. Box and Crop are optional and independent:
// a trace marked by hand may have neither, a detector-found trace usually has Box, and Crop is
// for a pipeline that already produced the face crop itself.
type TraceInput struct {
	Sequence      int16
	TraceType     string
	Box           *Box
	Crop          *FileInput // category "face_crop"
	Codifications []CodificationInput
}

// EvidenceInput is one evidence item (a lift card, a photo, ...) within a case, and every trace
// found on it. File is optional (category "evidence"): organizations whose source has no
// attachable image, e.g. the ones behind trace metadata alone, leave it nil.
type EvidenceInput struct {
	Sequence    int16
	Description string
	File        *FileInput
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
		evidenceFileID, err := upsertCaseFile(ctx, q, upserted.ID, "evidence", evidence.File)
		if err != nil {
			return Result{}, fmt.Errorf("cases: evidence file (case %s, evidence %d): %w", c.CaseID, evidence.Sequence, err)
		}

		evidenceID, err := q.UpsertCaseEvidence(ctx, db.UpsertCaseEvidenceParams{
			CriminalCaseID: upserted.ID,
			Sequence:       evidence.Sequence,
			CaseFileID:     evidenceFileID,
			Description:    nullString(evidence.Description),
		})
		if err != nil {
			return Result{}, fmt.Errorf("cases: upsert case_evidences (case %s, evidence %d): %w", c.CaseID, evidence.Sequence, err)
		}
		evidenceResult := EvidenceResult{Sequence: evidence.Sequence, EvidenceID: evidenceID}

		for _, trace := range evidence.Traces {
			cropFileID, err := upsertCaseFile(ctx, q, upserted.ID, "face_crop", trace.Crop)
			if err != nil {
				return Result{}, fmt.Errorf("cases: trace crop (case %s, evidence %d, trace %d): %w", c.CaseID, evidence.Sequence, trace.Sequence, err)
			}

			traceParams := db.UpsertCaseTraceParams{
				EvidenceID: evidenceID,
				Sequence:   trace.Sequence,
				TraceType:  trace.TraceType,
				CaseFileID: cropFileID,
			}
			if trace.Box != nil {
				traceParams.BoxX1 = sql.NullFloat64{Float64: trace.Box.X1, Valid: true}
				traceParams.BoxY1 = sql.NullFloat64{Float64: trace.Box.Y1, Valid: true}
				traceParams.BoxX2 = sql.NullFloat64{Float64: trace.Box.X2, Valid: true}
				traceParams.BoxY2 = sql.NullFloat64{Float64: trace.Box.Y2, Valid: true}
				traceParams.DetectionScore = sql.NullFloat64{Float64: trace.Box.DetectionScore, Valid: trace.Box.DetectionScore != 0}
			}
			traceID, err := q.UpsertCaseTrace(ctx, traceParams)
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

// upsertCaseFile writes f as a case_files row under category (nil f is a no-op, returning an
// invalid id) and returns the id to hang off case_evidences.case_file_id or
// case_traces.case_file_id. mediaType is best-effort: an unrecognized ContentType (something
// other than classifyContentType's pdf/image set) still stores the file, just with no
// media_type.
func upsertCaseFile(ctx context.Context, q *db.Queries, criminalCaseID int64, category string, f *FileInput) (sql.NullInt64, error) {
	if f == nil {
		return sql.NullInt64{}, nil
	}
	if f.HashID == "" {
		return sql.NullInt64{}, fmt.Errorf("HashID is required")
	}
	mediaType, _ := classifyContentType(f.ContentType)
	row, err := q.UpsertCaseFile(ctx, db.UpsertCaseFileParams{
		CriminalCaseID: criminalCaseID,
		Category:       category,
		MediaType:      nullString(mediaType),
		HashID:         sql.NullString{String: f.HashID, Valid: true},
		Filename:       nullString(f.Filename),
		StorageRef:     nullString(f.StorageRef),
		ContentType:    nullString(f.ContentType),
		SizeBytes:      sql.NullInt64{Int64: f.SizeBytes, Valid: f.SizeBytes != 0},
	})
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("upsert case_files: %w", err)
	}
	return sql.NullInt64{Int64: row.ID, Valid: true}, nil
}
