package operations_test

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

	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/operations"
)

// testDB mirrors the other packages' helper: a throwaway, migrated database.
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

func TestSlugify(t *testing.T) {
	for name, want := range map[string]string{
		"Operação Ágata 2":   "operacao-agata-2",
		"  Mão  na   Massa ": "mao-na-massa",
		"--A/B--":            "a-b",
		"???":                "",
	} {
		if got := operations.Slugify(name); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestOperationLifecycle(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)

	first, err := operations.Create(ctx, sqlDB, operations.CreateInput{Name: " Operação Um ", Description: " d "}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "operacao-um" || first.Name != "Operação Um" || first.Description != "d" ||
		first.Status != operations.StatusActive || first.ClosedAt != nil {
		t.Fatalf("created: %+v", first)
	}

	// Refused: the same slug, no name, a name that gives no slug, a malformed slug.
	if _, err := operations.Create(ctx, sqlDB, operations.CreateInput{Name: "Outro", Slug: "operacao-um"}, 0); !errors.Is(err, operations.ErrSlugTaken) {
		t.Errorf("a taken slug: %v", err)
	}
	var invalid operations.ValidationError
	for _, in := range []operations.CreateInput{{Name: "  "}, {Name: "???"}, {Name: "X", Slug: "Not A Slug"}} {
		if _, err := operations.Create(ctx, sqlDB, in, 0); !errors.As(err, &invalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}

	second, err := operations.Create(ctx, sqlDB, operations.CreateInput{Name: "Dois", Slug: "dois"}, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Closing records when, and lists it after the active ones; the slug stays; reopening clears it.
	closed, err := operations.Update(ctx, sqlDB, first.Slug, operations.UpdateInput{Name: "Renomeada", Status: operations.StatusClosed})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Slug != first.Slug || closed.Name != "Renomeada" || closed.ClosedAt == nil {
		t.Fatalf("closed: %+v", closed)
	}
	list, err := operations.List(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Slug != second.Slug || list[1].Slug != first.Slug {
		t.Fatalf("list: %+v", list)
	}
	reopened, err := operations.Update(ctx, sqlDB, first.Slug, operations.UpdateInput{Name: "Renomeada", Status: operations.StatusActive})
	if err != nil || reopened.ClosedAt != nil {
		t.Fatalf("reopened: %+v, %v", reopened, err)
	}

	if _, err := operations.Update(ctx, sqlDB, first.Slug, operations.UpdateInput{Name: "X", Status: "WHATEVER"}); !errors.As(err, &invalid) {
		t.Errorf("an unknown status: %v", err)
	}
	if _, err := operations.Update(ctx, sqlDB, "nope", operations.UpdateInput{Name: "X", Status: operations.StatusActive}); !errors.Is(err, operations.ErrNotFound) {
		t.Errorf("updating a missing one: %v", err)
	}
	if _, err := operations.Get(ctx, sqlDB, "nope"); !errors.Is(err, operations.ErrNotFound) {
		t.Errorf("getting a missing one: %v", err)
	}
}
