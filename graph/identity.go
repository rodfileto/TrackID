package graph

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/db"
)

// IdentitySyncStats reports what SyncIdentity materialized.
type IdentitySyncStats struct {
	Persons  int
	Features int
}

// SyncIdentity materializes Person nodes from the person table, and the full
// KNOWN identity chain — Identification, IdentityRegister, and KNOWN
// BiometricFeature nodes — from the biometricfeature table. It reads only from
// Postgres, rescanning every row every call; for materializing just the rows
// one identity.Ingest call already wrote, use SyncIdentityRows instead.
func SyncIdentity(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext) (IdentitySyncStats, error) {
	persons, chain, err := loadIdentityRows(ctx, sqlDB)
	if err != nil {
		return IdentitySyncStats{}, err
	}

	if err := writeIdentityRows(ctx, driver, persons, chain); err != nil {
		return IdentitySyncStats{}, err
	}
	return IdentitySyncStats{Persons: len(persons), Features: len(chain)}, nil
}

// IdentityChainParams is one row of the KNOWN identity chain, ready to MERGE into Neo4j. It
// mirrors what SyncIdentity reads back from Postgres, but is meant to be built directly from data
// a caller already has in hand (e.g. identity.Ingest's own Enrollment/Result) via
// SyncIdentityRows, without a round trip through Postgres.
type IdentityChainParams struct {
	PersonID string

	DocumentID     int64
	DocumentNumber string
	DocumentType   string
	FiscalNumber   string // empty = not set

	RegisterID     int64
	RegisterNumber string
	Name           string
	Parent1Name    string
	Parent1Gender  string
	Parent2Name    string
	Parent2Gender  string
	BirthDate      string // empty = not set

	IdentityFileID int64
	FeatureType    string
	SourcePath     string
	StorageRef     string
	ContentType    string // empty = not set
	SizeBytes      int64  // 0 = not set
}

// SyncIdentityRows MERGEs personID's Person node and, for each row, the
// Identification -> IdentityRegister -> KNOWN BiometricFeature chain it describes. Unlike
// SyncIdentity it never queries Postgres, so it stays O(len(rows)) per call regardless of how much
// identity data already exists -- the shape identity.Ingest needs to sync just the one Enrollment
// it wrote. Safe to call with zero rows (a register with no KNOWN-yielding files yet still gets
// its Person node merged).
func SyncIdentityRows(ctx context.Context, driver neo4j.DriverWithContext, personID string, rows []IdentityChainParams) error {
	chain := make([]map[string]any, len(rows))
	for i, r := range rows {
		chain[i] = identityChainRowParams(r, personID)
	}
	return writeIdentityRows(ctx, driver, []map[string]any{{"personId": personID}}, chain)
}

func writeIdentityRows(ctx context.Context, driver neo4j.DriverWithContext, persons, chain []map[string]any) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if len(persons) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncIdentityQuery, map[string]any{"persons": persons})
			return nil, err
		}); err != nil {
			return fmt.Errorf("materialize persons: %w", err)
		}
	}
	if len(chain) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncIdentityChainQuery, map[string]any{"rows": chain})
			return nil, err
		}); err != nil {
			return fmt.Errorf("materialize identity chain: %w", err)
		}
	}
	return nil
}

// SyncIdentityPlan returns the counts SyncIdentity would materialize.
func SyncIdentityPlan(ctx context.Context, sqlDB *sql.DB) (IdentitySyncStats, error) {
	persons, chain, err := loadIdentityRows(ctx, sqlDB)
	if err != nil {
		return IdentitySyncStats{}, err
	}
	return IdentitySyncStats{Persons: len(persons), Features: len(chain)}, nil
}

func loadIdentityRows(ctx context.Context, sqlDB *sql.DB) ([]map[string]any, []map[string]any, error) {
	if sqlDB == nil {
		return nil, nil, fmt.Errorf("database is not configured")
	}

	persons, err := loadPersons(ctx, sqlDB)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.New(sqlDB).ListKnownIdentityChain(ctx)
	if err != nil {
		return nil, nil, err
	}

	chain := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		chain = append(chain, identityChainRowParams(identityParamsFromRow(r), r.PersonID))
	}
	return persons, chain, nil
}

