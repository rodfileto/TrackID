// cluster-biometrics groups biometric features into same-modality clusters,
// reading biometric_decisions from Postgres and writing BiometricCluster nodes
// to Neo4j. See cluster.Run.
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
		stats, err := cluster.Plan(ctx, db)
		cmdutil.Fatal(err)
		log.Printf("dry run: %d cluster(s) over %d evidence item(s); %d would be created, %d merged",
			stats.Clusters, stats.Evidence, stats.Created, stats.Merged)
		return
	}

	driver := cmdutil.OpenNeo4j(ctx)
	defer driver.Close(ctx)

	stats, err := cluster.Run(ctx, db, driver)
	cmdutil.Fatal(err)
	log.Printf("clustering complete: %d cluster(s) over %d evidence item(s); %d created, %d merged",
		stats.Clusters, stats.Evidence, stats.Created, stats.Merged)
}
