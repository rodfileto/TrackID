// Package cmdutil holds the shared bootstrap helpers for the command-line
// tools under cmd/. It keeps the mains thin and avoids duplicating the
// DATABASE_URL / NEO4J_* wiring across the graph-writing commands.
package cmdutil

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/graph"
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

// OpenNeo4j reads NEO4J_URL (and NEO4J_USERNAME/NEO4J_PASSWORD, the username
// defaulting to "neo4j"), opens the driver, verifies connectivity, and exits on
// failure.
func OpenNeo4j(ctx context.Context) neo4j.DriverWithContext {
	url := os.Getenv("NEO4J_URL")
	if url == "" {
		log.Fatal("NEO4J_URL is not configured")
	}
	username := os.Getenv("NEO4J_USERNAME")
	if username == "" {
		username = "neo4j"
	}
	password := os.Getenv("NEO4J_PASSWORD")
	if password == "" {
		log.Fatal("NEO4J_PASSWORD is not configured")
	}
	driver, err := graph.OpenDriver(ctx, url, username, password)
	if err != nil {
		log.Fatal(err)
	}
	return driver
}

// Fatal logs err and exits non-zero when err is non-nil.
func Fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
