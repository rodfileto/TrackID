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
	"github.com/rodfileto/trackid/db"
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
func Run(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}

	featureInfos, err := loadFeatures(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}
	edges, err := loadConfirmedEdges(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}
	existing, memberCluster, err := loadClusters(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}

	modalities := make(map[string]string, len(featureInfos))
	featureTypes := make(map[string]string, len(featureInfos))
	for id, fi := range featureInfos {
		modalities[id] = fi.modality
		featureTypes[id] = fi.featureType
	}

	p := reconcile(modalities, edges, existing)

	if err := apply(ctx, sqlDB, p, memberCluster, existing); err != nil {
		return Stats{}, err
	}
	if err := materialize(ctx, sqlDB, driver, featureTypes); err != nil {
		return Stats{}, err
	}
	return stats(p), nil
}

// Plan returns the counts a Run would produce, without writing anything.
func Plan(ctx context.Context, sqlDB *sql.DB) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}

	featureInfos, err := loadFeatures(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}
	edges, err := loadConfirmedEdges(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}
	existing, _, err := loadClusters(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}

	modalities := make(map[string]string, len(featureInfos))
	for id, fi := range featureInfos {
		modalities[id] = fi.modality
	}
	return stats(reconcile(modalities, edges, existing)), nil
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
	modality    string
	featureType string
}

// loadFeatures reads every biometricfeature and maps its graph feature id to its
// case type and feature type. Feature ids follow graph.KnownFeatureID /
// graph.QuestionedFeatureID, the single source of truth for those conventions.
func loadFeatures(ctx context.Context, sqlDB *sql.DB) (map[string]featureInfo, error) {
	rows, err := db.New(sqlDB).ListBiometricFeatures(ctx)
	if err != nil {
		return nil, err
	}

	out := map[string]featureInfo{}
	for _, r := range rows {
		modality, ok := graph.ModalityForFeatureType(r.FeatureType)
		if !ok {
			continue
		}
		var featureID string
		if r.IdentityFileID.Valid {
			featureID = graph.KnownFeatureID(r.IdentityFileID.Int64)
		} else {
			featureID = graph.QuestionedFeatureID(r.CaseTraceID.Int64)
		}
		out[featureID] = featureInfo{modality: modality, featureType: r.FeatureType}
	}
	return out, nil
}

// loadConfirmedEdges reads biometric_decisions and returns the edges whose
// derived status is CONFIRMED. The decision modality (FACE/FINGERPRINT) is
// mapped onto the clusters table's modality vocabulary (FACIAL/FINGERPRINT).
func loadConfirmedEdges(ctx context.Context, sqlDB *sql.DB) ([]edge, error) {
	pairs, err := loadConfirmedPairs(ctx, sqlDB)
	if err != nil {
		return nil, err
	}
	edges := make([]edge, 0, len(pairs))
	for _, p := range pairs {
		edges = append(edges, edge{
			left:     p.featureA,
			right:    p.featureB,
			modality: clusterModalityFor(p.modality),
		})
	}
	return edges, nil
}

func loadClusters(ctx context.Context, sqlDB *sql.DB) (map[int64]clusterState, map[string]int64, error) {
	q := db.New(sqlDB)

	clusters, err := q.ListClusters(ctx)
	if err != nil {
		return nil, nil, err
	}
	existing := map[int64]clusterState{}
	for _, c := range clusters {
		existing[c.ID] = clusterState{modality: c.Modality}
	}

	members, err := q.ListClusterMembers(ctx)
	if err != nil {
		return nil, nil, err
	}
	memberCluster := map[string]int64{}
	for _, m := range members {
		cs := existing[m.ClusterID]
		cs.members = append(cs.members, m.FeatureID)
		existing[m.ClusterID] = cs
		memberCluster[m.FeatureID] = m.ClusterID
	}
	return existing, memberCluster, nil
}

// apply writes the plan to Postgres in one transaction.
func apply(ctx context.Context, sqlDB *sql.DB, p plan, memberCluster map[string]int64, existing map[int64]clusterState) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)

	newIDs := make([]int64, len(p.newClusters))
	for i, nc := range p.newClusters {
		id, err := q.CreateCluster(ctx, nc.modality)
		if err != nil {
			return err
		}
		newIDs[i] = id
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
			if err := q.DeleteClusterMember(ctx, featureID); err != nil {
				return err
			}
		}
		if err := q.InsertClusterMember(ctx, db.InsertClusterMemberParams{ClusterID: finalID, FeatureID: featureID}); err != nil {
			return err
		}
	}

	for id := range existing {
		if _, ok := p.kept[id]; ok {
			continue
		}
		if err := q.DeleteCluster(ctx, id); err != nil {
			return err
		}
	}

	for _, m := range p.merges {
		if err := q.InsertClusterMerge(ctx, db.InsertClusterMergeParams{
			FromClusterID: m.from,
			ToClusterID:   m.to,
			Reason:        sql.NullString{String: "decision_merge", Valid: true},
		}); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// materialize reads the persisted clusters and memberships and writes the
// BiometricCluster nodes and IN_CLUSTER edges to Neo4j. It rebuilds IN_CLUSTER
// edges from the DB each run but never touches IDENTIFIED_AS. Evidence and
// feature nodes are created by Sync/SyncIdentity, not here.
func materialize(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext, featureTypes map[string]string) error {
	rows, err := db.New(sqlDB).ListClusterMembersJoined(ctx)
	if err != nil {
		return err
	}

	type clusterData struct {
		modality string
		members  []string
	}
	var order []int64
	byID := map[int64]*clusterData{}
	for _, r := range rows {
		c, ok := byID[r.ClusterID]
		if !ok {
			c = &clusterData{modality: r.Modality}
			byID[r.ClusterID] = c
			order = append(order, r.ClusterID)
		}
		c.members = append(c.members, r.FeatureID)
	}

	params := make([]map[string]any, 0, len(order))
	ids := make([]string, 0, len(order))
	for _, id := range order {
		c := byID[id]
		mapping, ok := graph.Modalities[c.modality]
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
