// worker runs trackid's Asynq task processor against Redis. It's the
// consumer side of internal/queue: the API server enqueues jobs (e.g.
// embedding.TaskTypeComputeCodification from cases.SaveCodificationImage),
// this process handles them. Task handlers are registered here incrementally
// as each pipeline stage lands.
//
// This process is also a producer, not just a consumer: once a FACE
// embedding is computed, its handler enqueues embedding.TaskTypeSyncFace
// (see embedding.SyncFace) to run matching and clustering incrementally,
// without an operator running cmd/match-embeddings/cmd/cluster-biometrics by
// hand. Fingerprint templates do the same through fingerprint.TaskTypeSync.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/database"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/internal/queue"
	"github.com/rodfileto/trackid/storage"
	"github.com/rodfileto/trackid/video"
)

func main() {
	env.Load()

	configuration := config.Load()

	if configuration.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is not configured")
	}
	sqlDB, err := database.Open(configuration.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL is not configured")
	}
	// This process needs Redis both as a consumer (srv, below) and as a
	// producer: a compute-codification/compute-identity-feature handler
	// enqueues embedding.TaskTypeSyncFace once it lands a FACE embedding.
	queueClient, err := queue.OpenClient(redisURL)
	if err != nil {
		log.Fatal(err)
	}
	defer queueClient.Close()

	var store *storage.Client
	if configuration.S3Endpoint != "" && configuration.S3AccessKey != "" &&
		configuration.S3SecretKey != "" && configuration.S3Bucket != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		store, err = storage.NewClient(ctx, configuration.S3Endpoint, configuration.S3AccessKey, configuration.S3SecretKey, configuration.S3Bucket)
		cancel()
		if err != nil {
			log.Printf("object storage is configured but unreachable; embedding tasks will fail: %v", err)
		}
	} else {
		log.Printf("object storage is not configured; embedding tasks will fail")
	}

	mux := asynq.NewServeMux()

	if configuration.VisionDetectorPath == "" || configuration.VisionRecognizerPath == "" {
		log.Printf("VISION_DETECTOR_PATH/VISION_RECOGNIZER_PATH not configured; %s will not be processed", embedding.TaskTypeComputeCodification)
	} else {
		vis, err := vision.NewService(vision.Config{
			Detector:          vision.DetectorKind(configuration.VisionDetector),
			DetectorPath:      configuration.VisionDetectorPath,
			RecognizerPath:    configuration.VisionRecognizerPath,
			SharedLibraryPath: configuration.VisionSharedLibraryPath,
			UseGPU:            configuration.VisionUseGPU,
		})
		if err != nil {
			log.Printf("vision service unavailable; %s will not be processed: %v", embedding.TaskTypeComputeCodification, err)
		} else {
			defer vis.Close()
			mux.HandleFunc(embedding.TaskTypeComputeCodification, embedding.HandleComputeCodification(sqlDB, store, vis, queueClient))
			log.Printf("registered handler for %s", embedding.TaskTypeComputeCodification)
			mux.HandleFunc(embedding.TaskTypeComputeIdentityFeature, embedding.HandleComputeIdentityFeature(sqlDB, store, vis, queueClient))
			log.Printf("registered handler for %s", embedding.TaskTypeComputeIdentityFeature)
			// Video sent in as material: tracks of faces (needs ffmpeg/ffprobe on PATH).
			mux.HandleFunc(video.TaskTypeProcess, video.HandleProcess(sqlDB, store, video.VisionTracker(vis)))
			log.Printf("registered handler for %s", video.TaskTypeProcess)
		}
	}

	mux.HandleFunc(embedding.TaskTypeSyncFace, embedding.HandleSyncFace(sqlDB, queueClient))
	log.Printf("registered handler for %s", embedding.TaskTypeSyncFace)

	// Fingerprint doesn't need Vision: the sidecar extracts and matches.
	if configuration.FingerprintSidecarURL == "" {
		log.Printf("FINGERPRINT_SIDECAR_URL not configured; fingerprint tasks will not be processed")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		fp, err := fingerprint.NewClient(ctx, configuration.FingerprintSidecarURL)
		cancel()
		if err != nil {
			log.Printf("fingerprint sidecar unavailable; fingerprint tasks will not be processed: %v", err)
		} else {
			mux.HandleFunc(fingerprint.TaskTypeExtractCodification, fingerprint.HandleExtractCodification(sqlDB, store, fp, queueClient))
			mux.HandleFunc(fingerprint.TaskTypeExtractIdentityFeature, fingerprint.HandleExtractIdentityFeature(sqlDB, store, fp, queueClient))
			mux.HandleFunc(fingerprint.TaskTypeSync, fingerprint.HandleSync(sqlDB, fp, queueClient))
			log.Printf("registered handlers for %s, %s, %s",
				fingerprint.TaskTypeExtractCodification, fingerprint.TaskTypeExtractIdentityFeature, fingerprint.TaskTypeSync)
		}
	}

	// WORKER_CONCURRENCY sets how many tasks run at once (default 10); the stress test
	// varies it against the GPU and the fingerprint sidecar.
	concurrency := 10
	if v := os.Getenv("WORKER_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			log.Fatalf("WORKER_CONCURRENCY must be a positive integer, got %q", v)
		}
		concurrency = n
	}
	srv, err := queue.OpenServer(redisURL, asynq.Config{
		Concurrency: concurrency,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("trackid worker starting (concurrency %d)", concurrency)
	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
