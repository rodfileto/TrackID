// sync-graph materializes criminal_cases and QUESTIONED biometric features from
// Postgres into the Neo4j forensic graph (Evidence and BiometricFeature nodes).
// It reads only from the database; see graph.Sync.
//
// Defaults to a dry run that only reports counts. Pass -commit to write.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()

	commit := cmdutil.CommitFlag()
	flag.Parse()

	db := cmdutil.OpenDB()
	defer db.Close()

	ctx := context.Background()
	if !*commit {
		stats, err := graph.SyncPlan(ctx, db)
		cmdutil.Fatal(err)
		log.Printf("dry run: %d evidence item(s), %d feature(s) would be materialized", stats.Evidence, stats.Features)
		return
	}

	driver := cmdutil.OpenNeo4j(ctx)
	defer driver.Close(ctx)

	stats, err := graph.Sync(ctx, db, driver)
	cmdutil.Fatal(err)
	log.Printf("sync complete: %d evidence item(s), %d feature(s) materialized", stats.Evidence, stats.Features)
}
