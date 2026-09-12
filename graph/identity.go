package graph

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// IdentitySyncStats reports what SyncIdentity materialized.
type IdentitySyncStats struct {
	Persons  int
	Features int
}

// knownIdentityRow is one KNOWN biometricfeature joined up through
// identity_file -> identity_register -> identity_document -> person. A register
// can yield zero, one, or both feature types (photo -> FACE_RECORD, nist ->
// FINGERPRINT_TEMPLATE).
type knownIdentityRow struct {
	personID string

	documentID     int64
	documentNumber string
	documentType   string
	cpf            sql.NullString

	registerID       int64
	registerNumber   string
	name             string
	parent1Name      string
	parent1Gender    string
	parent2Name      string
	parent2Gender    string
	dataNascimento   sql.NullString

	identityFileID int64
	featureType    string
	sourcePath     string
	storageRef     string
	contentType    sql.NullString
	sizeBytes      sql.NullInt64
}

// SyncIdentity materializes Person nodes from the person table, and the full
// KNOWN identity chain — Identification, IdentityRegister, and KNOWN
// BiometricFeature nodes — from the biometricfeature table. It reads only from
// Postgres.
func SyncIdentity(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (IdentitySyncStats, error) {
	persons, chain, err := loadIdentityRows(ctx, db)
	if err != nil {
		return IdentitySyncStats{}, err
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if len(persons) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncIdentityQuery, map[string]any{"persons": persons})
			return nil, err
		}); err != nil {
			return IdentitySyncStats{}, fmt.Errorf("materialize persons: %w", err)
		}
	}
	if len(chain) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncIdentityChainQuery, map[string]any{"rows": chain})
			return nil, err
		}); err != nil {
			return IdentitySyncStats{}, fmt.Errorf("materialize identity chain: %w", err)
		}
	}
	return IdentitySyncStats{Persons: len(persons), Features: len(chain)}, nil
}

// SyncIdentityPlan returns the counts SyncIdentity would materialize.
func SyncIdentityPlan(ctx context.Context, db *sql.DB) (IdentitySyncStats, error) {
	persons, chain, err := loadIdentityRows(ctx, db)
	if err != nil {
		return IdentitySyncStats{}, err
	}
	return IdentitySyncStats{Persons: len(persons), Features: len(chain)}, nil
}

func loadIdentityRows(ctx context.Context, db *sql.DB) ([]map[string]any, []map[string]any, error) {
	if db == nil {
		return nil, nil, fmt.Errorf("database is not configured")
	}

	persons, err := loadPersons(ctx, db)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.QueryContext(ctx, knownIdentityChainQuery)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var chain []map[string]any
	for rows.Next() {
		var r knownIdentityRow
		if err := rows.Scan(
			&r.personID,
			&r.documentID, &r.documentNumber, &r.documentType, &r.cpf,
			&r.registerID, &r.registerNumber, &r.name,
			&r.parent1Name, &r.parent1Gender, &r.parent2Name, &r.parent2Gender,
			&r.dataNascimento,
			&r.identityFileID, &r.featureType, &r.sourcePath, &r.storageRef,
			&r.contentType, &r.sizeBytes,
		); err != nil {
			return nil, nil, err
		}
		chain = append(chain, identityRowParams(r))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return persons, chain, nil
}

func identityRowParams(r knownIdentityRow) map[string]any {
	var cpf, dataNascimento, contentType any
	var sizeBytes any
	if r.cpf.Valid {
		cpf = r.cpf.String
	}
	if r.dataNascimento.Valid {
		dataNascimento = r.dataNascimento.String
	}
	if r.contentType.Valid {
		contentType = r.contentType.String
	}
	if r.sizeBytes.Valid {
		sizeBytes = r.sizeBytes.Int64
	}
	return map[string]any{
		"personId":        r.personID,
		"documentId":      r.documentID,
		"documentNumber":  r.documentNumber,
		"documentType":    r.documentType,
		"cpf":             cpf,
		"registerId":      r.registerID,
		"registerNumber":  r.registerNumber,
		"name":            r.name,
		"parent1Name":     r.parent1Name,
		"parent1Gender":   r.parent1Gender,
		"parent2Name":     r.parent2Name,
		"parent2Gender":   r.parent2Gender,
		"dataNascimento":  dataNascimento,
		"featureId":       KnownFeatureID(r.identityFileID),
		"featureType":     r.featureType,
		"sourcePath":      r.sourcePath,
		"storageRef":      r.storageRef,
		"contentType":     contentType,
		"sizeBytes":       sizeBytes,
	}
}

func loadPersons(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
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

const knownIdentityChainQuery = `
SELECT
    p.person_id,
    d.id, d.document_number, d.document_type, d.cpf,
    r.id, r.register_number, r.name,
    r.parent_1_name, r.parent_1_gender, r.parent_2_name, r.parent_2_gender,
    r.data_nascimento,
    f.id, bf.feature_type, f.source_path, f.storage_ref, f.content_type, f.size_bytes
FROM biometricfeature bf
JOIN identity_file f ON f.id = bf.identity_file_id
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE bf.provenance = 'KNOWN'
ORDER BY bf.id
`

const syncIdentityQuery = `
UNWIND $persons AS person
MERGE (p:Person {personId: person.personId})
`

const syncIdentityChainQuery = `
UNWIND $rows AS row
MERGE (person:Person {personId: row.personId})

MERGE (ident:Object:Identification {documentId: row.documentId})
ON CREATE SET ident.documentNumber = row.documentNumber, ident.documentType = row.documentType,
              ident.cpf = row.cpf
MERGE (person)-[:HAS_IDENTITY]->(ident)

MERGE (reg:Object:IdentityRegister {registerId: row.registerId})
ON CREATE SET reg.registerNumber = row.registerNumber
SET reg.name = row.name, reg.parent1Name = row.parent1Name, reg.parent1Gender = row.parent1Gender,
    reg.parent2Name = row.parent2Name, reg.parent2Gender = row.parent2Gender,
    reg.dataNascimento = row.dataNascimento
MERGE (ident)-[:HAS_REGISTRATION]->(reg)

MERGE (feature:Object:BiometricFeature {featureId: row.featureId})
ON CREATE SET feature.featureType = row.featureType, feature.provenance = 'KNOWN',
              feature.sourcePath = row.sourcePath, feature.storageRef = row.storageRef,
              feature.contentType = row.contentType, feature.sizeBytes = row.sizeBytes
MERGE (reg)-[:HAS_FEATURE]->(feature)
`
