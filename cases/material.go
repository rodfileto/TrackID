package cases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Trace roles (case_traces.trace_role): QUESTIONED is material to identify; REFERENCE is known
// material sent in for comparison, e.g. a suspect's photo, never an unidentified face of the case.
const (
	RoleQuestioned = "QUESTIONED"
	RoleReference  = "REFERENCE"
)

// ErrRoleLocked is SetTraceRole's refusal once examiners have decided on the trace.
var ErrRoleLocked = errors.New("cases: the trace's role cannot change once examiners have decided on it")

// MaterialTrace is one trace of a case as the material panel and the reports show it: where it is,
// its role, and the examiner's value determination of it (questioned traces only).
type MaterialTrace struct {
	TraceID          int64         `json:"traceId"`
	FeatureID        string        `json:"featureId"`
	Label            string        `json:"label"` // "<evidence>.<trace>", e.g. "2.1"
	EvidenceFileID   int64         `json:"evidenceFileId,omitempty"`
	CropFileID       int64         `json:"cropFileId,omitempty"`
	Box              *ThumbnailBox `json:"box,omitempty"`
	Role             string        `json:"role"`
	Suitability      string        `json:"suitability,omitempty"`
	SuitabilityNotes string        `json:"suitabilityNotes,omitempty"`
	SuitabilityBy    string        `json:"suitabilityBy,omitempty"`
	SuitabilityAt    *time.Time    `json:"suitabilityAt,omitempty"`
	// Source is the video the trace's evidence frame was taken from, when it was (video package).
	Source *VideoSource `json:"source,omitempty"`
}

// VideoSource is where in a video an evidence frame was taken.
type VideoSource struct {
	Video       string `json:"video"`
	SHA256      string `json:"sha256"`
	TimestampMs int64  `json:"timestampMs"`
	Frame       *int64 `json:"frame,omitempty"`
}

// ListMaterial returns every trace of a case, in evidence and trace order.
func ListMaterial(ctx context.Context, sqlDB *sql.DB, caseID string) ([]MaterialTrace, error) {
	return ListMaterialWith(ctx, db.New(sqlDB), caseID)
}

// ListMaterialWith is ListMaterial on q, e.g. inside a transaction.
func ListMaterialWith(ctx context.Context, q *db.Queries, caseID string) ([]MaterialTrace, error) {
	if _, err := q.GetCaseByCaseID(ctx, caseID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := q.ListCaseTraceMaterial(ctx, caseID)
	if err != nil {
		return nil, err
	}
	out := make([]MaterialTrace, len(rows))
	for i, r := range rows {
		m := MaterialTrace{TraceID: r.ID, FeatureID: graph.QuestionedFeatureID(r.ID), Label: fmt.Sprintf("%d.%d", r.EvidenceSequence, r.Sequence),
			EvidenceFileID: r.CaseFileID.Int64, CropFileID: r.CropFileID.Int64, Role: r.TraceRole, Suitability: r.Suitability.String,
			SuitabilityNotes: r.SuitabilityNotes.String, SuitabilityBy: r.SuitabilityBy.String}
		if r.BoxX1.Valid {
			m.Box = &ThumbnailBox{X1: r.BoxX1.Float64, Y1: r.BoxY1.Float64, X2: r.BoxX2.Float64, Y2: r.BoxY2.Float64}
		}
		if r.SuitabilityAt.Valid {
			m.SuitabilityAt = &r.SuitabilityAt.Time
		}
		if r.SourceTimestampMs.Valid {
			m.Source = &VideoSource{Video: r.SourceVideo.String, SHA256: r.SourceVideoSha256.String, TimestampMs: r.SourceTimestampMs.Int64}
			if r.SourceFrame.Valid {
				f := r.SourceFrame.Int64
				m.Source.Frame = &f
			}
		}
		out[i] = m
	}
	return out, nil
}

// SetTraceRole marks a trace of the case as QUESTIONED or REFERENCE. It cannot change once a human
// decision touches the trace: those decisions were made on it in its role. A reference loses any
// suitability assessment.
func SetTraceRole(ctx context.Context, sqlDB *sql.DB, caseID string, traceID int64, role string) error {
	if role != RoleQuestioned && role != RoleReference {
		return ValidationError("role must be QUESTIONED or REFERENCE")
	}
	q := db.New(sqlDB)
	c, err := q.GetCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	tr, err := q.GetCaseTraceOfCase(ctx, db.GetCaseTraceOfCaseParams{ID: traceID, BiometricCaseID: c.ID})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if tr.TraceRole == role {
		return nil
	}
	n, err := q.CountHumanDecisionsOnFeature(ctx, graph.QuestionedFeatureID(traceID))
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrRoleLocked
	}
	return q.SetCaseTraceRole(ctx, db.SetCaseTraceRoleParams{ID: traceID, TraceRole: role})
}
