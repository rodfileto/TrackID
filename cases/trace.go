package cases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/graph"
)

// ErrUnsupportedCaseType is returned by CreateTraces when the case's
// case_type has no trace_type mapping (graph.TraceTypeForCaseType) -- this
// shouldn't happen given criminal_cases.case_type's CHECK constraint, but is
// guarded against rather than assumed.
var ErrUnsupportedCaseType = errors.New("cases: unsupported case type")

// ErrNotImage is returned by CreateTraces when the evidence file is not an
// image (e.g. a pdf).
var ErrNotImage = errors.New("cases: evidence file is not an image")

// TraceDetection is one trace marked on an evidence image -- a bounding box
// in pixel coordinates against the original image, plus a confidence score.
// The same shape covers both modalities: one face in a FACIAL case, one
// fingerprint lift in a FINGERPRINT case. It's also modality-agnostic in how
// the box was produced -- a user marking it by hand, or (FACIAL only, via a
// separate detector integration) an automatic detection tool.
// CaseFileID optionally names a case_files row (category "face_crop") for a
// cropped image of the trace, when the caller already produced one.
type TraceDetection struct {
	BoxX1, BoxY1, BoxX2, BoxY2 float64
	Score                      float64
	CaseFileID                 int64
}

// Trace is one case_traces row CreateTraces wrote, plus the QUESTIONED
// biometricfeature it produced. CreateTraces also writes a placeholder
// case_codifications row (sequence 1) for it, but that id isn't surfaced
// here -- nothing downstream needs it yet.
type Trace struct {
	ID        int64   `json:"id"`
	Sequence  int16   `json:"sequence"`
	TraceType string  `json:"traceType"`
	BoxX1     float64 `json:"boxX1"`
	BoxY1     float64 `json:"boxY1"`
	BoxX2     float64 `json:"boxX2"`
	BoxY2     float64 `json:"boxY2"`
	Score     float64 `json:"score"`
	FeatureID int64   `json:"featureId"`
}

