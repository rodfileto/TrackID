package main

import (
	"database/sql"
	"log"

	"github.com/rodfileto/trackid/api"
	"github.com/rodfileto/trackid/config"
	"github.com/rodfileto/trackid/database"
	"github.com/rodfileto/trackid/env"
)

func main() {
	env.Load()

	configuration := config.Load()

	var db *sql.DB
	if configuration.DatabaseURL != "" {
		var err error
		db, err = database.Open(configuration.DatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
	}

	log.Printf("trackid backend listening on :%s", configuration.Port)
	router := api.NewRouter(api.Dependencies{
		DB:          db,
		JWTSecret:   configuration.JWTSecret,
		EmailDomain: configuration.EmailDomain,
	})
	if err := router.Run(":" + configuration.Port); err != nil {
		log.Fatal(err)
	}
}
