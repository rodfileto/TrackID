// backfill-identity-face-embeddings enqueues face-embedding computation
// (embedding.ComputeForIdentityFeature) for every enrolled KNOWN FACE_RECORD
// biometricfeature that doesn't have one yet -- what person.SearchByFace
// matches an uploaded photo against (see db/queries.sql's
// FindNearestPersonsByFaceEmbedding, which only ever sees KNOWN embeddings).
//
// identity.Ingest enqueues this automatically for new enrollments going
// forward when given identity.WithEnqueuer; this is the one-off (and
// re-runnable) catch-up for identity data that was already ingested before
// that wiring existed, or ingested by a caller that didn't pass it. Like
// cmd/detect-facial-evidence, it's safe to re-run at any time: it only ever
// selects biometricfeature rows still missing an embedding, so an earlier
// run's successes are left untouched.
//
// Unlike cmd/detect-facial-evidence, this tool does no detection itself --
// an identity_file's photo already IS the face record, no evidence image to
// search for one in. It only needs DATABASE_URL and REDIS_URL; the actual
// download+embed work happens in cmd/worker (registered for
// embedding.TaskTypeComputeIdentityFeature), same as it does for
// TaskTypeComputeCodification.
//
// Defaults to a dry run that only reports counts; pass -commit to enqueue.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/internal/queue"
)

func main() {
	env.Load()

	commit := cmdutil.CommitFlag()
	flag.Parse()

	configuration := config.Load()
	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()

	ctx := context.Background()

	featureIDs, err := db.New(sqlDB).ListKnownFaceFeaturesMissingEmbedding(ctx, embedding.EmbeddingType)
	cmdutil.Fatal(err)

	if !*commit {
		log.Printf("dry run: %d KNOWN FACE_RECORD biometricfeature(s) missing a %s embedding", len(featureIDs), embedding.EmbeddingType)
		return
	}

	if configuration.RedisURL == "" {
		log.Fatal("REDIS_URL is required to enqueue embedding computation (cmd/worker consumes it from there)")
	}
	queueClient, err := queue.OpenClient(configuration.RedisURL)
	cmdutil.Fatal(err)
	defer queueClient.Close()

	enqueued, failed := 0, 0
	for _, featureID := range featureIDs {
		task, err := embedding.NewComputeIdentityFeatureTask(featureID)
		if err != nil {
			log.Printf("biometricfeature %d: build embed task: %v", featureID, err)
			failed++
			continue
		}
		if _, err := queueClient.Enqueue(task); err != nil {
			log.Printf("biometricfeature %d: enqueue embed task: %v", featureID, err)
			failed++
			continue
		}
		enqueued++
		if enqueued%500 == 0 {
			log.Printf("progress: %d/%d enqueued", enqueued, len(featureIDs))
		}
	}
	log.Printf("backfill-identity-face-embeddings complete: %d enqueued, %d failed (of %d missing)", enqueued, failed, len(featureIDs))
}
