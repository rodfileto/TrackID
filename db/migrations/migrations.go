// Package migrations embeds the goose-formatted SQL migration files applied
// against the primary Postgres database. See cmd/migrate.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
