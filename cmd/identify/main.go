// identify materializes IDENTIFIED_AS edges in the Neo4j graph: for every
// confirmed biometric_decisions pair linking a QUESTIONED feature to a KNOWN
// feature, the QUESTIONED feature's cluster is linked to the KNOWN feature's
// person. It reads only from Postgres; see cluster.Identify.
//
// Defaults to a dry run that only reports counts. Pass -commit to write.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/rodfileto/trackid/cluster"
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
		identifications, err := cluster.IdentifyPlan(ctx, db)
		cmdutil.Fatal(err)
		log.Printf("dry run: %d identification(s) would be written", identifications)
		return
	}

	driver := cmdutil.OpenNeo4j(ctx)
	defer driver.Close(ctx)

	identifications, err := cluster.Identify(ctx, db, driver)
	cmdutil.Fatal(err)
	log.Printf("identify complete: %d identification(s) written", identifications)
}
