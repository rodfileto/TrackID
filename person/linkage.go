package person

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// identityChain loads a person's full identity-chain rows once, by
// person.id (not person_id) -- the shared source GetIdentity, GetProfile,
// and the cluster/case resolution below all build from.
func identityChain(ctx context.Context, sqlDB *sql.DB, personRowID int64) ([]db.ListIdentityChainByPersonRow, error) {
	return db.New(sqlDB).ListIdentityChainByPerson(ctx, sql.NullInt64{Int64: personRowID, Valid: true})
}

// knownFeatureIDsFromChain extracts the graph feature ids (graph.KnownFeatureID)
// for every KNOWN biometricfeature in an already-loaded identity chain -- the
// set cluster/case resolution starts from. A file with no biometricfeature
// row (e.g. a "pdf") contributes nothing, since it was never fed into
// clustering.
func knownFeatureIDsFromChain(rows []db.ListIdentityChainByPersonRow) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, r := range rows {
		if !r.BiometricfeatureID.Valid {
			continue
		}
		id := graph.KnownFeatureID(r.IdentityFileID.Int64)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// loadPersonAndClusterIDs loads one person's row, their identity chain, and
// the clusters (cluster.Run) their enrolled features have resolved into.
// clusterIDs is nil, with no error, when the person has no enrolled features
// yet or none have been clustered. Returns ErrNotFound if no person row
// matches personID. Shared by ListClusters, ListCases, and GetProfile so a
// combined profile fetch pays for this resolution once, not three times.
func loadPersonAndClusterIDs(ctx context.Context, sqlDB *sql.DB, personID string) (db.Person, []db.ListIdentityChainByPersonRow, []int64, error) {
	q := db.New(sqlDB)

	personRow, err := q.GetPersonByPersonID(ctx, personID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.Person{}, nil, nil, ErrNotFound
		}
		return db.Person{}, nil, nil, err
	}

	chainRows, err := identityChain(ctx, sqlDB, personRow.ID)
	if err != nil {
		return db.Person{}, nil, nil, err
	}

	featureIDs := knownFeatureIDsFromChain(chainRows)
	if len(featureIDs) == 0 {
		return personRow, chainRows, nil, nil
	}

	clusterIDs, err := q.ListClusterIDsForFeatureIDs(ctx, featureIDs)
	if err != nil {
		return db.Person{}, nil, nil, err
	}
	return personRow, chainRows, clusterIDs, nil
}
