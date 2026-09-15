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

	"github.com/rodfileto/trackid/api"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/database"
	"github.com/rodfileto/trackid/internal/env"
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

	router := api.NewRouter(api.Dependencies{
		DB:          db,
		JWTSecret:   configuration.JWTSecret,
		EmailDomain: configuration.EmailDomain,
		Storage:     store,
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
