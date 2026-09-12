package cluster

import (
	"context"
	"database/sql"
)

// Role identifies who — or what — recorded one biometric_decisions row.
type Role string

const (
	RoleSystem        Role = "SYSTEM"
	RoleVerificator   Role = "VERIFICATOR"
	RoleReviewer      Role = "REVIEWER"
	RoleInconsistence Role = "INCONSISTENCE"
)

// Decision is the call one biometric_decisions row makes.
type Decision string

const (
	DecisionPositive     Decision = "POSITIVE"
	DecisionNegative     Decision = "NEGATIVE"
	DecisionInconclusive Decision = "INCONCLUSIVE"
)

// EdgeStatus is a pair's derived state, per DeriveEdgeStatus.
type EdgeStatus string

// Edge statuses derived from a pair's decision chain.
const (
	EdgeStatusPendingReview EdgeStatus = "PENDING_REVIEW"
	EdgeStatusConfirmed     EdgeStatus = "CONFIRMED"
	EdgeStatusDisputed      EdgeStatus = "DISPUTED"
	EdgeStatusRejected      EdgeStatus = "REJECTED"
)

// ChainEntry is the minimal shape of one biometric_decisions row DeriveEdgeStatus
// needs (role + decision). Exported so any caller -- an API handler recording an
// analyst decision, an import command seeding a SYSTEM one -- can derive or
// validate a pair's status without duplicating this package's sequencing rules.
type ChainEntry struct {
	Role     Role
	Decision Decision
}

// DeriveEdgeStatus is a pure function mapping a pair's full decision chain to its
// derived state. ok is false only for an empty chain (nothing decided yet).
//
// The rules, in order:
//   - INCONSISTENCE, if present, is final: POSITIVE confirms, NEGATIVE rejects,
//     INCONCLUSIVE leaves the pair DISPUTED.
//   - VERIFICATOR and REVIEWER together settle the pair by agreement: matching
//     decisions confirm/reject (or, both INCONCLUSIVE, stay PENDING_REVIEW);
//     differing decisions are DISPUTED.
//   - VERIFICATOR alone is always PENDING_REVIEW.
//   - SYSTEM alone is CONFIRMED for a POSITIVE score, PENDING_REVIEW for
//     INCONCLUSIVE, or REJECTED for an explicit NEGATIVE.
func DeriveEdgeStatus(chain []ChainEntry) (status EdgeStatus, ok bool) {
	var system, verificator, reviewer, inconsistence *Decision
	for i := range chain {
		switch chain[i].Role {
		case RoleSystem:
			system = &chain[i].Decision
		case RoleVerificator:
			verificator = &chain[i].Decision
		case RoleReviewer:
			reviewer = &chain[i].Decision
		case RoleInconsistence:
			inconsistence = &chain[i].Decision
		}
	}

	if inconsistence != nil {
		return decisionOutcome(*inconsistence, EdgeStatusDisputed), true
	}
	if verificator != nil && reviewer != nil {
		if *verificator == *reviewer {
			return decisionOutcome(*verificator, EdgeStatusPendingReview), true
		}
		return EdgeStatusDisputed, true
	}
	if verificator != nil {
		return EdgeStatusPendingReview, true
	}
	if system != nil {
		return decisionOutcome(*system, EdgeStatusPendingReview), true
	}
	return "", false
}

func decisionOutcome(d Decision, inconclusive EdgeStatus) EdgeStatus {
	switch d {
	case DecisionPositive:
		return EdgeStatusConfirmed
	case DecisionNegative:
		return EdgeStatusRejected
	default:
		return inconclusive
	}
}

// caseTypeForModality maps a biometric_decisions.modality (FACE/FINGERPRINT) onto
// the clusters table's case_type vocabulary (FACIAL/FINGERPRINT).
func caseTypeForModality(modality string) string {
	if modality == "FACE" {
		return "FACIAL"
	}
	return "FINGERPRINT"
}

// decisionRow is one biometric_decisions row.
type decisionRow struct {
	featureA     string
	featureB     string
	modality     string
	role         string
	decision     string
	systemSource sql.NullString
	username     sql.NullString
	confidence   sql.NullFloat64
}

// confirmedPair is a pair of features whose decision chain has settled to
// CONFIRMED. confidence/identifiedBy describe who/what settled it, for the
// IDENTIFIED_AS edge Identify writes.
type confirmedPair struct {
	featureA     string
	featureB     string
	modality     string // "FACE" | "FINGERPRINT"
	confidence   *float64
	identifiedBy string
}

const decisionsQuery = `
SELECT feature_a_id, feature_b_id, modality, role, decision, system_source, username, confidence
FROM biometric_decisions
ORDER BY feature_a_id, feature_b_id, decided_at
`

// loadConfirmedPairs reads every biometric_decisions row, groups them into
// per-pair chains, and returns the pairs whose derived status is CONFIRMED.
func loadConfirmedPairs(ctx context.Context, db *sql.DB) ([]confirmedPair, error) {
	rows, err := db.QueryContext(ctx, decisionsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type key struct{ a, b string }
	chains := map[key][]decisionRow{}
	var order []key
	for rows.Next() {
		var r decisionRow
		if err := rows.Scan(&r.featureA, &r.featureB, &r.modality, &r.role, &r.decision, &r.systemSource, &r.username, &r.confidence); err != nil {
			return nil, err
		}
		k := key{r.featureA, r.featureB}
		if _, ok := chains[k]; !ok {
			order = append(order, k)
		}
		chains[k] = append(chains[k], r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var pairs []confirmedPair
	for _, k := range order {
		rowsForPair := chains[k]
		chain := make([]ChainEntry, len(rowsForPair))
		for i, r := range rowsForPair {
			chain[i] = ChainEntry{Role: Role(r.role), Decision: Decision(r.decision)}
		}
		status, ok := DeriveEdgeStatus(chain)
		if !ok || status != EdgeStatusConfirmed {
			continue
		}
		pairs = append(pairs, confirmedPair{
			featureA:     k.a,
			featureB:     k.b,
			modality:     rowsForPair[0].modality,
			confidence:   chainConfidence(rowsForPair),
			identifiedBy: chainIdentifiedBy(rowsForPair),
		})
	}
	return pairs, nil
}

func chainConfidence(rows []decisionRow) *float64 {
	for _, r := range rows {
		if r.confidence.Valid {
			v := r.confidence.Float64
			return &v
		}
	}
	return nil
}

func chainIdentifiedBy(rows []decisionRow) string {
	var inconsistence, reviewer, verificator, systemSource string
	for _, r := range rows {
		switch r.role {
		case "SYSTEM":
			if r.systemSource.Valid {
				systemSource = r.systemSource.String
			}
		case "VERIFICATOR":
			if r.username.Valid {
				verificator = r.username.String
			}
		case "REVIEWER":
			if r.username.Valid {
				reviewer = r.username.String
			}
		case "INCONSISTENCE":
			if r.username.Valid {
				inconsistence = r.username.String
			}
		}
	}
	switch {
	case inconsistence != "":
		return inconsistence
	case reviewer != "":
		return reviewer
	case verificator != "":
		return verificator
	default:
		return systemSource
	}
}
