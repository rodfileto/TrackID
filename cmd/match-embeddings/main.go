// match-embeddings runs ANN similarity search over feature_embeddings and
// records SYSTEM biometric_decisions for pairs within threshold. See
// biometricmatch.Run. Unlike the graph-writing commands, it only touches
// Postgres -- no Neo4j driver is involved.
//
// Defaults to a dry run that only reports counts. Pass -commit to write.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()

	embeddingType := flag.String("embedding-type", "FACE_ARCFACE_512", "feature_embeddings.embedding_type to match")
	threshold := flag.Float64("threshold", 0.4, "maximum cosine distance to consider a match (lower is stricter)")
	source := flag.String("source", "", "biometric_decisions.system_source to record (defaults to -embedding-type)")
	commit := cmdutil.CommitFlag()
	flag.Parse()

	if *source == "" {
		*source = *embeddingType
	}

	db := cmdutil.OpenDB()
	defer db.Close()

	ctx := context.Background()
	if !*commit {
		stats, err := biometricmatch.Plan(ctx, db, *embeddingType, *threshold)
		cmdutil.Fatal(err)
		log.Printf("dry run: %d embedding(s) considered, %d decision(s) would be written", stats.Embeddings, stats.Decisions)
		return
	}

	stats, err := biometricmatch.Run(ctx, db, *embeddingType, *threshold, *source)
	cmdutil.Fatal(err)
	log.Printf("matching complete: %d embedding(s) considered, %d decision(s) written", stats.Embeddings, stats.Decisions)
}
