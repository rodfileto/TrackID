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

// CaseRights is what a user may do on one case (Rights).
type CaseRights struct {
	Read  bool // see the case: its page, evidence, analyses, clusters
	Write bool // add or change its evidence, traces and points
	Holds bool // they hold one of its analyses; reading without holding one is logged (access_log)
	Blind bool // they work one of its analyses blind right now (BlindIn): only that workspace is open
}

// Rights decides what user may do on a case (docs/access-profiles-plan.md). Holding one of its
// analyses (any step, open or closed) gives read and write. Intelligence (auth.IntelligenceAccess)
// and coordination (auth.Coordinates) give read on every case -- but not while the user works any
// analysis blind (BlindAnywhere), since another case could tell them who is on the one they
// verify. The account role gives nothing. It returns analyses.ErrCaseNotFound for an unknown case.
func Rights(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, caseID string) (CaseRights, error) {
	row, err := db.New(sqlDB).GetCaseAccess(ctx, db.GetCaseAccessParams{UserID: user.ID, CaseID: caseID})
	if errors.Is(err, sql.ErrNoRows) {
		return CaseRights{}, analyses.ErrCaseNotFound
	}
	if err != nil {
		return CaseRights{}, err
	}
	mine, err := AnalysesOf(ctx, sqlDB, user.ID)
	if err != nil {
		return CaseRights{}, err
	}
	var r CaseRights
	blindAnywhere := false
	for _, a := range mine {
		blindAnywhere = blindAnywhere || a.Blind
		r.Blind = r.Blind || (a.Blind && a.CaseID == caseID)
	}
	holds := row.HoldsAnalysis && user.IsActive
	r.Read = holds || (!blindAnywhere && (user.IntelligenceAccess() || user.Coordinates()))
	r.Write, r.Holds = holds, holds
	return r, nil
}

// CanOpen reports whether user may read a case (Rights): nil, analyses.ErrCaseNotFound or
// ErrNotAssigned. Deciding is narrower: see analyses.CanWork.
func CanOpen(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, caseID string) error {
	r, err := Rights(ctx, sqlDB, user, caseID)
	if err != nil {
		return err
	}
	if !r.Read {
		return ErrNotAssigned
	}
	return nil
}

// SeesEveryCase reports whether user may list every case: an analyst or a coordinator who is not
// working anything blind. Anyone else lists the cases where they hold an analysis.
func SeesEveryCase(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile) (bool, error) {
	if !user.IntelligenceAccess() && !user.Coordinates() {
		return false, nil
	}
	blind, err := BlindAnywhere(ctx, sqlDB, user.ID)
	return !blind, err
}

// BlindAnywhere reports whether userID works any open analysis blind right now. It suspends their
// intelligence and coordination rights until they submit: either could show them who is on the
// pairs, or what others concluded.
func BlindAnywhere(ctx context.Context, sqlDB *sql.DB, userID int64) (bool, error) {
	mine, err := AnalysesOf(ctx, sqlDB, userID)
	if err != nil {
		return false, err
	}
	for _, a := range mine {
		if a.Blind {
			return true, nil
		}
	}
	return false, nil
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
