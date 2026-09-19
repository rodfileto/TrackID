package biometricmatch_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/graph"
)

// testDB creates a throwaway en_US-collated database on the server named by
// TRACKID_TEST_DATABASE_URL (a role allowed to CREATE DATABASE), migrates it,
// and drops it when the test ends. The test is skipped when the variable is unset.
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
	// en_US is the collation that disagrees with byte order on graph feature ids;
	// template0 is required to pick a collation other than the server default.
	if _, err := admin.Exec(fmt.Sprintf(
		`CREATE DATABASE %s TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'en_US.utf8' LC_CTYPE 'en_US.utf8'`, name)); err != nil {
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

// TestRunOrdersPairByBytes is the regression for the collation-dependent order
// check: "TRACE:1#feature" < "TRACE:10#feature" in bytes but not under en_US,
// so Run's Go-ordered pair used to violate biometric_decisions_feature_order_check.
func TestRunOrdersPairByBytes(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)

	// A fresh database hands out case_traces ids 1..10 in order.
	in := cases.CaseInput{CaseID: "TEST-1", CaseType: "FACIAL"}
	ev := cases.EvidenceInput{Sequence: 1}
	for s := int16(1); s <= 10; s++ {
		ev.Traces = append(ev.Traces, cases.TraceInput{Sequence: s, TraceType: "FACE_RECORD"})
	}
	in.Evidences = []cases.EvidenceInput{ev}
	res, err := cases.Ingest(ctx, sqlDB, in)
	if err != nil {
		t.Fatal(err)
	}

	vec := "[" + strings.TrimSuffix(strings.Repeat("0.1,", 512), ",") + "]"
	var a, b string
	for _, tr := range res.Evidences[0].Traces {
		if tr.TraceID != 1 && tr.TraceID != 10 {
			continue
		}
		if _, err := db.New(sqlDB).UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{
			BiometricfeatureID: tr.FeatureID,
			EmbeddingType:      "FACE_ARCFACE_512",
			Embedding:          vec,
		}); err != nil {
			t.Fatal(err)
		}
		if tr.TraceID == 1 {
			a = graph.QuestionedFeatureID(tr.TraceID)
		} else {
			b = graph.QuestionedFeatureID(tr.TraceID)
		}
	}
	if a != "TRACE:1#feature" || b != "TRACE:10#feature" {
		t.Fatalf("fixture feature ids = %q, %q; want TRACE:1#feature, TRACE:10#feature", a, b)
	}

	stats, err := biometricmatch.Run(ctx, sqlDB, "FACE_ARCFACE_512", 0.1, "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Decisions != 1 {
		t.Fatalf("Decisions = %d, want 1", stats.Decisions)
	}

	var gotA, gotB string
	if err := sqlDB.QueryRowContext(ctx, `SELECT feature_a_id, feature_b_id FROM biometric_decisions`).Scan(&gotA, &gotB); err != nil {
		t.Fatal(err)
	}
	if gotA != a || gotB != b {
		t.Fatalf("stored pair = (%q, %q), want (%q, %q)", gotA, gotB, a, b)
	}

	// The view shows the decision from both sides.
	var sides int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM biometric_decision_sides WHERE feature_id = ANY($1) AND counterpart_id = ANY($1)`,
		"{"+a+","+b+"}").Scan(&sides); err != nil {
		t.Fatal(err)
	}
	if sides != 2 {
		t.Fatalf("biometric_decision_sides rows = %d, want 2", sides)
	}
}
