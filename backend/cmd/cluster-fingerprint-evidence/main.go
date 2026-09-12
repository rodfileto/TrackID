// cluster-fingerprint-evidence groups Object:Evidence nodes that cmd/import-latentes-graph linked
// via a COMPARED->Event:Decision->COMPARED path into connected components, and materializes each
// component of two or more members as an Object:BiometricCluster:Fingerprint node per
// backend/internal/graph/migrations/003_biometric_cluster.cypher, with an IN_CLUSTER edge from
// each member's Object:BiometricFeature:FingerprintLift.
//
// Every decision this command reads came from a CSV column that only ever names a related
// reference when a match was recorded (see fingerprintcase.normalizeRow) — there is no
// inconclusive/no-match row to filter out, so every COMPARED path is treated as a confirmed link.
//
// This is a full recompute each run, not an incremental merge: every cluster this command
// previously created (source = "FINGERPRINT_COMPARED_CLUSTERING") is deleted and rebuilt from the
// current graph. That keeps the algorithm simple — plain union-find in Go, no dependency on APOC
// or Graph Data Science (neither is installed on the neo4j:5-community image this project runs) —
// but it means a cluster's clusterId is not stable across runs, and any IDENTIFIED_AS edge a human
// examiner had added to a cluster is orphaned when that cluster is deleted and its replacement
// gets a new id. No cluster has been identified yet, so that doesn't lose anything today, but it
// needs an incremental-merge rewrite before this runs operationally against real casework.
//
// Defaults to a dry run: it only reports how many clusters it would create. Pass -commit to
// actually write, which requires NEO4J_URL (and NEO4J_PASSWORD; NEO4J_USERNAME defaults to
// "neo4j") to be configured.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodrigorfcm/trackid/backend/internal/env"
)

const clusterSource = "FINGERPRINT_COMPARED_CLUSTERING"

const readPairsQuery = `
MATCH (a:Object:Evidence)-[:COMPARED]->(:Event:Decision)-[:COMPARED]->(b:Object:Evidence)
RETURN a.evidenceId AS a, b.evidenceId AS b
`

const deleteExistingClustersQuery = `
MATCH (c:Object:BiometricCluster:Fingerprint {source: $source})
DETACH DELETE c
`

// createClustersQuery expects $clusters as a list of {clusterId, members: [evidenceId, ...]}.
// A cluster whose member has no BiometricFeature (shouldn't happen for data import-latentes-graph
// produced, but not guaranteed for hand-written data) simply gets fewer IN_CLUSTER edges than
// members, rather than failing the whole write.
const createClustersQuery = `
UNWIND $clusters AS cluster
CREATE (c:Object:BiometricCluster:Fingerprint {clusterId: cluster.clusterId, createdAt: datetime(), source: $source})
WITH c, cluster
UNWIND cluster.members AS evidenceId
MATCH (:Object:Evidence {evidenceId: evidenceId})-[:HAS_FEATURE]->(f:Object:BiometricFeature:FingerprintLift)
MERGE (f)-[:IN_CLUSTER]->(c)
`

func main() {
	env.Load()

	commit := flag.Bool("commit", false, "actually write clusters to Neo4j (default is a dry run that only reports counts)")
	flag.Parse()

	neo4jURL := os.Getenv("NEO4J_URL")
	if neo4jURL == "" {
		log.Fatal("NEO4J_URL is not configured")
	}
	username := os.Getenv("NEO4J_USERNAME")
	if username == "" {
		username = "neo4j"
	}
	password := os.Getenv("NEO4J_PASSWORD")
	if password == "" {
		log.Fatal("NEO4J_PASSWORD is not configured")
	}

	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(neo4jURL, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("could not connect to Neo4j at %s: %v", neo4jURL, err)
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	pairs, err := readPairs(ctx, session)
	if err != nil {
		log.Fatalf("failed to read compared evidence pairs: %v", err)
	}
	log.Printf("read %d compared evidence pair(s)", len(pairs))

	clusters := clusterPairs(pairs)
	sort.Slice(clusters, func(i, j int) bool { return len(clusters[i]) > len(clusters[j]) })
	log.Printf("found %d cluster(s) of 2+ evidence items", len(clusters))
	if len(clusters) > 0 {
		log.Printf("largest cluster has %d member(s); smallest has %d", len(clusters[0]), len(clusters[len(clusters)-1]))
	}
	if len(clusters) == 0 {
		return
	}

	if !*commit {
		log.Printf("dry run: pass -commit to replace any existing %s clusters with these %d", clusterSource, len(clusters))
		return
	}

	if err := writeClusters(ctx, session, clusters); err != nil {
		log.Fatalf("failed to write clusters: %v", err)
	}
	log.Printf("wrote %d cluster(s)", len(clusters))
}

func readPairs(ctx context.Context, session neo4j.SessionWithContext) ([][2]string, error) {
	result, err := session.ExecuteRead(ctx, func(transaction neo4j.ManagedTransaction) (any, error) {
		result, err := transaction.Run(ctx, readPairsQuery, nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}
	records, _ := result.([]*neo4j.Record)
	pairs := make([][2]string, 0, len(records))
	for _, record := range records {
		a, _ := record.Get("a")
		b, _ := record.Get("b")
		aStr, aOK := a.(string)
		bStr, bOK := b.(string)
		if !aOK || !bOK {
			return nil, fmt.Errorf("unexpected non-string evidenceId in pair %v/%v", a, b)
		}
		pairs = append(pairs, [2]string{aStr, bStr})
	}
	return pairs, nil
}

// clusterPairs runs plain union-find over the pairs and returns every resulting component that
// has two or more members, each sorted for deterministic output.
func clusterPairs(pairs [][2]string) [][]string {
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
	for _, pair := range pairs {
		union(pair[0], pair[1])
	}

	members := map[string][]string{}
	for node := range parent {
		root := find(node)
		members[root] = append(members[root], node)
	}

	var clusters [][]string
	for _, group := range members {
		if len(group) < 2 {
			continue
		}
		sort.Strings(group)
		clusters = append(clusters, group)
	}
	return clusters
}

func writeClusters(ctx context.Context, session neo4j.SessionWithContext, clusters [][]string) error {
	_, err := session.ExecuteWrite(ctx, func(transaction neo4j.ManagedTransaction) (any, error) {
		if _, err := transaction.Run(ctx, deleteExistingClustersQuery, map[string]any{"source": clusterSource}); err != nil {
			return nil, err
		}

		clusterParams := make([]map[string]any, 0, len(clusters))
		for i, members := range clusters {
			clusterParams = append(clusterParams, map[string]any{
				"clusterId": fmt.Sprintf("FPC-%05d", i+1),
				"members":   members,
			})
		}
		_, err := transaction.Run(ctx, createClustersQuery, map[string]any{
			"clusters": clusterParams,
			"source":   clusterSource,
		})
		return nil, err
	})
	return err
}
