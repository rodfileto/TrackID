package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/rodrigorfcm/trackid/backend/internal/database"
	"github.com/rodrigorfcm/trackid/backend/internal/env"
	"github.com/rodrigorfcm/trackid/backend/internal/fingerprintcase"
)

func main() {
	env.Load()

	path := flag.String("file", "data/LATENTES_SRMG.csv", "path to the fingerprint-case CSV")
	flag.Parse()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not configured")
	}
	file, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	db, err := database.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	stats, err := fingerprintcase.Import(context.Background(), db, file)
	if err != nil {
		for _, rowError := range stats.Errors {
			log.Printf("%s", rowError)
		}
		log.Fatalf("import failed after reading %d rows: %v", stats.Read, err)
	}
	log.Printf("import complete: read=%d inserted=%d updated=%d",
		stats.Read, stats.Inserted, stats.Updated)
}
