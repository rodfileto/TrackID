// Package identity is the generic identity-ingestion extension contract.
// Organizations normalize their own enrollment source (an API response, a CSV
// row, a NIST file listing, ...) into an Enrollment and call Ingest, which
// upserts the full person -> identity_document -> identity_register ->
// identity_file -> biometricfeature chain in one transaction. See MODEL.md
// section 2.1. This package has no knowledge of any organization's source
// format.
package identity

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/sqlc-dev/pqtype"
)

// FileInput is one raw file produced by an enrollment event (a photo, a NIST
// file, ...). FileType determines the KNOWN biometric feature it yields, via
// graph.FeatureTypeForFileType; file types that don't yield a feature (e.g. a
// pdf) are still stored, just without a biometricfeature row.
type FileInput struct {
	FileType    string
	Sequence    int16 // defaults to 1 when zero
	SourcePath  string
	StorageRef  string
	ContentType string
	SizeBytes   int64
}

// Enrollment is one normalized enrollment event: a person, the identity
// document it's filed under, the register (the specific enrollment event),
// and the raw files it produced.
type Enrollment struct {
	// PersonID is the stable business key for the person (person.person_id).
	PersonID   string
	PersonMeta []byte // optional JSON

	DocumentType   string
	DocumentNumber string
	FiscalNumber   string // optional; any government-issued taxpayer/fiscal id (e.g. Brazil's CPF)

	RegisterNumber string
	Name           string
	Parent1Name    string
	Parent1Gender  string
	Parent2Name    string
	Parent2Gender  string
	BirthDate      string // optional
	RegisterMeta   []byte // optional JSON

	Files []FileInput
}

// FeatureResult is one biometricfeature row produced from an Enrollment's
// files.
type FeatureResult struct {
	FileType    string
	FeatureType string
	FeatureID   int64
}

// Result is the set of rows Ingest wrote for one Enrollment.
type Result struct {
	PersonID        int64
	DocumentID      int64
	RegisterID      int64
	IdentityFileIDs map[string]int64 // keyed by FileType
	Features        []FeatureResult
}

// Ingest upserts one Enrollment's full chain in a single transaction: person,
// identity_document, identity_register, identity_file (one per Files entry),
// and the biometricfeature row for each file type that yields one.
func Ingest(ctx context.Context, sqlDB *sql.DB, e Enrollment) (Result, error) {
	if sqlDB == nil {
		return Result{}, fmt.Errorf("identity: nil db")
	}
	if e.PersonID == "" {
		return Result{}, fmt.Errorf("identity: PersonID is required")
	}
	if e.DocumentType == "" || e.DocumentNumber == "" {
		return Result{}, fmt.Errorf("identity: DocumentType and DocumentNumber are required")
	}
	if e.RegisterNumber == "" {
		return Result{}, fmt.Errorf("identity: RegisterNumber is required")
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()

	q := db.New(tx)

	personID, err := q.UpsertPerson(ctx, db.UpsertPersonParams{
		PersonID: e.PersonID,
		Meta:     rawMessage(e.PersonMeta),
	})
	if err != nil {
		return Result{}, fmt.Errorf("identity: upsert person: %w", err)
	}

	documentID, err := q.UpsertIdentityDocument(ctx, db.UpsertIdentityDocumentParams{
		PersonID:       sql.NullInt64{Int64: personID, Valid: true},
		DocumentNumber: e.DocumentNumber,
		DocumentType:   e.DocumentType,
		FiscalNumber:   nullString(e.FiscalNumber),
	})
	if err != nil {
		return Result{}, fmt.Errorf("identity: upsert identity_document: %w", err)
	}

	registerID, err := q.UpsertIdentityRegister(ctx, db.UpsertIdentityRegisterParams{
		DocumentID:     documentID,
		RegisterNumber: e.RegisterNumber,
		Name:           e.Name,
		Parent1Name:    e.Parent1Name,
		Parent1Gender:  e.Parent1Gender,
		Parent2Name:    e.Parent2Name,
		Parent2Gender:  e.Parent2Gender,
		BirthDate:      nullString(e.BirthDate),
		Meta:           rawMessage(e.RegisterMeta),
	})
	if err != nil {
		return Result{}, fmt.Errorf("identity: upsert identity_register: %w", err)
	}

	result := Result{
		PersonID:        personID,
		DocumentID:      documentID,
		RegisterID:      registerID,
		IdentityFileIDs: make(map[string]int64, len(e.Files)),
	}

	for _, f := range e.Files {
		seq := f.Sequence
		if seq == 0 {
			seq = 1
		}
		fileID, err := q.UpsertIdentityFile(ctx, db.UpsertIdentityFileParams{
			RegisterID:  registerID,
			FileType:    f.FileType,
			Sequence:    seq,
			SourcePath:  f.SourcePath,
			StorageRef:  f.StorageRef,
			ContentType: nullString(f.ContentType),
			SizeBytes:   nullInt64(f.SizeBytes),
		})
		if err != nil {
			return Result{}, fmt.Errorf("identity: upsert identity_file (%s): %w", f.FileType, err)
		}
		result.IdentityFileIDs[f.FileType] = fileID

		featureType, ok := graph.FeatureTypeForFileType(f.FileType)
		if !ok {
			continue
		}
		featureID, err := q.UpsertBiometricFeatureFromIdentityFile(ctx, db.UpsertBiometricFeatureFromIdentityFileParams{
			FeatureType:    featureType,
			Provenance:     "KNOWN",
			IdentityFileID: sql.NullInt64{Int64: fileID, Valid: true},
		})
		if err != nil {
			return Result{}, fmt.Errorf("identity: upsert biometricfeature (%s): %w", f.FileType, err)
		}
		result.Features = append(result.Features, FeatureResult{
			FileType:    f.FileType,
			FeatureType: featureType,
			FeatureID:   featureID,
		})
	}

	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullInt64(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: n != 0}
}

func rawMessage(b []byte) pqtype.NullRawMessage {
	return pqtype.NullRawMessage{RawMessage: b, Valid: len(b) > 0}
}
