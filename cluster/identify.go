package cluster

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// identificationRow is one IDENTIFIED_AS edge to write: a cluster resolved to a
// person via a POSITIVE decision linking a QUESTIONED feature to a KNOWN one.
type identificationRow struct {
	clusterID    int64
	personID     string
	confidence   *float64
	identifiedBy string
}

// Identify writes IDENTIFIED_AS edges from clusters to persons, derived from the
// biometric_decisions log: a CONFIRMED decision linking a QUESTIONED feature to
// a KNOWN feature resolves the QUESTIONED feature's cluster to the KNOWN
// feature's person. It reads only from Postgres and rebuilds the edges each run,
// so it is idempotent and consistent with the persisted clusters.
func Identify(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext) (int, error) {
	rows, err := loadIdentificationRows(ctx, sqlDB)
	if err != nil {
		return 0, err
	}

	params := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var confidence any
		if r.confidence != nil {
			confidence = *r.confidence
		}
		params = append(params, map[string]any{
			"clusterId":    fmt.Sprintf("CLUSTER-%d", r.clusterID),
			"personId":     r.personID,
			"confidence":   confidence,
			"identifiedBy": r.identifiedBy,
		})
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		if _, err := tx.Run(ctx, deleteIdentifiedAsQuery, nil); err != nil {
			return nil, err
		}
		if len(params) > 0 {
			_, err := tx.Run(ctx, identifyQuery, map[string]any{"identifications": params})
			return nil, err
		}
		return nil, nil
	}); err != nil {
		return 0, fmt.Errorf("write identifications: %w", err)
	}
	return len(params), nil
}

// IdentifyPlan returns how many identifications Identify would write.
func IdentifyPlan(ctx context.Context, sqlDB *sql.DB) (int, error) {
	rows, err := loadIdentificationRows(ctx, sqlDB)
	return len(rows), err
}

// loadIdentificationRows resolves confirmed QUESTIONED<->KNOWN decisions into
// cluster->person edges. It is a pure function of the three loads (decisions,
// known-feature persons, cluster membership).
func loadIdentificationRows(ctx context.Context, sqlDB *sql.DB) ([]identificationRow, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("database is not configured")
	}

	pairs, err := loadConfirmedPairs(ctx, sqlDB)
	if err != nil {
		return nil, err
	}

	knownPersons, err := db.New(sqlDB).ListKnownFeaturePersons(ctx)
	if err != nil {
		return nil, err
	}
	knownPerson := map[string]string{}
	for _, r := range knownPersons {
		knownPerson[graph.KnownFeatureID(r.IdentityFileID)] = r.PersonID
	}

	members, err := db.New(sqlDB).ListClusterMembers(ctx)
	if err != nil {
		return nil, err
	}
	memberCluster := map[string]int64{}
	for _, m := range members {
		memberCluster[m.FeatureID] = m.ClusterID
	}

	var out []identificationRow
	for _, pair := range pairs {
		personA, aKnown := knownPerson[pair.featureA]
		personB, bKnown := knownPerson[pair.featureB]
		if aKnown == bKnown {
			// Both known (dedup) or both questioned (linkage) — not an
			// identification.
			continue
		}
		var questionedFeature, person string
		if aKnown {
			questionedFeature = pair.featureB
			person = personA
		} else {
			questionedFeature = pair.featureA
			person = personB
		}
		clusterID, ok := memberCluster[questionedFeature]
		if !ok {
			continue
		}
		out = append(out, identificationRow{
			clusterID:    clusterID,
			personID:     person,
			confidence:   pair.confidence,
			identifiedBy: pair.identifiedBy,
		})
	}
	return out, nil
}

const deleteIdentifiedAsQuery = `
MATCH ()-[r:IDENTIFIED_AS]->() DELETE r
`

const identifyQuery = `
UNWIND $identifications AS i
MATCH (c:BiometricCluster {clusterId: i.clusterId})
MERGE (p:Person {personId: i.personId})
MERGE (c)-[r:IDENTIFIED_AS]->(p)
ON CREATE SET r.confidence = i.confidence, r.identifiedBy = i.identifiedBy, r.identifiedAt = datetime()
`
