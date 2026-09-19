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
// hand. Fingerprint has no such pipeline yet.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/graph"
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

	// Optional, like Vision below: sync-face clustering always materializes
	// to Neo4j (see cluster.Run), so its handler is only registered when
	// this succeeds -- an embedding still gets computed either way, it just
	// won't trigger an automatic match+cluster run until Neo4j is reachable.
	var neo4jDriver neo4j.DriverWithContext
	if configuration.Neo4jURL == "" {
		log.Printf("NEO4J_URL is not configured; %s will not be processed", embedding.TaskTypeSyncFace)
	} else if password := os.Getenv("NEO4J_PASSWORD"); password == "" {
		log.Printf("NEO4J_URL is configured but NEO4J_PASSWORD is not; %s will not be processed", embedding.TaskTypeSyncFace)
	} else {
		username := os.Getenv("NEO4J_USERNAME")
		if username == "" {
			username = "neo4j"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		driver, err := graph.OpenDriver(ctx, configuration.Neo4jURL, username, password)
		cancel()
		if err != nil {
			log.Printf("neo4j is configured but unreachable; %s will not be processed: %v", embedding.TaskTypeSyncFace, err)
		} else {
			neo4jDriver = driver
			defer driver.Close(context.Background())
		}
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
			mux.HandleFunc(embedding.TaskTypeComputeCodification, embedding.HandleComputeCodification(sqlDB, store, vis, queueClient))
			log.Printf("registered handler for %s", embedding.TaskTypeComputeCodification)
			mux.HandleFunc(embedding.TaskTypeComputeIdentityFeature, embedding.HandleComputeIdentityFeature(sqlDB, store, vis, queueClient))
			log.Printf("registered handler for %s", embedding.TaskTypeComputeIdentityFeature)
		}
	}

	if neo4jDriver != nil {
		mux.HandleFunc(embedding.TaskTypeSyncFace, embedding.HandleSyncFace(sqlDB, neo4jDriver))
		log.Printf("registered handler for %s", embedding.TaskTypeSyncFace)
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
