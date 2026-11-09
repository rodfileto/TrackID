// bulk-enroll loads an identity register into TrackID and vectorizes its face photos: the first
// of the two commands of a large-gallery load (docs/large-gallery-plan.md; bulk-match is the
// second).
//
//	bulk-enroll -manifest register.jsonl -shards /data/shards -commit
//
// It runs two resumable steps:
//  1. Rows (with -manifest): every JSON Lines enrollment (bulk.ManifestEnrollment) goes through
//     identity.Ingest, in parallel and with no task queue. Files with a "path" are uploaded to
//     core's bucket; files with a "uri" (file:///..., s3://bucket/key) stay where they are, and
//     STORAGE_FILE_ROOTS must list the directories file:// references may point into.
//  2. Vectorize: every KNOWN face photo with no embedding and no final outcome is fetched,
//     detected and embedded, by keyset pages, one trackid-vision service per GPU (-devices). Each
//     page's embeddings (match_path BULK, so the per-face sync leaves them to bulk-match) and its
//     outcomes (bulk_enroll_outcomes: EMBEDDED, MULTIPLE_FACES, NO_FACE, UNDECODABLE,
//     FETCH_FAILED) are written in one transaction, then the embeddings go to the FP16 vector
//     shards bulk-match reads.
//
// Rerunning resumes: Ingest upserts, and vectorizing skips what has an embedding or a final
// outcome (FETCH_FAILED is retried with -retry-failed). Ctrl-C stops after the page in flight.
// -rebuild-shards writes, from Postgres, shards for every BULK embedding no shard holds.
//
// Defaults to a dry run that only reads the manifest and counts; pass -commit to write.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/bulk"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/storage"
)

func main() {
	env.Load()

	manifest := flag.String("manifest", "", "JSON Lines manifest of enrollments to load first (empty: only vectorize what is already loaded)")
	startLine := flag.Int("start-line", 1, "first manifest line to load (1-based), to resume a long load quickly")
	ingestWorkers := flag.Int("ingest-workers", 8, "enrollments written in parallel")
	skipVectorize := flag.Bool("skip-vectorize", false, "load rows only")
	workers := flag.Int("workers", runtime.NumCPU(), "photos fetched, decoded and embedded in parallel")
	page := flag.Int("page", 512, "photos per page (one write transaction)")
	limit := flag.Int("limit", 0, "stop after this many photos (0: all)")
	devices := flag.String("devices", "", `GPU device ids for trackid-vision, one service each ("0,1"); "cpu" for none; empty: VISION_USE_GPU on device 0`)
	pad := flag.Bool("pad", false, "present each photo padded to the detector, for photos cropped tight to the face (slower)")
	crops := flag.Bool("crops", false, "store a 112-pixel face crop per embedded photo in core's bucket")
	retryFailed := flag.Bool("retry-failed", false, "retry photos whose last outcome was FETCH_FAILED")
	shardsDir := flag.String("shards", "", "directory of FP16 vector shards for bulk-match (required unless -no-shards)")
	noShards := flag.Bool("no-shards", false, "write no shards (bulk-match then fills them from Postgres)")
	shardRows := flag.Int("shard-rows", 1<<20, "embeddings per shard file")
	rebuild := flag.Bool("rebuild-shards", false, "only write shards, from Postgres, for every BULK embedding no shard holds")
	commit := cmdutil.CommitFlag()
	flag.Parse()

	configuration := config.Load()
	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()

	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		log.Printf("stopping after the page in flight (interrupt again to abort)")
		close(stop)
		<-signals
		os.Exit(130)
	}()
	ctx := context.Background()

	if *rebuild {
		if *shardsDir == "" {
			log.Fatal("-rebuild-shards needs -shards")
		}
		if !*commit {
			log.Printf("dry run: would write shards in %s for BULK %s embeddings no shard holds", *shardsDir, embedding.EmbeddingType)
			return
		}
		n, err := bulk.RebuildShards(ctx, sqlDB, *shardsDir, embedding.EmbeddingType, embedding.ModelVersion)
		cmdutil.Fatal(err)
		log.Printf("wrote %d embeddings to shards in %s", n, *shardsDir)
		return
	}
	if !*skipVectorize && !*noShards && *shardsDir == "" {
		log.Fatal("-shards is required (or -no-shards)")
	}

	store := openStorage(configuration)

	if *manifest != "" {
		f, err := os.Open(*manifest)
		cmdutil.Fatal(err)
		lines := make(chan bulk.ManifestLine, 4**ingestWorkers)
		readErr := make(chan error, 1)
		go func() { readErr <- bulk.ReadManifest(f, *startLine, lines) }()
		stats := bulk.Ingest(ctx, sqlDB, store, lines, bulk.IngestOptions{Workers: *ingestWorkers, StartLine: *startLine, Commit: *commit})
		f.Close()
		cmdutil.Fatal(<-readErr)
		verb := "loaded"
		if !*commit {
			verb = "dry run: would load"
		}
		log.Printf("%s %d enrollments (%d files: %d uploaded, %d by URI) of %d lines read; %d invalid, %d failed; last line %d; %s",
			verb, stats.Ingested, stats.Files, stats.Uploaded, stats.External, stats.Lines, stats.Invalid, stats.Failed, stats.LastLine,
			stats.Duration.Round(time.Second))
		for _, e := range stats.Errors() {
			log.Printf("  %s", e)
		}
	}
	if *skipVectorize {
		return
	}

	opts := bulk.VectorizeOptions{
		EmbeddingType: embedding.EmbeddingType,
		ModelVersion:  embedding.ModelVersion,
		Workers:       *workers,
		Page:          *page,
		Limit:         *limit,
		Pad:           *pad,
		Crops:         *crops,
		RetryFailed:   *retryFailed,
		Commit:        *commit,
		Stop:          stop,
	}
	if *commit {
		services := openVision(configuration, *devices)
		defer func() {
			for _, s := range services {
				s.Close()
			}
		}()
		for _, s := range services {
			opts.Embedders = append(opts.Embedders, s)
		}
		if !*noShards {
			cmdutil.Fatal(bulk.OpenShardDir(*shardsDir, embedding.EmbeddingType, embedding.ModelVersion))
			opts.Shards = bulk.NewShardWriter(*shardsDir, *shardRows)
		}
		var last time.Time
		opts.Progress = func(s bulk.VectorizeStats) {
			if time.Since(last) < 10*time.Second {
				return
			}
			last = time.Now()
			rate := float64(s.Done) / s.Duration.Seconds()
			log.Printf("vectorized %d/%d (%.0f photos/s) %v", s.Done, s.Pending, rate, s.Outcomes)
		}
	}
	stats, err := bulk.Vectorize(ctx, sqlDB, store, opts)
	if opts.Shards != nil {
		if ferr := opts.Shards.Flush(); ferr != nil {
			log.Printf("flush shards: %v (bulk-match fills missing shards from Postgres)", ferr)
		}
	}
	cmdutil.Fatal(err)
	if !*commit {
		log.Printf("dry run: %d face photos waiting to be vectorized as %s", stats.Pending, embedding.EmbeddingType)
		return
	}
	shards := 0
	if opts.Shards != nil {
		shards = opts.Shards.Written()
	}
	log.Printf("vectorized %d of %d waiting photos in %s: %v; %d embeddings written to shards",
		stats.Done, stats.Pending, stats.Duration.Round(time.Second), stats.Outcomes, shards)
}

