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
	"errors"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/graph"
	"github.com/sqlc-dev/pqtype"
)

// ErrGraphSyncFailed wraps a Neo4j sync failure that happened after this Enrollment's Postgres
// write already committed. The Result Ingest returns alongside it is still valid and the Postgres
// write is not rolled back -- Neo4j MERGE is idempotent, so a later graph.SyncIdentity backfill
// run heals whatever this call missed. Callers that pass WithNeo4jDriver should check
// errors.Is(err, ErrGraphSyncFailed) to tell this apart from a Postgres-level failure (which never
// returns a usable Result).
var ErrGraphSyncFailed = errors.New("identity: graph sync failed")

// Option configures optional Ingest behavior.
type Option func(*ingestOptions)

type ingestOptions struct {
	neo4jDriver neo4j.DriverWithContext
	enqueuer    embedding.Enqueuer
}

// WithNeo4jDriver makes Ingest MERGE this Enrollment's Person -> Identification ->
// IdentityRegister -> KNOWN BiometricFeature chain into Neo4j in the same call, right after the
// Postgres transaction commits. The sync is scoped to just this Enrollment (via
// graph.SyncIdentityRows), not a full-table resync, so it stays cheap regardless of how much
// identity data already exists. Omitting this option (the default) skips Neo4j entirely, matching
// every existing caller's behavior unchanged.
func WithNeo4jDriver(driver neo4j.DriverWithContext) Option {
	return func(o *ingestOptions) { o.neo4jDriver = driver }
}

// WithEnqueuer makes Ingest enqueue processing for every biometricfeature this
// Enrollment's files produce, right after the Postgres transaction commits --
// the KNOWN-side counterpart to how cases.CreateTraces/enqueueCodificationTasks
// enqueue it for QUESTIONED case traces. A FACE_RECORD feature gets face
// embedding (embedding.ComputeForIdentityFeature); a FINGERPRINT_TEMPLATE
// feature gets template extraction (fingerprint.ExtractForIdentityFeature).
// Omitting this option (the default) still writes the biometricfeature row,
// just without anything ever populating feature_embeddings/biometric_templates
// for it, so person.SearchByFace and the fingerprint match path have nothing
// to match against.
func WithEnqueuer(queue embedding.Enqueuer) Option {
	return func(o *ingestOptions) { o.enqueuer = queue }
}

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
func Ingest(ctx context.Context, sqlDB *sql.DB, e Enrollment, opts ...Option) (Result, error) {
	var o ingestOptions
	for _, opt := range opts {
		opt(&o)
	}

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

	if o.enqueuer != nil {
		for _, feat := range result.Features {
			var task *asynq.Task
			var err error
			switch feat.FeatureType {
			case graph.FeatureTypeFaceRecord:
				task, err = embedding.NewComputeIdentityFeatureTask(feat.FeatureID)
			case graph.FeatureTypeFingerprintTemplate:
				task, err = fingerprint.NewExtractIdentityFeatureTask(feat.FeatureID)
			default:
				continue
			}
			if err != nil {
				log.Printf("identity: build task for biometricfeature %d: %v", feat.FeatureID, err)
				continue
			}
			if _, err := o.enqueuer.Enqueue(task); err != nil {
				log.Printf("identity: enqueue %s for biometricfeature %d: %v", task.Type(), feat.FeatureID, err)
			}
		}
	}

	if o.neo4jDriver != nil {
		if err := syncToGraph(ctx, o.neo4jDriver, e, result); err != nil {
			return result, fmt.Errorf("%w: %v", ErrGraphSyncFailed, err)
		}
	}
	return result, nil
}

// syncToGraph MERGEs e/result's KNOWN chain into Neo4j via graph.SyncIdentityRows, built entirely
// from data Ingest already has -- no re-querying Postgres. Only files that yielded a
// biometricfeature (result.Features) become chain rows, matching graph.SyncIdentity's own
// biometricfeature-driven join; a register with no KNOWN-yielding files yet still gets its Person
// node merged (SyncIdentityRows handles the zero-rows case).
func syncToGraph(ctx context.Context, driver neo4j.DriverWithContext, e Enrollment, result Result) error {
	filesByType := make(map[string]FileInput, len(e.Files))
	for _, f := range e.Files {
		filesByType[f.FileType] = f
	}

	rows := make([]graph.IdentityChainParams, 0, len(result.Features))
	for _, feat := range result.Features {
		f := filesByType[feat.FileType]
		rows = append(rows, graph.IdentityChainParams{
			DocumentID:     result.DocumentID,
			DocumentNumber: e.DocumentNumber,
			DocumentType:   e.DocumentType,
			FiscalNumber:   e.FiscalNumber,
			RegisterID:     result.RegisterID,
			RegisterNumber: e.RegisterNumber,
			Name:           e.Name,
			Parent1Name:    e.Parent1Name,
			Parent1Gender:  e.Parent1Gender,
			Parent2Name:    e.Parent2Name,
			Parent2Gender:  e.Parent2Gender,
			BirthDate:      e.BirthDate,
			IdentityFileID: result.IdentityFileIDs[feat.FileType],
			FeatureType:    feat.FeatureType,
			SourcePath:     f.SourcePath,
			StorageRef:     f.StorageRef,
			ContentType:    f.ContentType,
			SizeBytes:      f.SizeBytes,
		})
	}
	return graph.SyncIdentityRows(ctx, driver, e.PersonID, rows)
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
