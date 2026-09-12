// Package cluster groups criminal-case evidence into biometric clusters by
// modality. Clusters are persisted in Postgres with stable, sequential ids and
// updated incrementally (extend/merge/split), then materialized to Neo4j.
package cluster

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/graph"
)

// clusterSource is the value stored on BiometricCluster.source for clusters
// this package materializes.
const clusterSource = "TRACKID_CLUSTERING"

// Stats reports what a run found and changed.
type Stats struct {
	Evidence int
	Clusters int
	Created  int
	Merged   int
}

// Run reconciles the confirmed comparisons against the persisted clusters and
// materializes the result to Neo4j. It reads only from Postgres.
func Run(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (Stats, error) {
	if db == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	caseTypes, edges, existing, memberCluster, err := load(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	p := reconcile(caseTypes, edges, existing)

	if err := apply(ctx, db, p, memberCluster, existing); err != nil {
		return Stats{}, err
	}
	if err := materialize(ctx, db, driver); err != nil {
		return Stats{}, err
	}
	return stats(p), nil
}

// Plan returns the counts a Run would produce, without writing anything.
func Plan(ctx context.Context, db *sql.DB) (Stats, error) {
	if db == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	caseTypes, edges, existing, _, err := load(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	return stats(reconcile(caseTypes, edges, existing)), nil
}

func stats(p plan) Stats {
	evidence := 0
	for _, members := range p.kept {
		evidence += len(members)
	}
	for _, nc := range p.newClusters {
		evidence += len(nc.evidence)
	}
	return Stats{
		Evidence: evidence,
		Clusters: len(p.kept) + len(p.newClusters),
		Created:  len(p.newClusters),
		Merged:   len(p.merges),
	}
}

// load reads everything a run needs: case types, confirmed edges, and the
// persisted clusters and memberships.
func load(ctx context.Context, db *sql.DB) (map[string]string, []edge, map[int64]clusterState, map[string]int64, error) {
	caseTypes, err := loadCaseTypes(ctx, db)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	edges, err := loadConfirmedEdges(ctx, db)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	existing, memberCluster, err := loadClusters(ctx, db)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return caseTypes, edges, existing, memberCluster, nil
}

const caseTypesQuery = `
SELECT case_id, case_type
FROM criminal_cases
`

func loadCaseTypes(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, caseTypesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, caseType string
		if err := rows.Scan(&id, &caseType); err != nil {
			return nil, err
		}
		out[id] = caseType
	}
	return out, rows.Err()
}

const edgesQuery = `
SELECT evidence_a, evidence_b, case_type
FROM comparisons
WHERE status = 'confirmed'
`

func loadConfirmedEdges(ctx context.Context, db *sql.DB) ([]edge, error) {
	rows, err := db.QueryContext(ctx, edgesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.left, &e.right, &e.modality); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

const clustersQuery = `
SELECT id, case_type
FROM clusters
ORDER BY id
`

const membersQuery = `
SELECT cluster_id, evidence_id
FROM cluster_members
`

func loadClusters(ctx context.Context, db *sql.DB) (map[int64]clusterState, map[string]int64, error) {
	existing := map[int64]clusterState{}
	rows, err := db.QueryContext(ctx, clustersQuery)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id int64
		var caseType string
		if err := rows.Scan(&id, &caseType); err != nil {
			rows.Close()
			return nil, nil, err
		}
		existing[id] = clusterState{caseType: caseType}
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}

	memberCluster := map[string]int64{}
	rows, err = db.QueryContext(ctx, membersQuery)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var clusterID int64
		var evidenceID string
		if err := rows.Scan(&clusterID, &evidenceID); err != nil {
			return nil, nil, err
		}
		cs := existing[clusterID]
		cs.members = append(cs.members, evidenceID)
		existing[clusterID] = cs
		memberCluster[evidenceID] = clusterID
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return existing, memberCluster, nil
}

// apply writes the plan to Postgres in one transaction.
func apply(ctx context.Context, db *sql.DB, p plan, memberCluster map[string]int64, existing map[int64]clusterState) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newIDs := make([]int64, len(p.newClusters))
	for i, nc := range p.newClusters {
		if err := tx.QueryRowContext(ctx, `INSERT INTO clusters (case_type) VALUES ($1) RETURNING id`, nc.modality).Scan(&newIDs[i]); err != nil {
			return err
		}
	}

	final := map[string]int64{}
	for id, members := range p.kept {
		for _, m := range members {
			final[m] = id
		}
	}
	for i, nc := range p.newClusters {
		for _, m := range nc.evidence {
			final[m] = newIDs[i]
		}
	}

	for evidence, finalID := range final {
		currentID, ok := memberCluster[evidence]
		if ok && currentID == finalID {
			continue
		}
		if ok {
			if _, err := tx.ExecContext(ctx, `DELETE FROM cluster_members WHERE evidence_id = $1`, evidence); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_members (cluster_id, evidence_id) VALUES ($1, $2)`, finalID, evidence); err != nil {
			return err
		}
	}

	for id := range existing {
		if _, ok := p.kept[id]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM clusters WHERE id = $1`, id); err != nil {
			return err
		}
	}

	for _, m := range p.merges {
		if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_merges (from_cluster_id, to_cluster_id, reason) VALUES ($1, $2, $3)`, m.from, m.to, "comparison_merge"); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// materialize reads the persisted clusters and memberships and writes the
// BiometricCluster nodes and IN_CLUSTER edges to Neo4j. It rebuilds IN_CLUSTER
// edges from the DB each run but never touches cluster nodes or IDENTIFIED_AS.
func materialize(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) error {
	rows, err := db.QueryContext(ctx, `
SELECT c.id, c.case_type, m.evidence_id
FROM clusters c
JOIN cluster_members m ON m.cluster_id = c.id
ORDER BY c.id, m.evidence_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type clusterData struct {
		caseType string
		members  []string
	}
	var order []int64
	byID := map[int64]*clusterData{}
	for rows.Next() {
		var id int64
		var caseType, evidenceID string
		if err := rows.Scan(&id, &caseType, &evidenceID); err != nil {
			return err
		}
		c, ok := byID[id]
		if !ok {
			c = &clusterData{caseType: caseType}
			byID[id] = c
			order = append(order, id)
		}
		c.members = append(c.members, evidenceID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	params := make([]map[string]any, 0, len(order))
	ids := make([]string, 0, len(order))
	for _, id := range order {
		c := byID[id]
		mapping, ok := graph.Modalities[c.caseType]
		if !ok {
			continue
		}
		clusterID := fmt.Sprintf("CLUSTER-%d", id)
		ids = append(ids, clusterID)
		members := make([]map[string]any, 0, len(c.members))
		for _, m := range c.members {
			members = append(members, map[string]any{
				"evidenceId": m,
				"featureId":  graph.FeatureID(m),
			})
		}
		params = append(params, map[string]any{
			"clusterId":    clusterID,
			"clusterLabel": mapping.ClusterLabel,
			"evidenceType": mapping.EvidenceType,
			"featureType":  mapping.FeatureType,
			"featureLabel": mapping.FeatureLabel,
			"members":      members,
		})
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		if _, err := tx.Run(ctx, deleteInClusterQuery, nil); err != nil {
			return nil, err
		}
		if _, err := tx.Run(ctx, deleteStaleClustersQuery, map[string]any{"ids": ids}); err != nil {
			return nil, err
		}
		if len(params) > 0 {
			_, err := tx.Run(ctx, upsertClustersQuery, map[string]any{"clusters": params, "source": clusterSource})
			return nil, err
		}
		return nil, nil
	}); err != nil {
		return fmt.Errorf("materialize clusters: %w", err)
	}
	return nil
}

const deleteInClusterQuery = `
MATCH ()-[r:IN_CLUSTER]->(:BiometricCluster)
DELETE r
`

const deleteStaleClustersQuery = `
MATCH (c:BiometricCluster)
WHERE NOT c.clusterId IN $ids
DETACH DELETE c
`

const upsertClustersQuery = `
UNWIND $clusters AS cluster
MERGE (c:Object:BiometricCluster {clusterId: cluster.clusterId})
ON CREATE SET c.createdAt = datetime(), c.source = $source
FOREACH (_ IN CASE WHEN cluster.clusterLabel = 'Fingerprint' THEN [1] ELSE [] END | SET c:Fingerprint)
FOREACH (_ IN CASE WHEN cluster.clusterLabel = 'Face' THEN [1] ELSE [] END | SET c:Face)
WITH c, cluster
UNWIND cluster.members AS member
MERGE (e:Object:Evidence {evidenceId: member.evidenceId})
MERGE (f:Object:BiometricFeature {featureId: member.featureId})
ON CREATE SET f.featureType = cluster.featureType
FOREACH (_ IN CASE WHEN cluster.featureLabel = 'FingerprintLift' THEN [1] ELSE [] END | SET f:FingerprintLift)
FOREACH (_ IN CASE WHEN cluster.featureLabel = 'FaceCapture' THEN [1] ELSE [] END | SET f:FaceCapture)
MERGE (e)-[:HAS_FEATURE]->(f)
MERGE (f)-[:IN_CLUSTER]->(c)
`