// CreateTraces records one or more marked traces (see TraceDetection) found
// within an evidence file as case_traces rows, each paired with its own
// QUESTIONED biometricfeature and a placeholder case_codifications row
// (graph.TraceTypeForCaseType/FeatureTypeForTraceType/
// CodificationTypeForTraceType pick the right types for the case's modality).
// It is the storage half of trace marking -- how the boxes were produced, by
// hand or by an automatic detector, is a separate concern that calls this
// with its results.
//
// Evidence added through AddEvidence only gets a case_files row; case_traces
// hangs off case_evidences (MODEL.md section 2.2), so the evidence's
// case_evidences row is created on first use here, the same way imported
// evidence already has one. Calling this again for the same evidence file
// appends new traces after whatever is already there -- it does not replace
// or deduplicate prior ones.
//
// Each codification created gets its processing enqueued (see
// enqueueCodificationTasks), the same as SaveCodificationImage does -- so a
// trace marked and Codify'd by hand, with no crop of its own, still gets a
// stored face embedding or fingerprint template, computed from the evidence
// image and the trace's own box. A nil queue (Redis not configured)
// or an enqueue error only logs; nothing here depends on it succeeding.
func CreateTraces(ctx context.Context, sqlDB *sql.DB, queue embedding.Enqueuer, caseID string, evidenceFileID int64, detections []TraceDetection) ([]Trace, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}
	if len(detections) == 0 {
		return nil, nil
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	q := db.New(tx)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	traceType, ok := graph.TraceTypeForCaseType(caseRow.CaseType)
	if !ok {
		return nil, ErrUnsupportedCaseType
	}
	featureType, _ := graph.FeatureTypeForTraceType(traceType)

	fileRow, err := q.GetCaseFile(ctx, db.GetCaseFileParams{ID: evidenceFileID, CriminalCaseID: caseRow.ID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if fileRow.Category != "evidence" {
		return nil, ErrNotFound
	}
	if !strings.HasPrefix(fileRow.ContentType.String, "image/") {
		return nil, ErrNotImage
	}

	evidenceID, err := q.GetCaseEvidenceByCaseFile(ctx, db.GetCaseEvidenceByCaseFileParams{
		CriminalCaseID: caseRow.ID,
		CaseFileID:     sql.NullInt64{Int64: evidenceFileID, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		evidenceID, err = q.CreateCaseEvidenceForFile(ctx, db.CreateCaseEvidenceForFileParams{
			CriminalCaseID: caseRow.ID,
			CaseFileID:     sql.NullInt64{Int64: evidenceFileID, Valid: true},
		})
	}
	if err != nil {
		return nil, fmt.Errorf("cases: resolve case_evidences for file %d: %w", evidenceFileID, err)
	}

	nextSequence, err := q.MaxCaseTraceSequence(ctx, evidenceID)
	if err != nil {
		return nil, fmt.Errorf("cases: max case_traces sequence: %w", err)
	}

	traces := make([]Trace, 0, len(detections))
	var codificationIDs []int64
	codificationType, _ := graph.CodificationTypeForTraceType(traceType)
	for _, detection := range detections {
		nextSequence++

		var caseFileID sql.NullInt64
		if detection.CaseFileID != 0 {
			caseFileID = sql.NullInt64{Int64: detection.CaseFileID, Valid: true}
		}

		traceID, err := q.UpsertCaseTrace(ctx, db.UpsertCaseTraceParams{
			EvidenceID:     evidenceID,
			Sequence:       nextSequence,
			TraceType:      traceType,
			BoxX1:          sql.NullFloat64{Float64: detection.BoxX1, Valid: true},
			BoxY1:          sql.NullFloat64{Float64: detection.BoxY1, Valid: true},
			BoxX2:          sql.NullFloat64{Float64: detection.BoxX2, Valid: true},
			BoxY2:          sql.NullFloat64{Float64: detection.BoxY2, Valid: true},
			DetectionScore: sql.NullFloat64{Float64: detection.Score, Valid: true},
			CaseFileID:     caseFileID,
		})
		if err != nil {
			return nil, fmt.Errorf("cases: create case_traces (evidence %d, sequence %d): %w", evidenceID, nextSequence, err)
		}

		featureID, err := q.UpsertBiometricFeatureFromCaseTrace(ctx, db.UpsertBiometricFeatureFromCaseTraceParams{
			FeatureType: featureType,
			Provenance:  "QUESTIONED",
			CaseTraceID: sql.NullInt64{Int64: traceID, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("cases: create biometricfeature (trace %d): %w", traceID, err)
		}

		if codificationType != "" {
			codificationID, err := q.UpsertCaseCodification(ctx, db.UpsertCaseCodificationParams{
				TraceID:          traceID,
				Sequence:         1,
				CodificationType: codificationType,
			})
			if err != nil {
				return nil, fmt.Errorf("cases: create case_codifications (trace %d): %w", traceID, err)
			}
			codificationIDs = append(codificationIDs, codificationID)
		}

		traces = append(traces, Trace{
			ID:        traceID,
			Sequence:  nextSequence,
			TraceType: traceType,
			BoxX1:     detection.BoxX1,
			BoxY1:     detection.BoxY1,
			BoxX2:     detection.BoxX2,
			BoxY2:     detection.BoxY2,
			Score:     detection.Score,
			FeatureID: featureID,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	enqueueCodificationTasks(queue, codificationType, codificationIDs)

	return traces, nil
}

// ListTraces returns every case_traces row marked on an evidence file, in
// sequence order.
func ListTraces(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID int64) ([]Trace, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rows, err := q.ListCaseTracesByCaseFile(ctx, db.ListCaseTracesByCaseFileParams{
		CriminalCaseID: caseRow.ID,
		CaseFileID:     sql.NullInt64{Int64: evidenceFileID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("cases: list case_traces for file %d: %w", evidenceFileID, err)
	}

	traces := make([]Trace, 0, len(rows))
	for _, row := range rows {
		traces = append(traces, Trace{
			ID:        row.ID,
			Sequence:  row.Sequence,
			TraceType: row.TraceType,
			BoxX1:     row.BoxX1.Float64,
			BoxY1:     row.BoxY1.Float64,
			BoxX2:     row.BoxX2.Float64,
			BoxY2:     row.BoxY2.Float64,
			Score:     row.DetectionScore.Float64,
			FeatureID: row.FeatureID.Int64,
		})
	}
	return traces, nil
}

// DeleteTrace removes one case_traces row marked on an evidence file.
// case_codifications and biometricfeature (and, through it,
// feature_embeddings) cascade off case_traces, so nothing else needs to be
// deleted alongside it.
func DeleteTrace(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID int64) error {
	if sqlDB == nil {
		return fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	deleted, err := q.DeleteCaseTrace(ctx, db.DeleteCaseTraceParams{
		ID:             traceID,
		CriminalCaseID: caseRow.ID,
		CaseFileID:     sql.NullInt64{Int64: evidenceFileID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("cases: delete case_traces %d: %w", traceID, err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}
