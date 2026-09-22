package cases_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
)

// testDB creates a throwaway database on the server named by TRACKID_TEST_DATABASE_URL (a role
// allowed to CREATE DATABASE), migrates it, and drops it when the test ends. The test is
// skipped when the variable is unset. Mirrors biometricmatch/match_db_test.go's helper of the
// same name; there is no shared testutil package yet.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	adminURL := os.Getenv("TRACKID_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TRACKID_TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })

	name := fmt.Sprintf("trackid_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`) })

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	sqlDB, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

// TestIngestEvidenceFileAndTraceBox is C2's regression: an evidence image and a trace box
// attach on Ingest and survive a re-ingest (the upsert path) without duplicating rows, and the
// result is exactly what embedding.ComputeForCodification reads via GetCodificationSource --
// the DB-only half of that "done when", since exercising ComputeForCodification itself needs a
// running storage backend and vision model, left to an integration test.
func TestIngestEvidenceFileAndTraceBox(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)

	in := cases.CaseInput{
		CaseID:   "TEST-C2-1",
		CaseType: "FACIAL",
		Evidences: []cases.EvidenceInput{{
			Sequence: 1,
			File: &cases.FileInput{
				StorageRef:  "s3://evidence/test-c2-1/frame.jpg",
				ContentType: "image/jpeg",
				SizeBytes:   1234,
				HashID:      "deadbeef",
				Filename:    "frame.jpg",
			},
			Traces: []cases.TraceInput{{
				Sequence:  1,
				TraceType: "FACE_RECORD",
				Box:       &cases.Box{X1: 10, Y1: 20, X2: 110, Y2: 140, DetectionScore: 0.93},
				Codifications: []cases.CodificationInput{{
					Sequence:         1,
					CodificationType: "FACE_EMBEDDING",
				}},
			}},
		}},
	}

	for attempt := 1; attempt <= 2; attempt++ {
		res, err := cases.Ingest(ctx, sqlDB, in)
		if err != nil {
			t.Fatalf("attempt %d: Ingest: %v", attempt, err)
		}
		if len(res.Evidences) != 1 || len(res.Evidences[0].Traces) != 1 {
			t.Fatalf("attempt %d: got %d evidences, %d traces on the first; want 1, 1",
				attempt, len(res.Evidences), len(res.Evidences[0].Traces))
		}

		var evidenceCount, traceCount, fileCount int
		if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM case_evidences`).Scan(&evidenceCount); err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM case_traces`).Scan(&traceCount); err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM case_files WHERE category = 'evidence'`).Scan(&fileCount); err != nil {
			t.Fatal(err)
		}
		if evidenceCount != 1 || traceCount != 1 || fileCount != 1 {
			t.Fatalf("attempt %d: rows = %d case_evidences, %d case_traces, %d evidence case_files; want 1, 1, 1",
				attempt, evidenceCount, traceCount, fileCount)
		}

		codificationID := res.Evidences[0].Traces[0].Codifications[0].CodificationID
		source, err := db.New(sqlDB).GetCodificationSource(ctx, codificationID)
		if err != nil {
			t.Fatalf("attempt %d: GetCodificationSource: %v", attempt, err)
		}
		if !source.EvidenceStorageRef.Valid || source.EvidenceStorageRef.String != in.Evidences[0].File.StorageRef {
			t.Fatalf("attempt %d: EvidenceStorageRef = %+v, want %q", attempt, source.EvidenceStorageRef, in.Evidences[0].File.StorageRef)
		}
		wantBox := in.Evidences[0].Traces[0].Box
		if !source.BoxX1.Valid || source.BoxX1.Float64 != wantBox.X1 ||
			!source.BoxX2.Valid || source.BoxX2.Float64 != wantBox.X2 {
			t.Fatalf("attempt %d: box = (%+v, .., %+v, ..), want (%v, .., %v, ..)",
				attempt, source.BoxX1, source.BoxX2, wantBox.X1, wantBox.X2)
		}
		// This is exactly the input ComputeForCodification needs (embedding/embedding.go):
		// storageRef, box := row.CodificationStorageRef, image.Rectangle{}; falls back to
		// TraceCropStorageRef, then EvidenceStorageRef+box, since neither of the first two is
		// set here.
		if source.CodificationStorageRef.Valid || source.TraceCropStorageRef.Valid {
			t.Fatalf("attempt %d: expected no codification/crop image, got %+v / %+v",
				attempt, source.CodificationStorageRef, source.TraceCropStorageRef)
		}
	}
}
