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

// PersonTraceDecision is one of a person's enrolled features against one face of a case: the
// person side of ListCaseIdentityDecisions, with the same statuses (IdentityPending for the
// matcher's candidate not yet settled by examiners, IdentityConfirmed, IdentityExcluded,
// IdentityInconclusive, IdentityDisputed) and the matcher's score when it scored the pair.
type PersonTraceDecision struct {
	CaseID         string             `json:"caseId"`
	Modality       string             `json:"modality"`
	TraceID        int64              `json:"traceId"`
	KnownFeatureID string             `json:"knownFeatureId"`
	Score          *float64           `json:"score,omitempty"`
	Status         string             `json:"status"`
	Basis          cluster.LinkBasis  `json:"basis,omitempty"`
	Conclusion     cluster.Conclusion `json:"conclusion,omitempty"`
	Link           *cluster.Link      `json:"link,omitempty"`
}

// ListPersonTraceDecisions returns every decided or candidate pair between the given enrolled
// features (graph.KnownFeatureID) and case faces, whatever its state, best score first: what
// the matcher proposed and examiners have not settled, as well as what they confirmed,
// excluded or left inconclusive. Pairs below the review threshold are not written by the
// matcher and so never appear.
func ListPersonTraceDecisions(ctx context.Context, sqlDB *sql.DB, knownFeatureIDs []string) ([]PersonTraceDecision, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}
	out := []PersonTraceDecision{}
	if len(knownFeatureIDs) == 0 {
		return out, nil
	}
	q := db.New(sqlDB)
	rows, err := q.ListBiometricDecisionsForFeatures(ctx, knownFeatureIDs)
	if err != nil {
		return nil, err
	}
	type pair struct {
		known string
		trace int64
	}
	chains := map[pair][]db.ListBiometricDecisionsForFeaturesRow{}
	var order []pair
	var traceIDs []int64
	for _, r := range rows {
		traceID, questioned := graph.ParseQuestionedFeatureID(r.CounterpartID)
		if !questioned {
			continue
		}
		p := pair{known: r.FeatureID, trace: traceID}
		if _, seen := chains[p]; !seen {
			order = append(order, p)
			traceIDs = append(traceIDs, traceID)
		}
		chains[p] = append(chains[p], r)
	}
	if len(order) == 0 {
		return out, nil
	}
	caseRows, err := q.ListCasesByCaseTraceIDs(ctx, traceIDs)
	if err != nil {
		return nil, err
	}
	caseOf := map[int64]db.ListCasesByCaseTraceIDsRow{}
	for _, c := range caseRows {
		caseOf[c.CaseTraceID] = c
	}
	level, err := cluster.IdentificationLevel(ctx, q)
	if err != nil {
		return nil, err
	}

	for _, p := range order {
		c, ok := caseOf[p.trace]
		if !ok {
			continue
		}
		chain := chains[p]
		entries := chainEntries(chain)
		status, ok := identityStatus(entries)
		if !ok {
			continue
		}
		d := PersonTraceDecision{CaseID: c.CaseID, Modality: c.Modality, TraceID: p.trace, KnownFeatureID: p.known, Status: status}
		_, _, d.Score = reviewedDecision(chain)
		if status != IdentityPending {
			d.Basis, d.Conclusion = roundBasis(entries)
		}
		if l, ok := cluster.ReviewedLink(entries, level); ok {
			d.Link = &l
		}
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b PersonTraceDecision) int {
		sa, sb := -1.0, -1.0
		if a.Score != nil {
			sa = *a.Score
		}
		if b.Score != nil {
			sb = *b.Score
		}
		if c := cmp.Compare(sb, sa); c != 0 {
			return c
		}
		return cmp.Or(cmp.Compare(a.CaseID, b.CaseID), cmp.Compare(a.TraceID, b.TraceID))
	})
	return out, nil
}
