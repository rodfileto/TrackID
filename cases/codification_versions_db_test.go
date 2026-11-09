package cases_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/embedding"
)

// vec is a 512-dimension embedding literal with v in its first component.
func vec(v string) string {
	parts := make([]string, 512)
	for i := range parts {
		parts[i] = "0"
	}
	parts[0] = v
	return "[" + strings.Join(parts, ",") + "]"
}

// A face's versions: the original is the one the matcher uses until another is chosen; choosing a
// version with an embedding makes it the face's (to be matched again); the original and the chosen
// version cannot be deleted, others can.
func TestCodificationVersions(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	exec := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRow(query, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}
	bc := exec(`INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ('2026.0700', 'CRIMINAL', 'FACIAL', 'x') RETURNING id`)
	file := exec(`INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename, content_type) VALUES ($1, 'evidence', 'image', 'h1', 'f.png', 'image/png') RETURNING id`, bc)
	traces, err := cases.CreateTraces(ctx, sqlDB, nil, "2026.0700", file, []cases.TraceDetection{{BoxX1: 10, BoxY1: 10, BoxX2: 60, BoxY2: 70, Score: 0.9}})
	if err != nil {
		t.Fatal(err)
	}
	traceID := traces[0].ID
	feature := exec(`SELECT id FROM biometric_features WHERE case_trace_id = $1`, traceID)
	original := exec(`SELECT id FROM case_codifications WHERE trace_id = $1 AND sequence = 1`, traceID)
	version := exec(`INSERT INTO case_codifications (trace_id, sequence, codification_type, label, recipe) VALUES ($1, 2, 'FACE_EMBEDDING', 'mirrored', '{"version":1,"mirror":true}') RETURNING id`, traceID)
	exec(`INSERT INTO codification_embeddings (codification_id, embedding_type, embedding) VALUES ($1, $2, $3::vector) RETURNING id`, version, embedding.EmbeddingType, vec("1"))
	exec(`INSERT INTO feature_embeddings (biometric_feature_id, embedding_type, embedding, matched_at) VALUES ($1, $2, $3::vector, NOW()) RETURNING id`, feature, embedding.EmbeddingType, vec("0.5"))

	versions, err := cases.ListCodificationVersions(ctx, sqlDB, "2026.0700", file, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || !versions[0].ForRecognition || versions[1].ForRecognition || versions[1].Recipe == nil || !versions[1].Recipe.Mirror || !versions[1].HasEmbedding {
		t.Fatalf("versions = %+v", versions)
	}

	if err := cases.DeleteCodificationVersion(ctx, sqlDB, "2026.0700", file, traceID, original); !errors.Is(err, cases.ErrVersionInUse) {
		t.Fatalf("deleting the original: %v", err)
	}
	if err := cases.SetRecognitionVersion(ctx, sqlDB, nil, "2026.0700", file, traceID, version); err != nil {
		t.Fatal(err)
	}
	var first string
	var matched sql.NullTime
	if err := sqlDB.QueryRow(`SELECT split_part(trim(both '[]' from embedding::text), ',', 1), matched_at FROM feature_embeddings WHERE biometric_feature_id = $1`, feature).Scan(&first, &matched); err != nil {
		t.Fatal(err)
	}
	if first != "1" || matched.Valid {
		t.Fatalf("the face's embedding is not the chosen version's, ready to match again: first=%s matched=%v", first, matched)
	}
	if err := cases.DeleteCodificationVersion(ctx, sqlDB, "2026.0700", file, traceID, version); !errors.Is(err, cases.ErrVersionInUse) {
		t.Fatalf("deleting the version in use: %v", err)
	}
	all, err := cases.ListCaseCodifications(ctx, sqlDB, "2026.0700")
	if err != nil || len(all) != 2 || all[0].ForRecognition || !all[1].ForRecognition || all[1].Label != "mirrored" {
		t.Fatalf("case codifications = %+v, %v", all, err)
	}
	if err := cases.SetRecognitionVersion(ctx, sqlDB, nil, "2026.0700", file, traceID, original); err != nil {
		t.Fatal(err)
	}
	if err := cases.DeleteCodificationVersion(ctx, sqlDB, "2026.0700", file, traceID, version); err != nil {
		t.Fatalf("deleting a version no longer used: %v", err)
	}
	if err := cases.SetRecognitionVersion(ctx, sqlDB, nil, "2026.0700", file, traceID, 999999); !errors.Is(err, cases.ErrVersionNotFound) {
		t.Fatalf("an unknown version: %v", err)
	}
}
