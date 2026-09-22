package person

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// RelatedCase is one biometric case linked to a person through a resolved
// biometric cluster: at least one QUESTIONED trace in the case belongs to a
// cluster one of the person's enrolled (KNOWN) features has been confirmed
// into. ClusterIDs names which of the person's clusters supplied the link --
// a case can carry more than one when it holds traces from several of the
// person's clusters (e.g. both face and fingerprint evidence).
//
// A case here is never inferred from co-occurrence (e.g. many faces in one
// photo) -- only from a CONFIRMED decision chain (cluster.DeriveEdgeStatus),
// the same one cluster.Run already used to place the trace's feature in the
// cluster in the first place.
type RelatedCase struct {
	CaseID      string  `json:"caseId"`
	Modality    string  `json:"modality"`
	Description string  `json:"description"`
	ClusterIDs  []int64 `json:"clusterIds"`
}

// ListCases returns every biometric case linked to the given person through
// their resolved biometric clusters. Returns ErrNotFound if no person row
// matches.
func ListCases(ctx context.Context, sqlDB *sql.DB, personID string) ([]RelatedCase, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("person: nil db")
	}
	if personID == "" {
		return nil, fmt.Errorf("person: personID is required")
	}

	_, _, clusterIDs, err := loadPersonAndClusterIDs(ctx, sqlDB, personID)
	if err != nil {
		return nil, err
	}
	if len(clusterIDs) == 0 {
		return nil, nil
	}

	q := db.New(sqlDB)
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}

	return buildCases(ctx, q, memberRows)
}

// buildCases assembles the RelatedCase list from an already-loaded set of
// cluster members -- the reusable core both ListCases and GetProfile build
// on. A cluster can hold both the person's own KNOWN features and QUESTIONED
// ones from crime-scene evidence; only the latter resolve to a case.
// cluster_members.feature_id is UNIQUE across every cluster, so each
// case_trace_id maps to exactly one cluster.
func buildCases(ctx context.Context, q *db.Queries, memberRows []db.ListClusterMembersByClusterIDsRow) ([]RelatedCase, error) {
	traceCluster := map[int64]int64{}
	var traceIDs []int64
	for _, m := range memberRows {
		traceID, ok := graph.ParseQuestionedFeatureID(m.FeatureID)
		if !ok {
			continue
		}
		traceCluster[traceID] = m.ClusterID
		traceIDs = append(traceIDs, traceID)
	}
	if len(traceIDs) == 0 {
		return nil, nil
	}

	caseRows, err := q.ListCasesByCaseTraceIDs(ctx, traceIDs)
	if err != nil {
		return nil, err
	}

	var order []string
	cases := map[string]*RelatedCase{}
	clusterSeen := map[string]map[int64]struct{}{}
	for _, r := range caseRows {
		c, ok := cases[r.CaseID]
		if !ok {
			c = &RelatedCase{CaseID: r.CaseID, Modality: r.Modality, Description: r.Description}
			cases[r.CaseID] = c
			clusterSeen[r.CaseID] = map[int64]struct{}{}
			order = append(order, r.CaseID)
		}

		clusterID := traceCluster[r.CaseTraceID]
		if _, dup := clusterSeen[r.CaseID][clusterID]; dup {
			continue
		}
		clusterSeen[r.CaseID][clusterID] = struct{}{}
		c.ClusterIDs = append(c.ClusterIDs, clusterID)
	}

	out := make([]RelatedCase, 0, len(order))
	for _, id := range order {
		out = append(out, *cases[id])
	}
	return out, nil
}
