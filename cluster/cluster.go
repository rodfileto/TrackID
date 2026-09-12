// Package cluster groups criminal-case evidence into biometric clusters by
// modality, reading comparison records directly from Postgres and writing
// BiometricCluster nodes to Neo4j.
package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodrigorfcm/trackid/graph"
)

// clusterSource is the value stored on BiometricCluster.source for clusters
// this package creates. Run deletes and rebuilds every cluster carrying it.
const clusterSource = "CRIMINAL_CASE_CLUSTERING"

// Stats reports what Run found and wrote.
type Stats struct {
	Comparisons int
	Clusters    int
}

// clusterParam is one cluster prepared for the create query.
type clusterParam struct {
	clusterID    string
	clusterLabel string
	evidenceType string
	featureType  string
	featureLabel string
	members      []memberParam
}

type memberParam struct {
	evidenceID string
	featureID  string
}

// Run groups criminal-case evidence into biometric clusters by modality and
// writes each cluster of two or more members to Neo4j. It reads only from
// Postgres, so it is independent of graph.Sync. It is a full recompute: every
// cluster previously written with clusterSource is deleted first.
func Run(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (Stats, error) {
	clusters, stats, err := buildClusters(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	if len(clusters) == 0 {
		return stats, nil
	}

	params := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		members := make([]map[string]any, 0, len(c.members))
		for _, m := range c.members {
			members = append(members, map[string]any{
				"evidenceId": m.evidenceID,
				"featureId":  m.featureID,
			})
		}
		params = append(params, map[string]any{
			"clusterId":    c.clusterID,
			"clusterLabel": c.clusterLabel,
			"evidenceType": c.evidenceType,
			"featureType":  c.featureType,
			"featureLabel": c.featureLabel,
			"members":      members,
		})
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		if _, err := tx.Run(ctx, deleteClustersQuery, map[string]any{"source": clusterSource}); err != nil {
			return nil, err
		}
		_, err := tx.Run(ctx, createClustersQuery, map[string]any{
			"clusters": params,
			"source":   clusterSource,
		})
		return nil, err
	}); err != nil {
		return Stats{}, fmt.Errorf("write clusters: %w", err)
	}
	return stats, nil
}

// Plan returns the counts Run would produce, without connecting to Neo4j.
func Plan(ctx context.Context, db *sql.DB) (Stats, error) {
	_, stats, err := buildClusters(ctx, db)
	return stats, err
}

// buildClusters loads evidence-to-evidence comparisons and groups them into
// connected components per modality.
func buildClusters(ctx context.Context, db *sql.DB) ([]clusterParam, Stats, error) {
	if db == nil {
		return nil, Stats{}, fmt.Errorf("database is not configured")
	}
	edges, err := loadEdges(ctx, db)
	if err != nil {
		return nil, Stats{}, err
	}

	edgesByModality := map[string][][2]string{}
	for _, e := range edges {
		edgesByModality[e.modality] = append(edgesByModality[e.modality], [2]string{e.left, e.right})
	}

	modalities := make([]string, 0, len(edgesByModality))
	for modality := range edgesByModality {
		modalities = append(modalities, modality)
	}
	sort.Strings(modalities)

	var clusters []clusterParam
	for _, modality := range modalities {
		mapping, ok := graph.Modalities[modality]
		if !ok {
			continue
		}
		for _, members := range connectedComponents(edgesByModality[modality]) {
			cluster := clusterParam{
				clusterLabel: mapping.ClusterLabel,
				evidenceType: mapping.EvidenceType,
				featureType:  mapping.FeatureType,
				featureLabel: mapping.FeatureLabel,
			}
			for _, evidenceID := range members {
				cluster.members = append(cluster.members, memberParam{
					evidenceID: evidenceID,
					featureID:  graph.FeatureID(evidenceID),
				})
			}
			clusters = append(clusters, cluster)
		}
	}

	// Largest-first ordering, deterministic within equal sizes.
	sort.Slice(clusters, func(i, j int) bool {
		if len(clusters[i].members) != len(clusters[j].members) {
			return len(clusters[i].members) > len(clusters[j].members)
		}
		return clusters[i].clusterLabel < clusters[j].clusterLabel
	})
	for i := range clusters {
		clusters[i].clusterID = fmt.Sprintf("CLUSTER-%05d", i+1)
	}

	return clusters, Stats{Comparisons: len(edges), Clusters: len(clusters)}, nil
}

type edge struct {
	left     string
	right    string
	modality string
}

const edgesQuery = `
SELECT evidence_a, evidence_b, case_type
FROM comparisons
ORDER BY evidence_a, evidence_b
`

func loadEdges(ctx context.Context, db *sql.DB) ([]edge, error) {
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

// connectedComponents groups undirected edges into connected components and
// returns each component of two or more members, sorted.
func connectedComponents(edges [][2]string) [][]string {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if _, ok := parent[x]; !ok {
			parent[x] = x
		}
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y string) {
		rootX, rootY := find(x), find(y)
		if rootX != rootY {
			parent[rootX] = rootY
		}
	}
	for _, e := range edges {
		union(e[0], e[1])
	}

	members := map[string][]string{}
	for node := range parent {
		root := find(node)
		members[root] = append(members[root], node)
	}

	var components [][]string
	for _, group := range members {
		if len(group) < 2 {
			continue
		}
		sort.Strings(group)
		components = append(components, group)
	}
	return components
}

const deleteClustersQuery = `
MATCH (c:Object:BiometricCluster {source: $source})
DETACH DELETE c
`

const createClustersQuery = `
UNWIND $clusters AS cluster
CREATE (c:Object:BiometricCluster {clusterId: cluster.clusterId, createdAt: datetime(), source: $source})
FOREACH (_ IN CASE WHEN cluster.clusterLabel = 'Fingerprint' THEN [1] ELSE [] END | SET c:Fingerprint)
FOREACH (_ IN CASE WHEN cluster.clusterLabel = 'Face' THEN [1] ELSE [] END | SET c:Face)
WITH c, cluster
UNWIND cluster.members AS member
MERGE (e:Object:Evidence {evidenceId: member.evidenceId})
ON CREATE SET e.evidenceType = cluster.evidenceType
MERGE (f:Object:BiometricFeature {featureId: member.featureId})
ON CREATE SET f.featureType = cluster.featureType
FOREACH (_ IN CASE WHEN cluster.featureLabel = 'FingerprintLift' THEN [1] ELSE [] END | SET f:FingerprintLift)
FOREACH (_ IN CASE WHEN cluster.featureLabel = 'FaceCapture' THEN [1] ELSE [] END | SET f:FaceCapture)
MERGE (e)-[:HAS_FEATURE]->(f)
MERGE (f)-[:IN_CLUSTER]->(c)
`
