package cluster

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// identificationRow is one identification resolved through the identity chain
// (register -> document -> person) and to the trace's current cluster.
type identificationRow struct {
	clusterID       int64
	personID        string
	confidence      sql.NullFloat64
	responsibleUser sql.NullString
}

// Identify writes IDENTIFIED_AS edges from clusters to persons, derived from the
// identifications table. It reads only from Postgres and rebuilds the edges each
// run, so it is idempotent and consistent with the persisted clusters.
func Identify(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (int, error) {
	rows, err := loadIdentifications(ctx, db)
	if err != nil {
		return 0, err
	}

	params := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var confidence any
		if r.confidence.Valid {
			confidence = r.confidence.Float64
		}
		var responsibleUser any
		if r.responsibleUser.Valid {
			responsibleUser = r.responsibleUser.String
		}
		params = append(params, map[string]any{
			"clusterId":       fmt.Sprintf("CLUSTER-%d", r.clusterID),
			"personId":        r.personID,
			"confidence":      confidence,
			"responsibleUser": responsibleUser,
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
	rows, err := loadIdentifications(ctx, db)
	return len(rows), err
}

const identificationsQuery = `
SELECT m.cluster_id, p.person_id, i.confidence, i.responsible_user
FROM identifications i
JOIN identity_register r ON r.id = i.identity_register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
JOIN cluster_members m ON m.evidence_id = i.trace_id
ORDER BY m.cluster_id, p.person_id
`

func loadIdentifications(ctx context.Context, db *sql.DB) ([]identificationRow, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	rows, err := db.QueryContext(ctx, identificationsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []identificationRow
	for rows.Next() {
		var r identificationRow
		if err := rows.Scan(&r.clusterID, &r.personID, &r.confidence, &r.responsibleUser); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const deleteIdentifiedAsQuery = `
MATCH ()-[r:IDENTIFIED_AS]->() DELETE r
`

const identifyQuery = `
UNWIND $identifications AS i
MATCH (c:BiometricCluster {clusterId: i.clusterId})
MERGE (p:Person {personId: i.personId})
MERGE (c)-[r:IDENTIFIED_AS]->(p)
ON CREATE SET r.confidence = i.confidence, r.identifiedBy = i.responsibleUser, r.identifiedAt = datetime()
`
