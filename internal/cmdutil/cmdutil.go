// Package cmdutil holds the shared bootstrap helpers for the command-line
// tools under cmd/. It keeps the mains thin and avoids duplicating the
// DATABASE_URL wiring across them.
package cmdutil

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/rodfileto/trackid/internal/database"
)

// CommitFlag registers the -commit flag shared by every tool that defaults to a
// dry run. Callers still invoke flag.Parse() themselves.
func CommitFlag() *bool {
	return flag.Bool("commit", false, "actually write (default is a dry run that only reports counts)")
}

// OpenDB reads DATABASE_URL, opens the pooled connection, and exits on failure.
func OpenDB() *sql.DB {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

// Fatal logs err and exits non-zero when err is non-nil.
func Fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
