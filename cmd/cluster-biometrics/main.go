// cluster-biometrics groups criminal-case evidence into biometric clusters by
// modality, reading comparison records from Postgres and writing
// BiometricCluster nodes to Neo4j. See cluster.Run.
//
// Defaults to a dry run that only reports counts. Pass -commit to write, which
// requires NEO4J_URL (and NEO4J_PASSWORD; NEO4J_USERNAME defaults to "neo4j").
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/database"
	"github.com/rodfileto/trackid/env"
	"github.com/rodfileto/trackid/graph"
)

func main() {
	env.Load()

	commit := flag.Bool("commit", false, "actually write to Neo4j (default is a dry run that only reports counts)")
	flag.Parse()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if !*commit {
		stats, err := cluster.Plan(ctx, db)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("dry run: %d cluster(s) over %d evidence item(s); %d would be created, %d merged",
			stats.Clusters, stats.Evidence, stats.Created, stats.Merged)
		return
	}

	driver, err := graph.OpenDriver(ctx, neo4jURL(), neo4jUsername(), neo4jPassword())
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close(ctx)

	stats, err := cluster.Run(ctx, db, driver)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("clustering complete: %d cluster(s) over %d evidence item(s); %d created, %d merged",
		stats.Clusters, stats.Evidence, stats.Created, stats.Merged)
}

func neo4jURL() string {
	url := os.Getenv("NEO4J_URL")
	if url == "" {
		log.Fatal("NEO4J_URL is not configured")
	}
	return url
}

func neo4jUsername() string {
	username := os.Getenv("NEO4J_USERNAME")
	if username == "" {
		username = "neo4j"
	}
	return username
}

func neo4jPassword() string {
	password := os.Getenv("NEO4J_PASSWORD")
	if password == "" {
		log.Fatal("NEO4J_PASSWORD is not configured")
	}
	return password
}
