// worker runs trackid's Asynq task processor against Redis. It's the
// consumer side of internal/queue: the API server enqueues jobs (e.g.
// embedding.TaskTypeComputeCodification from cases.SaveCodificationImage),
// this process handles them. Task handlers are registered here incrementally
// as each pipeline stage lands.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/database"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/internal/queue"
	"github.com/rodfileto/trackid/storage"
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
			DetectorPath:      configuration.VisionDetectorPath,
			RecognizerPath:    configuration.VisionRecognizerPath,
			SharedLibraryPath: configuration.VisionSharedLibraryPath,
			UseGPU:            configuration.VisionUseGPU,
		})
		if err != nil {
			log.Printf("vision service unavailable; %s will not be processed: %v", embedding.TaskTypeComputeCodification, err)
		} else {
			defer vis.Close()
			mux.HandleFunc(embedding.TaskTypeComputeCodification, embedding.HandleComputeCodification(sqlDB, store, vis))
			log.Printf("registered handler for %s", embedding.TaskTypeComputeCodification)
		}
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL is not configured")
	}
	srv, err := queue.OpenServer(redisURL, asynq.Config{
		Concurrency: 10,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("trackid worker starting")
	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
