package person

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rodfileto/trackid/db"
)

// Cluster is one biometric cluster a person's enrolled (KNOWN) features have
// resolved into: a group of same-modality biometric samples -- enrollment
// records and/or crime-scene evidence -- that a confirmed decision chain has
// linked together (see cluster.Run). See MODEL.md section 4.
type Cluster struct {
	ClusterID   int64     `json:"clusterId"`
	CaseType    string    `json:"caseType"`
	CreatedAt   time.Time `json:"createdAt"`
	MemberCount int       `json:"memberCount"`
}

// ListClusters returns every biometric cluster resolved to the given
// person -- one entry per cluster a CONFIRMED decision has placed one of the
// person's enrolled features into. A person with documents/registers of more
// than one modality can resolve into several clusters; most resolve into at
// most one per modality. Returns ErrNotFound if no person row matches.
func ListClusters(ctx context.Context, sqlDB *sql.DB, personID string) ([]Cluster, error) {
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
	clusterRows, err := q.ListClustersByIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}

	return buildClusters(clusterRows, memberRows), nil
}

// buildClusters assembles the Cluster list from an already-loaded set of
// cluster rows and their combined membership -- the reusable core both
// ListClusters and GetProfile build on.
func buildClusters(clusterRows []db.ListClustersByIDsRow, memberRows []db.ListClusterMembersByClusterIDsRow) []Cluster {
	counts := make(map[int64]int, len(clusterRows))
	for _, m := range memberRows {
		counts[m.ClusterID]++
	}

	out := make([]Cluster, 0, len(clusterRows))
	for _, c := range clusterRows {
		out = append(out, Cluster{
			ClusterID:   c.ID,
			CaseType:    c.CaseType,
			CreatedAt:   c.CreatedAt,
			MemberCount: counts[c.ID],
		})
	}
	return out
}
