// verify-storage checks every file the database refers to against object storage: each must be
// there and, where the database recorded its SHA-256, have it (storagecheck). It writes nothing.
// Run it after restoring a backup, and from time to time on the live deployment. It exits 1 when
// a file is missing or differs, and names the rows, not the files' contents.
//
//	verify-storage                 # every file
//	verify-storage -limit 1000     # a quick sample
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/storage"
	"github.com/rodfileto/trackid/storagecheck"
)

func main() {
	env.Load()
	workers := flag.Int("workers", 4, "files read at once")
	limit := flag.Int("limit", 0, "check only this many references (0: all)")
	flag.Parse()

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()
	ctx := context.Background()
	store, err := storage.NewClient(ctx, os.Getenv("S3_ENDPOINT"), os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), os.Getenv("S3_BUCKET"))
	if err != nil {
		log.Fatalf("object storage: %v", err)
	}

	start := time.Now()
	report, err := storagecheck.Run(ctx, sqlDB, store, storagecheck.Options{Workers: *workers, Limit: *limit})
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range report.Problems {
		log.Printf("%s %s id=%d %s", p.Kind, p.Table, p.ID, p.Detail)
	}
	log.Printf("checked %d files (%d against a recorded SHA-256), skipped %d placeholders, %d problems, in %s",
		report.Checked, report.Verified, report.Skipped, len(report.Problems), time.Since(start).Round(time.Second))
	if !report.OK() {
		os.Exit(1)
	}
}
