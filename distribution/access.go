package distribution

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
)

// ErrNotAssigned is CanOpen's refusal.
var ErrNotAssigned = errors.New("case is not assigned to you")

// CanOpen reports whether user may see and work on a case: an ADMIN or SUPER_ADMIN may open any
// case, anyone else only a case one of whose analyses (open or closed) is currently assigned to
// them. Deciding is narrower: see analyses.CanDecide. It returns nil, analyses.ErrCaseNotFound or
// ErrNotAssigned.
func CanOpen(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, caseID string) error {
	row, err := db.New(sqlDB).GetCaseAccess(ctx, db.GetCaseAccessParams{UserID: user.ID, CaseID: caseID})
	if errors.Is(err, sql.ErrNoRows) {
		return analyses.ErrCaseNotFound
	}
	if err != nil {
		return err
	}
	if user.AccountRole.IsAdmin() || row.HoldsAnalysis {
		return nil
	}
	return ErrNotAssigned
}

// BlindIn reports whether userID works one of the case's open analyses blind right now
// (analyses.BlindWork). Until they submit, the case shows them only that work: anything else on
// the case -- its analyses and examiners, its clusters, its decisions -- could tell them what
// others concluded.
func BlindIn(ctx context.Context, sqlDB *sql.DB, userID int64, caseID string) (bool, error) {
	mine, err := AnalysesOf(ctx, sqlDB, userID)
	if err != nil {
		return false, err
	}
	for _, a := range mine {
		if a.CaseID == caseID && a.Blind {
			return true, nil
		}
	}
	return false, nil
}
