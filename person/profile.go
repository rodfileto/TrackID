package person

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rodfileto/trackid/db"
)

// Profile is one person's full intelligence view: their enrollment identity,
// the biometric clusters they've resolved into, and the biometric cases
// linked to them through those clusters. This is the shape a case-detail
// "person profile" panel renders from -- everything in it still traces back
// to a reviewed, CONFIRMED decision chain (see MODEL.md sections 3-4); it
// asserts no link of its own.
type Profile struct {
	Identity Identity      `json:"identity"`
	Clusters []Cluster     `json:"clusters"`
	Cases    []RelatedCase `json:"cases"`
}

// GetProfile loads a person's full Profile in one call: GetIdentity,
// ListClusters, and ListCases combined, sharing their common identity-chain
// and cluster-membership loads instead of repeating them. Returns
// ErrNotFound if no person row matches personID.
func GetProfile(ctx context.Context, sqlDB *sql.DB, personID string) (Profile, error) {
	if sqlDB == nil {
		return Profile{}, fmt.Errorf("person: nil db")
	}
	if personID == "" {
		return Profile{}, fmt.Errorf("person: personID is required")
	}

	personRow, chainRows, clusterIDs, err := loadPersonAndClusterIDs(ctx, sqlDB, personID)
	if err != nil {
		return Profile{}, err
	}

	profile := Profile{Identity: buildIdentity(personID, personRow.Meta, chainRows)}
	if len(clusterIDs) == 0 {
		return profile, nil
	}

	q := db.New(sqlDB)
	clusterRows, err := q.ListClustersByIDs(ctx, clusterIDs)
	if err != nil {
		return Profile{}, err
	}
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return Profile{}, err
	}

	profile.Clusters, err = buildClusters(ctx, q, clusterRows, memberRows)
	if err != nil {
		return Profile{}, err
	}
	profile.Cases, err = buildCases(ctx, q, memberRows)
	if err != nil {
		return Profile{}, err
	}

	return profile, nil
}
