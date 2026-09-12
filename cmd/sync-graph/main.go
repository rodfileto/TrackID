// sync-graph materializes criminal_cases from Postgres into the Neo4j forensic
// graph (Evidence, BiometricFeature, and Decision nodes). It reads only from
// the database; see graph.Sync.
//
// Defaults to a dry run that only reports counts. Pass -commit to write, which
// requires NEO4J_URL (and NEO4J_PASSWORD; NEO4J_USERNAME defaults to "neo4j").
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/rodrigorfcm/trackid/database"
	"github.com/rodrigorfcm/trackid/env"
	"github.com/rodrigorfcm/trackid/graph"
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
		stats, err := graph.SyncPlan(ctx, db)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("dry run: %d case(s), %d decision(s) would be materialized", stats.Cases, stats.Decisions)
		return
	}

	driver, err := graph.OpenDriver(ctx, neo4jURL(), neo4jUsername(), neo4jPassword())
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close(ctx)

	stats, err := graph.Sync(ctx, db, driver)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("sync complete: %d case(s), %d decision(s) materialized", stats.Cases, stats.Decisions)
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
