// init-storage makes sure the evidence bucket exists with object lock and the deployment's
// retention period (S3_RETENTION_DAYS, in days), before the API or the worker first stores
// anything (docs/production-plan.md, decision 12). Locking can only be turned on when a bucket is
// created, so it refuses an existing bucket without it rather than let evidence go unprotected.
// Run once at every start; it changes nothing when the bucket is already as asked.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/storage"
)

func main() {
	env.Load()
	endpoint, accessKey, secretKey, bucket := os.Getenv("S3_ENDPOINT"), os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), os.Getenv("S3_BUCKET")
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		log.Fatal("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY and S3_BUCKET must all be set")
	}
	value := os.Getenv("S3_RETENTION_DAYS")
	days, err := strconv.ParseUint(value, 10, 32)
	if value == "" || err != nil || days == 0 {
		log.Fatalf("S3_RETENTION_DAYS must be a whole number of days, at least 1 (got %q): how long stored evidence "+
			"can't be deleted or overwritten is the institute's policy, so there is no default", value)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := storage.EnsureLockedBucket(ctx, endpoint, accessKey, secretKey, bucket, uint(days))
	if err != nil {
		log.Fatal(err)
	}
	switch {
	case result.Created:
		log.Printf("created bucket %q with object lock, COMPLIANCE retention %d days", bucket, result.Days)
	case result.Changed:
		log.Printf("bucket %q: default retention set to %d days (objects already stored keep theirs)", bucket, result.Days)
	default:
		log.Printf("bucket %q is locked, COMPLIANCE retention %d days", bucket, result.Days)
	}
}
