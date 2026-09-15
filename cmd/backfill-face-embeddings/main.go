// backfill-face-embeddings populates biometricfeature and feature_embeddings from the
// legacy pre-refactor face_embeddings table (see db/migrations/016's comment and
// MODEL.md section 2.2) -- schema drift on some local databases left
// biometricfeature/feature_embeddings empty even though face_embeddings already has a
// real vector for every FACE_RECORD case_trace and "photo" identity_file.
//
// face_embeddings predates this repo's migration history (it isn't created by any file
// under db/migrations, only found on databases seeded from a pre-refactor snapshot), so
// it's read with a plain query here rather than a generated one. Every write goes
// through the same upsert queries the live ingest/identity code paths already use, so
// this is safe to re-run.
//
// Legacy vectors were produced by an "auraface" model, a different (not
// cosine-comparable) space from FACE_ARCFACE_512, so they're written under their own
// embedding_type: FACE_AURAFACE_512 (see migration 017). Run cmd/match-embeddings
// -embedding-type=FACE_AURAFACE_512 afterward to mark them matched; it's dedup-safe
// against decisions the legacy system already produced (see biometricmatch.Run).
//
// Defaults to a dry run that only reports counts. Pass -commit to write.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

// embeddingType and modelVersion describe the legacy vectors this tool backfills.
// face_embeddings has only ever held one model's output (see the investigation in
// MODEL.md / the migration 017 comment), so these aren't read per-row.
const (
	embeddingType = "FACE_AURAFACE_512"
	modelVersion  = "auraface:v1"
	batchSize     = 200
)

// legacyRow is one face_embeddings row: exactly one of caseTraceID/identityFileID is
// set, matching that table's own one-subject CHECK constraint. embedding is pgvector's
// text form ("[v1,v2,...]"), read via ::text so no vector-aware driver is needed.
type legacyRow struct {
	id             int64
	caseTraceID    sql.NullInt64
	identityFileID sql.NullInt64
	embedding      string
}

func main() {
	env.Load()

	commit := cmdutil.CommitFlag()
	flag.Parse()

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()

	ctx := context.Background()
	rows, err := loadLegacyRows(ctx, sqlDB)
	cmdutil.Fatal(err)

	questioned, known := 0, 0
	for _, r := range rows {
		if r.caseTraceID.Valid {
			questioned++
		} else {
			known++
		}
	}

	if !*commit {
		log.Printf("dry run: %d legacy face_embeddings row(s) would be backfilled (%d QUESTIONED case_trace, %d KNOWN identity_file)",
			len(rows), questioned, known)
		return
	}

	written := 0
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		n, err := writeBatch(ctx, sqlDB, rows[start:end])
		cmdutil.Fatal(err)
		written += n
		log.Printf("backfilled %d/%d", written, len(rows))
	}
	log.Printf("backfill complete: %d row(s) written (%d QUESTIONED case_trace, %d KNOWN identity_file)", written, questioned, known)
}

func loadLegacyRows(ctx context.Context, sqlDB *sql.DB) ([]legacyRow, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, case_trace_id, identity_file_id, embedding::text
		FROM face_embeddings
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("backfill: query face_embeddings: %w", err)
	}
	defer rows.Close()

	var out []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.caseTraceID, &r.identityFileID, &r.embedding); err != nil {
			return nil, fmt.Errorf("backfill: scan face_embeddings: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// writeBatch upserts one batch of legacyRow into biometricfeature + feature_embeddings
// in a single transaction, mirroring cases/trace.go and identity/identity.go's own
// per-row upsert pattern.
func writeBatch(ctx context.Context, sqlDB *sql.DB, batch []legacyRow) (int, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	q := db.New(tx)
	for _, r := range batch {
		var featureID int64
		var err error
		switch {
		case r.caseTraceID.Valid:
			featureID, err = q.UpsertBiometricFeatureFromCaseTrace(ctx, db.UpsertBiometricFeatureFromCaseTraceParams{
				FeatureType: graph.FeatureTypeFaceCapture,
				Provenance:  "QUESTIONED",
				CaseTraceID: r.caseTraceID,
			})
		case r.identityFileID.Valid:
			featureID, err = q.UpsertBiometricFeatureFromIdentityFile(ctx, db.UpsertBiometricFeatureFromIdentityFileParams{
				FeatureType:    graph.FeatureTypeFaceRecord,
				Provenance:     "KNOWN",
				IdentityFileID: r.identityFileID,
			})
		default:
			return 0, fmt.Errorf("backfill: face_embeddings row %d has neither case_trace_id nor identity_file_id", r.id)
		}
		if err != nil {
			return 0, fmt.Errorf("backfill: upsert biometricfeature (face_embeddings %d): %w", r.id, err)
		}

		if _, err := q.UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{
			BiometricfeatureID: featureID,
			EmbeddingType:      embeddingType,
			Embedding:          r.embedding,
			ModelVersion:       sql.NullString{String: modelVersion, Valid: true},
		}); err != nil {
			return 0, fmt.Errorf("backfill: upsert feature_embeddings (face_embeddings %d): %w", r.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(batch), nil
}
