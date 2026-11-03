package examination

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
)

// Suitability is an examiner's value determination of a questioned trace (case_traces.suitability,
// FISWG's analysis phase): whether its image supports a forensic comparison, only an investigative
// lead, or nothing. It is what lets a positive lead stand on an image a comparison could not
// conclude on.
type Suitability string

const (
	Suitable   Suitability = "SUITABLE"
	LeadOnly   Suitability = "LEAD_ONLY"
	Unsuitable Suitability = "UNSUITABLE"
)

// Errors returned to callers.
var (
	ErrInvalidSuitability = errors.New("suitability must be SUITABLE, LEAD_ONLY or UNSUITABLE")
	ErrNotQuestioned      = errors.New("only a questioned trace of the case is assessed")
	ErrCannotAssess       = analyses.ErrCannotDecide
)

// AssessTrace records the suitability of one questioned trace of the analysis's case. Only the
// analysis's first examiner, working it now (analyses.CanWork in the VERIFICATOR step), may: the
// blind steps see the images and nothing else, and an assessment is part of the examination, not of
// its verification.
func AssessTrace(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, analysisID, traceID int64, s Suitability, notes string) error {
	if s != Suitable && s != LeadOnly && s != Unsuitable {
		return ErrInvalidSuitability
	}
	q := db.New(sqlDB)
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return err
	}
	if step, err := analyses.CanWork(user, t); err != nil || step != analyses.StepVerificator {
		return ErrCannotAssess
	}
	tr, err := q.GetCaseTraceOfCase(ctx, db.GetCaseTraceOfCaseParams{ID: traceID, BiometricCaseID: t.BiometricCaseID})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && tr.TraceRole != "QUESTIONED") {
		return ErrNotQuestioned
	}
	if err != nil {
		return err
	}
	return q.SetCaseTraceSuitability(ctx, db.SetCaseTraceSuitabilityParams{ID: traceID, Suitability: sql.NullString{String: string(s), Valid: true},
		SuitabilityNotes: sql.NullString{String: notes, Valid: notes != ""}, SuitabilityBy: sql.NullInt64{Int64: user.ID, Valid: true}})
}
