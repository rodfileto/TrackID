package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/rodfileto/trackid/database"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/env"
)

func main() {
	env.Load()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not configured")
	}

	db, err := database.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}

	switch command {
	case "up":
		err = goose.Up(db, ".")
	case "down":
		err = goose.Down(db, ".")
	case "status":
		err = goose.Status(db, ".")
	default:
		log.Fatal(fmt.Sprintf("unknown command %q: expected up, down, or status", command))
	}
	if err != nil {
		log.Fatal(err)
	}
}