// openStorage connects to core's bucket when it is configured, and otherwise reads only file://
// references (STORAGE_FILE_ROOTS).
func openStorage(configuration config.Config) *storage.Client {
	if configuration.S3Endpoint == "" || configuration.S3AccessKey == "" || configuration.S3SecretKey == "" || configuration.S3Bucket == "" {
		log.Printf("object storage is not configured: only file:// photos can be read, and no path upload or crop written")
		return storage.NewFileClient(storage.FileRootsFromEnv())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := storage.NewClient(ctx, configuration.S3Endpoint, configuration.S3AccessKey, configuration.S3SecretKey, configuration.S3Bucket)
	cmdutil.Fatal(err)
	return store
}

// openVision starts one trackid-vision service per device.
func openVision(configuration config.Config, devices string) []*vision.Service {
	if configuration.VisionDetectorPath == "" || configuration.VisionRecognizerPath == "" {
		log.Fatal("VISION_DETECTOR_PATH and VISION_RECOGNIZER_PATH are required to vectorize")
	}
	base := vision.Config{
		Detector:          vision.DetectorKind(configuration.VisionDetector),
		DetectorPath:      configuration.VisionDetectorPath,
		RecognizerPath:    configuration.VisionRecognizerPath,
		SharedLibraryPath: configuration.VisionSharedLibraryPath,
	}
	var configs []vision.Config
	switch strings.TrimSpace(devices) {
	case "":
		c := base
		c.UseGPU = configuration.VisionUseGPU
		configs = append(configs, c)
	case "cpu":
		configs = append(configs, base)
	default:
		for _, d := range strings.Split(devices, ",") {
			id, err := strconv.Atoi(strings.TrimSpace(d))
			if err != nil {
				log.Fatalf("-devices: %q is not a device id", d)
			}
			c := base
			c.UseGPU, c.DeviceID = true, id
			configs = append(configs, c)
		}
	}
	var out []*vision.Service
	for _, c := range configs {
		s, err := vision.NewService(c)
		if err != nil {
			log.Fatal(fmt.Errorf("vision service (device %d): %w", c.DeviceID, err))
		}
		out = append(out, s)
	}
	return out
}
