package graph

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// SyncIdentity materializes Person nodes from the person table. It reads only
// from Postgres.
func SyncIdentity(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (int, error) {
	persons, err := loadPersons(ctx, db)
	if err != nil {
		return 0, err
	}
	if len(persons) == 0 {
		return 0, nil
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, syncIdentityQuery, map[string]any{"persons": persons})
		return nil, err
	}); err != nil {
		return 0, fmt.Errorf("materialize persons: %w", err)
	}
	return len(persons), nil
}

// SyncIdentityPlan returns how many persons SyncIdentity would materialize.
func SyncIdentityPlan(ctx context.Context, db *sql.DB) (int, error) {
	persons, err := loadPersons(ctx, db)
	return len(persons), err
}

func loadPersons(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	rows, err := db.QueryContext(ctx, `SELECT person_id FROM person ORDER BY person_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var persons []map[string]any
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		persons = append(persons, map[string]any{"personId": id})
	}
	return persons, rows.Err()
}

const syncIdentityQuery = `
UNWIND $persons AS person
MERGE (p:Person {personId: person.personId})
`
