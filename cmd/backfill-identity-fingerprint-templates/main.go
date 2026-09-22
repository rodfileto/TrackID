// backfill-identity-fingerprint-templates enqueues fingerprint template
// extraction (fingerprint.ExtractForIdentityFeature) for every enrolled KNOWN
// FINGERPRINT_TEMPLATE biometricfeature that doesn't have one yet -- the
// fingerprint counterpart of cmd/backfill-identity-face-embeddings.
//
// identity.Ingest enqueues this automatically for new enrollments going
// forward when given identity.WithEnqueuer; this is the one-off (and
// re-runnable) catch-up for identity data that was already ingested before
// that wiring existed, or ingested by a caller that didn't pass it. Safe to
// re-run at any time: it only ever selects biometricfeature rows still
// missing a template, so an earlier run's successes are left untouched.
//
// It only needs DATABASE_URL and REDIS_URL; the actual download+extract work
// happens in cmd/worker (registered for
// fingerprint.TaskTypeExtractIdentityFeature), same as it does for
// fingerprint.TaskTypeExtractCodification.
//
// Defaults to a dry run that only reports counts; pass -commit to enqueue.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/fingerprint"
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

	featureIDs, err := db.New(sqlDB).ListKnownFingerprintFeaturesMissingTemplate(ctx, fingerprint.TemplateType)
	cmdutil.Fatal(err)

	if !*commit {
		log.Printf("dry run: %d KNOWN FINGERPRINT_TEMPLATE biometricfeature(s) missing a %s template", len(featureIDs), fingerprint.TemplateType)
		return
	}

	if configuration.RedisURL == "" {
		log.Fatal("REDIS_URL is required to enqueue extraction (cmd/worker consumes it from there)")
	}
	queueClient, err := queue.OpenClient(configuration.RedisURL)
	cmdutil.Fatal(err)
	defer queueClient.Close()

	enqueued, failed := 0, 0
	for _, featureID := range featureIDs {
		task, err := fingerprint.NewExtractIdentityFeatureTask(featureID)
		if err != nil {
			log.Printf("biometricfeature %d: build extract task: %v", featureID, err)
			failed++
			continue
		}
		if _, err := queueClient.Enqueue(task); err != nil {
			log.Printf("biometricfeature %d: enqueue extract task: %v", featureID, err)
			failed++
			continue
		}
		enqueued++
		if enqueued%500 == 0 {
			log.Printf("progress: %d/%d enqueued", enqueued, len(featureIDs))
		}
	}
	log.Printf("backfill-identity-fingerprint-templates complete: %d enqueued, %d failed (of %d missing)", enqueued, failed, len(featureIDs))
}
