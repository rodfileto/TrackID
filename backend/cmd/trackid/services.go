package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/rodrigorfcm/trackid/backend/internal/config"
	"github.com/rodrigorfcm/trackid/backend/internal/database"
	"github.com/rodrigorfcm/trackid/backend/internal/infobio"
)

// services holds every optional integration main.go wires into the router.
// Each field is nil when its configuration isn't set; handlers already know
// how to treat a nil dependency as "not configured" (503).
type services struct {
	db                     *sql.DB
	infoBioService         *infobio.SearchService
	infoBioDownloadService *infobio.DownloadService
	infoBioSessionManager  *infobio.SessionManager
}

// newServices builds every optional service based on which configuration
// values are set. The returned cleanup func closes whatever was opened, in
// reverse order, and is safe to call even if construction failed partway
// through (newServices already runs it before returning a non-nil error).
func newServices(configuration config.Config) (*services, func(), error) {
	var closers []func()
	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	result := &services{}

	if configuration.DatabaseURL != "" {
		db, err := database.Open(configuration.DatabaseURL)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		result.db = db
		closers = append(closers, func() { db.Close() })
	}

	if configuration.InfoBioBaseURL != "" {
		result.infoBioService = infobio.NewSearchService(configuration.InfoBioBaseURL, http.DefaultClient, nil)

		var sessionStore infobio.SessionStore
		if configuration.RedisURL != "" {
			redisStore, err := infobio.NewRedisSessionStore(configuration.RedisURL)
			if err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("invalid REDIS_URL for InfoBio sessions: %w", err)
			}
			sessionStore = redisStore
			closers = append(closers, func() { redisStore.Close() })
		} else {
			sessionStore = infobio.NewMemorySessionStore()
			log.Printf("REDIS_URL is not configured; using in-memory InfoBio session storage")
		}
		result.infoBioSessionManager = infobio.NewSessionManager(configuration.InfoBioBaseURL, http.DefaultClient, sessionStore)
	}

	if configuration.InfoBioImagesURL != "" || configuration.InfoBioNISTBaseURL != "" {
		result.infoBioDownloadService = infobio.NewDownloadService(configuration.InfoBioImagesURL, configuration.InfoBioNISTBaseURL, http.DefaultClient, nil)
		result.infoBioDownloadService.Search = result.infoBioService
	}

	return result, cleanup, nil
}
