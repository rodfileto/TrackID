// Package cluster groups biometric features into same-modality clusters driven
// by biometric_decisions: a POSITIVE decision that settles to CONFIRMED links
// two features. Clusters are persisted in Postgres with stable, sequential ids
// and updated incrementally (extend/merge/split).
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

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

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
// both layers and writes the result.
func Run(ctx context.Context, sqlDB *sql.DB) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	// One rewrite at a time: a face sync and a fingerprint sync can both end here at once.
	var out Stats
	err := exclusive(ctx, sqlDB, lockClusterRun, func(ctx context.Context) error {
		var err error
		out, err = run(ctx, sqlDB, true)
		return err
	})
	return out, err
}

// Plan returns the counts a Run would produce, without writing anything.
func Plan(ctx context.Context, sqlDB *sql.DB) (Stats, error) {
	return run(ctx, sqlDB, false)
}

// run reconciles both layers, writing them when commit is set.
func run(ctx context.Context, sqlDB *sql.DB, commit bool) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}

	featureInfos, err := loadFeatures(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}
	pairs, err := loadConfirmedPairs(ctx, sqlDB)
	if err != nil {
		return Stats{}, err
	}

	modalities := make(map[string]string, len(featureInfos))
	for id, fi := range featureInfos {
		modalities[id] = fi.modality
	}

	var out Stats
	for _, basis := range Bases {
		existing, memberCluster, err := loadClusters(ctx, sqlDB, basis)
		if err != nil {
			return Stats{}, err
		}
		p := reconcile(modalities, edgesFor(pairs, basis), existing)
		if commit {
			if err := apply(ctx, sqlDB, basis, p, memberCluster, existing); err != nil {
				return Stats{}, err
			}
		}
		if basis == BasisReviewed {
			out.Reviewed = stats(p)
		} else {
			out.LayerStats = stats(p)
		}
	}
	return out, nil
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

// featureInfo is a biometricfeature's modality.
type featureInfo struct {
	modality string
}

// loadFeatures reads every biometricfeature and maps its feature id to its
// modality. Feature ids follow graph.KnownFeatureID /
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
		out[featureID] = featureInfo{modality: modality}
	}
	return out, nil
}

// edgesFor turns the CONFIRMED pairs into the edges of one layer: every pair
// for AUTOMATIC, the reviewed ones for REVIEWED.
func edgesFor(pairs []confirmedPair, basis Basis) []edge {
	edges := make([]edge, 0, len(pairs))
	for _, p := range pairs {
		if basis == BasisReviewed && !p.reviewed {
			continue
		}
		edges = append(edges, edge{
			left:     p.featureA,
			right:    p.featureB,
			modality: p.modality,
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
