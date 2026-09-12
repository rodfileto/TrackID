// Package cluster groups biometric features into same-modality clusters driven
// by biometric_decisions: a POSITIVE decision that settles to CONFIRMED links
// two features. Clusters are persisted in Postgres with stable, sequential ids
// and updated incrementally (extend/merge/split), then materialized to Neo4j.
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

// Run reconciles the confirmed decisions against the persisted clusters and
// materializes the result to Neo4j. It reads only from Postgres.
func Run(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (Stats, error) {
	if db == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}

	featureInfos, err := loadFeatures(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	edges, err := loadConfirmedEdges(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	existing, memberCluster, err := loadClusters(ctx, db)
	if err != nil {
		return Stats{}, err
	}

	caseTypes := make(map[string]string, len(featureInfos))
	featureTypes := make(map[string]string, len(featureInfos))
	for id, fi := range featureInfos {
		caseTypes[id] = fi.caseType
		featureTypes[id] = fi.featureType
	}

	p := reconcile(caseTypes, edges, existing)

	if err := apply(ctx, db, p, memberCluster, existing); err != nil {
		return Stats{}, err
	}
	if err := materialize(ctx, db, driver, featureTypes); err != nil {
		return Stats{}, err
	}
	return stats(p), nil
}

// Plan returns the counts a Run would produce, without writing anything.
func Plan(ctx context.Context, db *sql.DB) (Stats, error) {
	if db == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}

	featureInfos, err := loadFeatures(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	edges, err := loadConfirmedEdges(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	existing, _, err := loadClusters(ctx, db)
	if err != nil {
		return Stats{}, err
	}

	caseTypes := make(map[string]string, len(featureInfos))
	for id, fi := range featureInfos {
		caseTypes[id] = fi.caseType
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

// featureInfo is a biometricfeature's modality (case type) and graph feature type.
type featureInfo struct {
	caseType    string
	featureType string
}

const featuresQuery = `
SELECT identity_file_id, case_trace_id, feature_type
FROM biometricfeature
ORDER BY id
`

// loadFeatures reads every biometricfeature and maps its graph feature id to its
// case type and feature type. Feature ids follow graph.KnownFeatureID /
// graph.QuestionedFeatureID, the single source of truth for those conventions.
func loadFeatures(ctx context.Context, db *sql.DB) (map[string]featureInfo, error) {
	rows, err := db.QueryContext(ctx, featuresQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]featureInfo{}
	for rows.Next() {
		var fileID, traceID sql.NullInt64
		var featureType string
		if err := rows.Scan(&fileID, &traceID, &featureType); err != nil {
			return nil, err
		}
		caseType, ok := graph.CaseTypeForFeatureType(featureType)
		if !ok {
			continue
		}
		var featureID string
		if fileID.Valid {
			featureID = graph.KnownFeatureID(fileID.Int64)
		} else {
			featureID = graph.QuestionedFeatureID(traceID.Int64)
		}
		out[featureID] = featureInfo{caseType: caseType, featureType: featureType}
	}
	return out, rows.Err()
}

// loadConfirmedEdges reads biometric_decisions and returns the edges whose
// derived status is CONFIRMED. The decision modality (FACE/FINGERPRINT) is
// mapped onto the clusters table's case_type vocabulary (FACIAL/FINGERPRINT).
func loadConfirmedEdges(ctx context.Context, db *sql.DB) ([]edge, error) {
	pairs, err := loadConfirmedPairs(ctx, db)
	if err != nil {
		return nil, err
	}
	edges := make([]edge, 0, len(pairs))
	for _, p := range pairs {
		edges = append(edges, edge{
			left:     p.featureA,
			right:    p.featureB,
			modality: caseTypeForModality(p.modality),
		})
	}
	return edges, nil
}

const clustersQuery = `
SELECT id, case_type
FROM clusters
ORDER BY id
`

const membersQuery = `
SELECT cluster_id, feature_id
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
		var featureID string
		if err := rows.Scan(&clusterID, &featureID); err != nil {
			return nil, nil, err
		}
		cs := existing[clusterID]
		cs.members = append(cs.members, featureID)
		existing[clusterID] = cs
		memberCluster[featureID] = clusterID
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

	for featureID, finalID := range final {
		currentID, ok := memberCluster[featureID]
		if ok && currentID == finalID {
			continue
		}
		if ok {
			if _, err := tx.ExecContext(ctx, `DELETE FROM cluster_members WHERE feature_id = $1`, featureID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_members (cluster_id, feature_id) VALUES ($1, $2)`, finalID, featureID); err != nil {
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_merges (from_cluster_id, to_cluster_id, reason) VALUES ($1, $2, $3)`, m.from, m.to, "decision_merge"); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// materialize reads the persisted clusters and memberships and writes the
// BiometricCluster nodes and IN_CLUSTER edges to Neo4j. It rebuilds IN_CLUSTER
// edges from the DB each run but never touches IDENTIFIED_AS. Evidence and
// feature nodes are created by Sync/SyncIdentity, not here.
func materialize(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext, featureTypes map[string]string) error {
	rows, err := db.QueryContext(ctx, `
SELECT c.id, c.case_type, m.feature_id
FROM clusters c
JOIN cluster_members m ON m.cluster_id = c.id
ORDER BY c.id, m.feature_id`)
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
		var caseType, featureID string
		if err := rows.Scan(&id, &caseType, &featureID); err != nil {
			return err
		}
		c, ok := byID[id]
		if !ok {
			c = &clusterData{caseType: caseType}
			byID[id] = c
			order = append(order, id)
		}
		c.members = append(c.members, featureID)
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
				"featureId":   m,
				"featureType": featureTypes[m],
			})
		}
		params = append(params, map[string]any{
			"clusterId":    clusterID,
			"clusterLabel": mapping.ClusterLabel,
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
MERGE (f:Object:BiometricFeature {featureId: member.featureId})
SET f.featureType = member.featureType
MERGE (f)-[:IN_CLUSTER]->(c)
`
