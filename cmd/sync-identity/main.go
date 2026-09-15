// sync-identity materializes the KNOWN identity chain from Postgres into the
// Neo4j graph: Person nodes from the person table, and the
// Identification -> IdentityRegister -> BiometricFeature (KNOWN) chain from the
// biometricfeature table. It reads only from Postgres; see graph.SyncIdentity.
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
		stats, err := graph.SyncIdentityPlan(ctx, db)
		cmdutil.Fatal(err)
		log.Printf("dry run: %d person(s), %d known feature(s) would be materialized", stats.Persons, stats.Features)
		return
	}

	driver := cmdutil.OpenNeo4j(ctx)
	defer driver.Close(ctx)

	stats, err := graph.SyncIdentity(ctx, db, driver)
	cmdutil.Fatal(err)
	log.Printf("sync-identity complete: %d person(s), %d known feature(s) materialized", stats.Persons, stats.Features)
}
