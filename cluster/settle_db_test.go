package cluster_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db/migrations"
)

// testDB mirrors cases/ingest_test.go's helper: a throwaway, migrated database on the server
// named by TRACKID_TEST_DATABASE_URL, skipped when unset.
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
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
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

// TestSettle is the review exercise's dry run in miniature: an examiner submits while a sync is
// running, so the submit's own sync enqueue is rejected as a duplicate, and the running sync has
// to take the decision in with another pass.
func TestSettle(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	decide := func(a, b string) {
		t.Helper()
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO biometric_decisions
			(feature_a_id, feature_b_id, modality, role, decision, username)
			VALUES ($1, $2, 'FACIAL', 'VERIFICATOR', 'POSITIVE', 'examiner')`, a, b); err != nil {
			t.Fatal(err)
		}
	}
	none := func(context.Context) (int64, error) { return 0, nil }

	passes := 0
	err := cluster.Settle(ctx, sqlDB, none, func(context.Context) error {
		passes++
		if passes == 1 {
			decide("1", "2") // submitted during the first pass
		}
		return nil
	})
	if err != nil || passes != 2 {
		t.Fatalf("decision during a pass: passes=%d err=%v, want 2 passes", passes, err)
	}

	passes = 0
	if err := cluster.Settle(ctx, sqlDB, none, func(context.Context) error { passes++; return nil }); err != nil || passes != 1 {
		t.Fatalf("nothing new: passes=%d err=%v, want 1 pass", passes, err)
	}

	// A feature that stays unmatched cannot keep the sync running.
	stuck := func(context.Context) (int64, error) { return 1, nil }
	passes = 0
	if err := cluster.Settle(ctx, sqlDB, stuck, func(context.Context) error { passes++; return nil }); err != nil || passes != cluster.MaxSettlePasses {
		t.Fatalf("stuck backlog: passes=%d err=%v, want %d passes", passes, err, cluster.MaxSettlePasses)
	}

	boom := errors.New("boom")
	if err := cluster.Settle(ctx, sqlDB, none, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("pass error: got %v", err)
	}
}
