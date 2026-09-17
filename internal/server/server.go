// Package server assembles the HTTP application: it loads configuration, opens
// the database, wires the API router, mounts the prototype frontend, and runs
// the server. It is the composition root for cmd/trackid.
package server

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/api"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/database"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/internal/queue"
	"github.com/rodfileto/trackid/internal/web"
	"github.com/rodfileto/trackid/storage"
)

// Run starts the API server on the configured port. The database is optional:
// when DATABASE_URL is empty the API still serves /health and the frontend, and
// every DB-backed route returns 503.
func Run() error {
	env.Load()

	configuration := config.Load()

	var db *sql.DB
	if configuration.DatabaseURL != "" {
		var err error
		db, err = database.Open(configuration.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
	}

	var store *storage.Client
	if configuration.S3Endpoint != "" && configuration.S3AccessKey != "" &&
		configuration.S3SecretKey != "" && configuration.S3Bucket != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		store, _ = storage.NewClient(ctx, configuration.S3Endpoint, configuration.S3AccessKey, configuration.S3SecretKey, configuration.S3Bucket)
		cancel()
		if store == nil {
			log.Printf("object storage is configured but unreachable; evidence uploads will return 503")
		}
	} else {
		log.Printf("object storage is not configured; evidence uploads will return 503")
	}

	var queueClient *asynq.Client
	if configuration.RedisURL != "" {
		var err error
		queueClient, err = queue.OpenClient(configuration.RedisURL)
		if err != nil {
			log.Printf("Redis is configured but the queue client could not be created; background jobs will not be enqueued: %v", err)
		} else {
			defer queueClient.Close()
		}
	} else {
		log.Printf("REDIS_URL is not configured; background jobs will not be enqueued")
	}

	// Assigned only on success: a nil *vision.Service stored in the interface
	// would not compare equal to nil, and the handler relies on that check.
	var faceVision cases.FaceVision
	if configuration.VisionDetectorPath != "" && configuration.VisionRecognizerPath != "" {
		vis, err := vision.NewService(vision.Config{
			DetectorPath:      configuration.VisionDetectorPath,
			RecognizerPath:    configuration.VisionRecognizerPath,
			SharedLibraryPath: configuration.VisionSharedLibraryPath,
			UseGPU:            configuration.VisionUseGPU,
		})
		if err != nil {
			log.Printf("vision service unavailable; face detection and on-the-fly face comparison will return 503: %v", err)
		} else {
			defer vis.Close()
			faceVision = vis
		}
	} else {
		log.Printf("VISION_DETECTOR_PATH/VISION_RECOGNIZER_PATH not configured; face detection and on-the-fly face comparison will return 503")
	}

	router := api.NewRouter(api.Dependencies{
		DB:          db,
		JWTSecret:   configuration.JWTSecret,
		EmailDomain: configuration.EmailDomain,
		Storage:     store,
		Queue:       queueClient,
		FaceVision:  faceVision,
	})

	if distDir := web.DistDir(); distDir != "" {
		log.Printf("serving prototype frontend from %s", distDir)
		router.NoRoute(gin.WrapH(web.SPAHandler(distDir)))
	} else {
		log.Printf("frontend build not found (build it with: cd frontend && npm run build); serving API only")
	}

	log.Printf("trackid backend listening on :%s", configuration.Port)
	return router.Run(":" + configuration.Port)
}
