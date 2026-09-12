// Package graph provides the Neo4j schema migrations and applies them.
package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/graph/migrations"
)

// Apply runs every embedded Cypher migration against the given driver, in
// filename order. Each file's statements run in their own write transactions,
// so a failure in one file leaves the others applied.
func Apply(ctx context.Context, driver neo4j.DriverWithContext) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if err := applyFile(ctx, driver, string(content)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// applyFile splits one migration file into individual statements and runs each
// in its own write transaction. The Bolt protocol accepts a single statement
// per RUN, so the file cannot be sent whole.
func applyFile(ctx context.Context, driver neo4j.DriverWithContext, cypher string) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	for _, statement := range splitStatements(cypher) {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, statement, nil)
			return nil, err
		}); err != nil {
			return err
		}
	}
	return nil
}

// splitStatements splits a Cypher script on semicolons that are outside
// strings, backtick identifiers, and line/block comments.
func splitStatements(cypher string) []string {
	var statements []string
	var builder strings.Builder

	var inSingle, inDouble, inBacktick bool
	var inLineComment, inBlockComment bool

	runes := []rune(cypher)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		var next rune
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if inLineComment {
			builder.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			builder.WriteRune(r)
			if r == '*' && next == '/' {
				builder.WriteRune(next)
				i++
				inBlockComment = false
			}
			continue
		}
		if inSingle || inDouble || inBacktick {
			builder.WriteRune(r)
			if (inSingle && r == '\'') || (inDouble && r == '"') || (inBacktick && r == '`') {
				inSingle, inDouble, inBacktick = false, false, false
			}
			continue
		}

		switch {
		case r == '/' && next == '/':
			inLineComment = true
			builder.WriteRune(r)
			builder.WriteRune(next)
			i++
		case r == '/' && next == '*':
			inBlockComment = true
			builder.WriteRune(r)
			builder.WriteRune(next)
			i++
		case r == '\'':
			inSingle = true
			builder.WriteRune(r)
		case r == '"':
			inDouble = true
			builder.WriteRune(r)
		case r == '`':
			inBacktick = true
			builder.WriteRune(r)
		case r == ';':
			if statement := strings.TrimSpace(builder.String()); statement != "" {
				statements = append(statements, statement)
			}
			builder.Reset()
		default:
			builder.WriteRune(r)
		}
	}
	if statement := strings.TrimSpace(builder.String()); statement != "" {
		statements = append(statements, statement)
	}
	return statements
}
