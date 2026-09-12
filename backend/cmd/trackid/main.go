package main

import (
	"log"

	"github.com/rodrigorfcm/trackid/backend/internal/api"
	"github.com/rodrigorfcm/trackid/backend/internal/config"
	"github.com/rodrigorfcm/trackid/backend/internal/env"
)

func main() {
	env.Load()

	configuration := config.Load()

	appServices, cleanup, err := newServices(configuration)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	log.Printf("trackid backend listening on :%s", configuration.Port)
	router := api.NewRouter(api.Dependencies{
		DB:                     appServices.db,
		JWTSecret:              configuration.JWTSecret,
		InfoBioService:         appServices.infoBioService,
		InfoBioDownloadService: appServices.infoBioDownloadService,
		InfoBioSessionManager:  appServices.infoBioSessionManager,
	})
	if err := router.Run(":" + configuration.Port); err != nil {
		log.Fatal(err)
	}
}