// identityParamsFromRow converts a ListKnownIdentityChainRow (the sqlc view of
// the KNOWN identity chain) into the IdentityChainParams shape the Cypher
// builder expects. Empty string / zero fields become "not set", matching the
// IdentityChainParams contract.
func identityParamsFromRow(r db.ListKnownIdentityChainRow) IdentityChainParams {
	return IdentityChainParams{
		DocumentID:     r.DocumentID,
		DocumentNumber: r.DocumentNumber,
		DocumentType:   r.DocumentType,
		FiscalNumber:   r.FiscalNumber.String,
		RegisterID:     r.RegisterID,
		RegisterNumber: r.RegisterNumber,
		Name:           r.Name,
		Parent1Name:    r.Parent1Name,
		Parent1Gender:  r.Parent1Gender,
		Parent2Name:    r.Parent2Name,
		Parent2Gender:  r.Parent2Gender,
		BirthDate:      r.BirthDate.String,
		IdentityFileID: r.IdentityFileID,
		FeatureType:    r.FeatureType,
		SourcePath:     r.SourcePath,
		StorageRef:     r.StorageRef,
		ContentType:    r.ContentType.String,
		SizeBytes:      r.SizeBytes.Int64,
	}
}

// identityChainRowParams builds syncIdentityChainQuery's per-row params. Empty string/zero fields
// map to Cypher null (not the empty value) so an absent optional column stays absent in Neo4j too,
// matching what the sql.Null* -> map[string]any conversion in identityParamsFromRow already did.
func identityChainRowParams(r IdentityChainParams, personID string) map[string]any {
	var fiscalNumber, birthDate, contentType any
	var sizeBytes any
	if r.FiscalNumber != "" {
		fiscalNumber = r.FiscalNumber
	}
	if r.BirthDate != "" {
		birthDate = r.BirthDate
	}
	if r.ContentType != "" {
		contentType = r.ContentType
	}
	if r.SizeBytes != 0 {
		sizeBytes = r.SizeBytes
	}
	return map[string]any{
		"personId":       personID,
		"documentId":     r.DocumentID,
		"documentNumber": r.DocumentNumber,
		"documentType":   r.DocumentType,
		"fiscalNumber":   fiscalNumber,
		"registerId":     r.RegisterID,
		"registerNumber": r.RegisterNumber,
		"name":           r.Name,
		"parent1Name":    r.Parent1Name,
		"parent1Gender":  r.Parent1Gender,
		"parent2Name":    r.Parent2Name,
		"parent2Gender":  r.Parent2Gender,
		"birthDate":      birthDate,
		"featureId":      KnownFeatureID(r.IdentityFileID),
		"featureType":    r.FeatureType,
		"sourcePath":     r.SourcePath,
		"storageRef":     r.StorageRef,
		"contentType":    contentType,
		"sizeBytes":      sizeBytes,
	}
}

func loadPersons(ctx context.Context, sqlDB *sql.DB) ([]map[string]any, error) {
	ids, err := db.New(sqlDB).ListPersonIDs(ctx)
	if err != nil {
		return nil, err
	}
	persons := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		persons = append(persons, map[string]any{"personId": id})
	}
	return persons, nil
}

const syncIdentityQuery = `
UNWIND $persons AS person
MERGE (p:Person {personId: person.personId})
`

const syncIdentityChainQuery = `
UNWIND $rows AS row
MERGE (person:Person {personId: row.personId})

MERGE (ident:Object:Identification {documentId: row.documentId})
ON CREATE SET ident.documentNumber = row.documentNumber, ident.documentType = row.documentType,
              ident.fiscalNumber = row.fiscalNumber
MERGE (person)-[:HAS_IDENTITY]->(ident)

MERGE (reg:Object:IdentityRegister {registerId: row.registerId})
ON CREATE SET reg.registerNumber = row.registerNumber
SET reg.name = row.name, reg.parent1Name = row.parent1Name, reg.parent1Gender = row.parent1Gender,
    reg.parent2Name = row.parent2Name, reg.parent2Gender = row.parent2Gender,
    reg.birthDate = row.birthDate
MERGE (ident)-[:HAS_REGISTRATION]->(reg)

MERGE (feature:Object:BiometricFeature {featureId: row.featureId})
ON CREATE SET feature.featureType = row.featureType, feature.provenance = 'KNOWN',
              feature.sourcePath = row.sourcePath, feature.storageRef = row.storageRef,
              feature.contentType = row.contentType, feature.sizeBytes = row.sizeBytes
MERGE (reg)-[:HAS_FEATURE]->(feature)
`
