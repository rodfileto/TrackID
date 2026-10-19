// Package cluster groups biometric features into same-modality clusters driven
// by biometric_decisions: a POSITIVE decision that settles to CONFIRMED links
// two features. Clusters are persisted in Postgres with stable, sequential ids
// and updated incrementally (extend/merge/split), then materialized to Neo4j.
//
// There are two layers of clusters (Basis). AUTOMATIC clusters follow every
// CONFIRMED pair, a SYSTEM match included, for intelligence; REVIEWED clusters
// follow only the pairs people confirmed (Reviewed), and are the only ones
// forensic views read.
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

// Basis names a layer of clusters (clusters.basis).
type Basis string

const (
	// BasisAutomatic clusters follow every CONFIRMED pair, a SYSTEM match
	// included: fast and wide, for intelligence (the Clusters page, the graph).
	BasisAutomatic Basis = "AUTOMATIC"
	// BasisReviewed clusters follow only the pairs people confirmed (Reviewed):
	// the only layer forensic views (a case, a person) read.
	BasisReviewed Basis = "REVIEWED"
)

// Bases lists both layers, in the order Run reconciles them.
var Bases = []Basis{BasisAutomatic, BasisReviewed}

// LayerStats reports what a run found and changed in one layer.
type LayerStats struct {
	Evidence int
	Clusters int
	Created  int
	Merged   int
}

// Stats reports what a run found and changed: the AUTOMATIC layer's counts
// (embedded, so Stats.Clusters is the automatic count) and the REVIEWED
// layer's.
type Stats struct {
	LayerStats
	Reviewed LayerStats
}

// Run reconciles the confirmed decisions against the persisted clusters of
// both layers (Persist) and materializes the result to Neo4j. It reads only
// from Postgres.
func Run(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext) (Stats, error) {
	st, featureTypes, err := run(ctx, sqlDB, true)
	if err != nil {
		return Stats{}, err
	}
	if err := materialize(ctx, sqlDB, driver, featureTypes); err != nil {
		return Stats{}, err
	}
	return st, nil
}

// Persist reconciles the confirmed decisions against the persisted clusters of
// both layers and writes the result to Postgres only. Run is Persist plus the
// Neo4j materialization; Persist alone serves callers with no graph, e.g.
// database tests.
func Persist(ctx context.Context, sqlDB *sql.DB) (Stats, error) {
	st, _, err := run(ctx, sqlDB, true)
	return st, err
}

// Plan returns the counts a Run would produce, without writing anything.
func Plan(ctx context.Context, sqlDB *sql.DB) (Stats, error) {
	st, _, err := run(ctx, sqlDB, false)
	return st, err
}

// run reconciles both layers, writing them when commit is set, and returns the stats and every
// feature's graph feature type (for materialize).
func run(ctx context.Context, sqlDB *sql.DB, commit bool) (Stats, map[string]string, error) {
	if sqlDB == nil {
		return Stats{}, nil, fmt.Errorf("database is not configured")
	}

	featureInfos, err := loadFeatures(ctx, sqlDB)
	if err != nil {
		return Stats{}, nil, err
	}
	pairs, err := loadConfirmedPairs(ctx, sqlDB)
	if err != nil {
		return Stats{}, nil, err
	}

	modalities := make(map[string]string, len(featureInfos))
	featureTypes := make(map[string]string, len(featureInfos))
	for id, fi := range featureInfos {
		modalities[id] = fi.modality
		featureTypes[id] = fi.featureType
	}

	var out Stats
	for _, basis := range Bases {
		existing, memberCluster, err := loadClusters(ctx, sqlDB, basis)
		if err != nil {
			return Stats{}, nil, err
		}
		p := reconcile(modalities, edgesFor(pairs, basis), existing)
		if commit {
			if err := apply(ctx, sqlDB, basis, p, memberCluster, existing); err != nil {
				return Stats{}, nil, err
			}
		}
		if basis == BasisReviewed {
			out.Reviewed = stats(p)
		} else {
			out.LayerStats = stats(p)
		}
	}
	return out, featureTypes, nil
}

