package cluster

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/graph"
)

// identificationRow is one IDENTIFIED_AS edge to write: a cluster resolved to a
// person via a POSITIVE decision linking a QUESTIONED feature to a KNOWN one.
type identificationRow struct {
	clusterID      int64
	personID       string
	confidence     *float64
	identifiedBy   string
}

// Identify writes IDENTIFIED_AS edges from clusters to persons, derived from the
// biometric_decisions log: a CONFIRMED decision linking a QUESTIONED feature to
// a KNOWN feature resolves the QUESTIONED feature's cluster to the KNOWN
// feature's person. It reads only from Postgres and rebuilds the edges each run,
// so it is idempotent and consistent with the persisted clusters.
func Identify(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (int, error) {
	rows, err := loadIdentificationRows(ctx, db)
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
			"clusterId":     fmt.Sprintf("CLUSTER-%d", r.clusterID),
			"personId":      r.personID,
			"confidence":    confidence,
			"identifiedBy":  r.identifiedBy,
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
func IdentifyPlan(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := loadIdentificationRows(ctx, db)
	return len(rows), err
}

// knownFeaturePersonsQuery maps each KNOWN feature's graph id (the bare
// identity_file.id) to its person, via the identity chain.
const knownFeaturePersonsQuery = `
SELECT p.person_id, f.id
FROM biometricfeature bf
JOIN identity_file f ON f.id = bf.identity_file_id
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE bf.provenance = 'KNOWN'
`

// loadIdentificationRows resolves confirmed QUESTIONED<->KNOWN decisions into
// cluster->person edges. It is a pure function of the three loads (decisions,
// known-feature persons, cluster membership).
func loadIdentificationRows(ctx context.Context, db *sql.DB) ([]identificationRow, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not configured")
	}

	pairs, err := loadConfirmedPairs(ctx, db)
	if err != nil {
		return nil, err
	}

	knownPerson := map[string]string{}
	rows, err := db.QueryContext(ctx, knownFeaturePersonsQuery)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var personID string
		var fileID int64
		if err := rows.Scan(&personID, &fileID); err != nil {
			rows.Close()
			return nil, err
		}
		knownPerson[graph.KnownFeatureID(fileID)] = personID
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	memberCluster := map[string]int64{}
	rows, err = db.QueryContext(ctx, membersQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var clusterID int64
		var featureID string
		if err := rows.Scan(&clusterID, &featureID); err != nil {
			return nil, err
		}
		memberCluster[featureID] = clusterID
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
