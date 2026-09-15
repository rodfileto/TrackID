// migrate applies the goose-formatted Postgres schema migrations. Run with
// "up", "down", or "status"; defaults to "up".
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	db := cmdutil.OpenDB()
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}

	var err error
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
