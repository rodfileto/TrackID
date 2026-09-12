// Package migrations embeds the Cypher migration files applied against Neo4j.
// See the graph package's Apply function.
package migrations

import "embed"

//go:embed *.cypher
var FS embed.FS