func stats(p plan) LayerStats {
	evidence := 0
	for _, members := range p.kept {
		evidence += len(members)
	}
	for _, nc := range p.newClusters {
		evidence += len(nc.evidence)
	}
	return LayerStats{
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

// edgesFor turns the CONFIRMED pairs into the edges of one layer: every pair
// for AUTOMATIC, the reviewed ones for REVIEWED. The decision modality
// (FACE/FINGERPRINT) is mapped onto the clusters table's modality vocabulary
// (FACIAL/FINGERPRINT).
func edgesFor(pairs []confirmedPair, basis Basis) []edge {
	edges := make([]edge, 0, len(pairs))
	for _, p := range pairs {
		if basis == BasisReviewed && !p.reviewed {
			continue
		}
		edges = append(edges, edge{
			left:     p.featureA,
			right:    p.featureB,
			modality: clusterModalityFor(p.modality),
		})
	}
	return edges
}

func loadClusters(ctx context.Context, sqlDB *sql.DB, basis Basis) (map[int64]clusterState, map[string]int64, error) {
	q := db.New(sqlDB)

	clusters, err := q.ListClusters(ctx, string(basis))
	if err != nil {
		return nil, nil, err
	}
	existing := map[int64]clusterState{}
	for _, c := range clusters {
		existing[c.ID] = clusterState{modality: c.Modality}
	}

	members, err := q.ListClusterMembers(ctx, string(basis))
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

// apply writes one layer's plan to Postgres in one transaction.
func apply(ctx context.Context, sqlDB *sql.DB, basis Basis, p plan, memberCluster map[string]int64, existing map[int64]clusterState) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)

	newIDs := make([]int64, len(p.newClusters))
	for i, nc := range p.newClusters {
		id, err := q.CreateCluster(ctx, db.CreateClusterParams{Modality: nc.modality, Basis: string(basis)})
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
			if err := q.DeleteClusterMember(ctx, db.DeleteClusterMemberParams{Basis: string(basis), FeatureID: featureID}); err != nil {
				return err
			}
		}
		if err := q.InsertClusterMember(ctx, db.InsertClusterMemberParams{ClusterID: finalID, FeatureID: featureID, Basis: string(basis)}); err != nil {
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
			Basis:         string(basis),
		}); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// materialize reads the persisted clusters and memberships of both layers and
// writes the BiometricCluster nodes (with their basis) and IN_CLUSTER edges to
// Neo4j, so a feature has one IN_CLUSTER edge per layer. It rebuilds IN_CLUSTER
// edges from the DB each run but never touches IDENTIFIED_AS. Evidence and
// feature nodes are created by Sync/SyncIdentity, not here.
func materialize(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext, featureTypes map[string]string) error {
	rows, err := db.New(sqlDB).ListClusterMembersJoined(ctx)
	if err != nil {
		return err
	}

	type clusterData struct {
		modality string
		basis    string
		members  []string
	}
	var order []int64
	byID := map[int64]*clusterData{}
	for _, r := range rows {
		c, ok := byID[r.ClusterID]
		if !ok {
			c = &clusterData{modality: r.Modality, basis: r.Basis}
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
			"basis":        c.basis,
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
SET c.basis = cluster.basis
FOREACH (_ IN CASE WHEN cluster.clusterLabel = 'Fingerprint' THEN [1] ELSE [] END | SET c:Fingerprint)
FOREACH (_ IN CASE WHEN cluster.clusterLabel = 'Face' THEN [1] ELSE [] END | SET c:Face)
WITH c, cluster
UNWIND cluster.members AS member
MERGE (f:Object:BiometricFeature {featureId: member.featureId})
SET f.featureType = member.featureType
MERGE (f)-[:IN_CLUSTER]->(c)
`
