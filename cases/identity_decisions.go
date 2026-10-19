package cases

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Identity decision statuses, as the case page reads them. Unlike the clusters
// (ListCaseClusters), which show only links people confirmed, this is the
// case's own decision record on every enrolled identity its faces reached at
// the review threshold, whatever its state.
const (
	// IdentityPending: the matcher's candidate (at or above the review
	// threshold, a SYSTEM POSITIVE included) that examiners have not settled.
	IdentityPending = "PENDING"
	// IdentityConfirmed: examiners confirmed it (cluster.Reviewed).
	IdentityConfirmed = "CONFIRMED"
	// IdentityExcluded: examiners settled it NEGATIVE.
	IdentityExcluded = "EXCLUDED"
	// IdentityInconclusive: examiners settled it INCONCLUSIVE (both agreeing,
	// or the inconsistence step).
	IdentityInconclusive = "INCONCLUSIVE"
	// IdentityDisputed: the two examiners disagreed and the inconsistence step
	// has not decided yet.
	IdentityDisputed = "DISPUTED"
)

// CaseFaceIdentities is one of a case's faces and the enrolled identities
// decided for it (IdentityDecision), best score first.
type CaseFaceIdentities struct {
	TraceID    int64              `json:"traceId"`
	Identities []IdentityDecision `json:"identities"`
}

// IdentityDecision is one enrolled identity a face of the case reached: a
// candidate the matcher wrote at or above the review threshold (it writes
// nothing below it), or a pair examiners decided in a comparison. Status is
// one of the Identity* constants; DecisionRole/DecidedBy name who settled it
// (empty while pending); Score is the matcher's, when it scored the pair.
type IdentityDecision struct {
	PersonID       string   `json:"personId"`
	Name           string   `json:"name"`
	IdentityFileID int64    `json:"identityFileId"`
	ContentType    string   `json:"contentType,omitempty"`
	FeatureID      string   `json:"featureId"`
	Score          *float64 `json:"score,omitempty"`
	Status         string   `json:"status"`
	DecisionRole   string   `json:"decisionRole,omitempty"`
	DecidedBy      string   `json:"decidedBy,omitempty"`
}

// ListCaseIdentityDecisions returns, for each codified face of a case, every
// enrolled identity decided for it (see IdentityDecision), in the order the
// faces appear in ListCaseCodifications. Every face is listed, an empty
// Identities meaning no identity reached the review threshold for it.
// Other cases' faces are left out: they belong to the clusters, not to this
// case's decisions on identities.
func ListCaseIdentityDecisions(ctx context.Context, sqlDB *sql.DB, caseID string) ([]CaseFaceIdentities, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}
	codifications, err := ListCaseCodifications(ctx, sqlDB, caseID)
	if err != nil {
		return nil, err
	}
	var traceIDs []int64
	var featureIDs []string
	seen := map[int64]bool{}
	for _, c := range codifications {
		if !seen[c.TraceID] {
			seen[c.TraceID] = true
			traceIDs = append(traceIDs, c.TraceID)
			featureIDs = append(featureIDs, graph.QuestionedFeatureID(c.TraceID))
		}
	}
	if len(traceIDs) == 0 {
		return []CaseFaceIdentities{}, nil
	}

	q := db.New(sqlDB)
	rows, err := q.ListBiometricDecisionsForFeatures(ctx, featureIDs)
	if err != nil {
		return nil, err
	}
	// chains[own][known] is the chain of one face's pair with one enrolled identity.
	chains := map[string]map[string][]db.ListBiometricDecisionsForFeaturesRow{}
	var knownIDs []string
	for _, r := range rows {
		if _, questioned := graph.ParseQuestionedFeatureID(r.CounterpartID); questioned {
			continue
		}
		if chains[r.FeatureID] == nil {
			chains[r.FeatureID] = map[string][]db.ListBiometricDecisionsForFeaturesRow{}
		}
		chains[r.FeatureID][r.CounterpartID] = append(chains[r.FeatureID][r.CounterpartID], r)
		knownIDs = append(knownIDs, r.CounterpartID)
	}
	described, err := DescribeFeatures(ctx, q, knownIDs)
	if err != nil {
		return nil, err
	}

	out := make([]CaseFaceIdentities, 0, len(traceIDs))
	for i, traceID := range traceIDs {
		face := CaseFaceIdentities{TraceID: traceID, Identities: []IdentityDecision{}}
		for known, chain := range chains[featureIDs[i]] {
			person, ok := described[known]
			if !ok {
				continue
			}
			status, ok := identityStatus(chainEntries(chain))
			if !ok {
				continue
			}
			d := IdentityDecision{PersonID: person.PersonID, Name: person.Name, IdentityFileID: person.IdentityFileID,
				ContentType: person.ContentType, FeatureID: known, Status: status}
			var role string
			role, d.DecidedBy, d.Score = reviewedDecision(chain)
			if status == IdentityPending {
				d.DecidedBy = "" // the matcher, or a first examiner whose step is not settled yet
			} else {
				d.DecisionRole = role
			}
			face.Identities = append(face.Identities, d)
		}
		slices.SortFunc(face.Identities, func(a, b IdentityDecision) int {
			if c := cmp.Compare(scoreOf(b), scoreOf(a)); c != 0 {
				return c
			}
			return cmp.Compare(a.FeatureID, b.FeatureID)
		})
		out = append(out, face)
	}
	return out, nil
}

// identityStatus maps one face-identity chain to its Identity* status, from its deciding round
// and who settled it (cluster.DecidingRoundBy). ok is false for a pair below the review
// threshold: settled NEGATIVE by the matcher alone, which it does not write today.
func identityStatus(chain []cluster.ChainEntry) (status string, ok bool) {
	_, edge, settledBy, ok := cluster.DecidingRoundBy(chain)
	if !ok {
		return "", false
	}
	byPeople := settledBy == cluster.RoleReviewer || settledBy == cluster.RoleInconsistence
	switch {
	case edge == cluster.EdgeStatusConfirmed && byPeople:
		return IdentityConfirmed, true
	case edge == cluster.EdgeStatusRejected && byPeople:
		return IdentityExcluded, true
	case edge == cluster.EdgeStatusRejected:
		return "", false
	case edge == cluster.EdgeStatusDisputed && settledBy == cluster.RoleInconsistence:
		return IdentityInconclusive, true
	case edge == cluster.EdgeStatusDisputed:
		return IdentityDisputed, true
	case edge == cluster.EdgeStatusPendingReview && settledBy == cluster.RoleReviewer:
		return IdentityInconclusive, true // both examiners found it inconclusive
	default:
		return IdentityPending, true // the matcher's candidate, a SYSTEM POSITIVE included
	}
}

func scoreOf(d IdentityDecision) float64 {
	if d.Score == nil {
		return -1
	}
	return *d.Score
}
