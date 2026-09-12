package cluster

import (
	"context"
	"database/sql"
)

// decisionRole identifies who — or what — recorded one biometric_decisions row.
type decisionRole string

const (
	roleSystem        decisionRole = "SYSTEM"
	roleVerificator   decisionRole = "VERIFICATOR"
	roleReviewer      decisionRole = "REVIEWER"
	roleInconsistence decisionRole = "INCONSISTENCE"
)

// decision is the call one biometric_decisions row makes.
type decision string

const (
	decisionPositive     decision = "POSITIVE"
	decisionNegative     decision = "NEGATIVE"
	decisionInconclusive decision = "INCONCLUSIVE"
)

// Edge statuses derived from a pair's decision chain.
const (
	edgeStatusPendingReview = "PENDING_REVIEW"
	edgeStatusConfirmed     = "CONFIRMED"
	edgeStatusDisputed      = "DISPUTED"
	edgeStatusRejected      = "REJECTED"
)

// chainEntry is the minimal shape of one biometric_decisions row DeriveEdgeStatus
// needs (role + decision).
type chainEntry struct {
	role     decisionRole
	decision decision
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
func DeriveEdgeStatus(chain []chainEntry) (status string, ok bool) {
	var system, verificator, reviewer, inconsistence *decision
	for i := range chain {
		switch chain[i].role {
		case roleSystem:
			system = &chain[i].decision
		case roleVerificator:
			verificator = &chain[i].decision
		case roleReviewer:
			reviewer = &chain[i].decision
		case roleInconsistence:
			inconsistence = &chain[i].decision
		}
	}

	if inconsistence != nil {
		return decisionOutcome(*inconsistence, edgeStatusDisputed), true
	}
	if verificator != nil && reviewer != nil {
		if *verificator == *reviewer {
			return decisionOutcome(*verificator, edgeStatusPendingReview), true
		}
		return edgeStatusDisputed, true
	}
	if verificator != nil {
		return edgeStatusPendingReview, true
	}
	if system != nil {
		return decisionOutcome(*system, edgeStatusPendingReview), true
	}
	return "", false
}

func decisionOutcome(d decision, inconclusive string) string {
	switch d {
	case decisionPositive:
		return edgeStatusConfirmed
	case decisionNegative:
		return edgeStatusRejected
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
		chain := make([]chainEntry, len(rowsForPair))
		for i, r := range rowsForPair {
			chain[i] = chainEntry{role: decisionRole(r.role), decision: decision(r.decision)}
		}
		status, ok := DeriveEdgeStatus(chain)
		if !ok || status != edgeStatusConfirmed {
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
