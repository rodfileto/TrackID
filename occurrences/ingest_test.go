package occurrences_test

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
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/occurrences"
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

// TestIngestAndLink: an occurrence upserts with its location and types, re-ingesting replaces
// its types without duplicating it, two reports share one location, and cases.Ingest links a
// case to it by (agency, number).
func TestIngestAndLink(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	when := time.Date(2024, 5, 3, 21, 30, 0, 0, time.UTC)
	in := occurrences.Input{
		Agency: "DP-005", Number: "SIM-2024-000001",
		Location: occurrences.LocationInput{
			LocationID: "SIM-L-1", Lon: -43.1773, Lat: -22.9104, Precision: "EXACT",
			MunicipalityCode: "3304557", UF: "RJ", PlaceType: "BANK",
		},
		OccurredFrom: when, RegisteredAt: when.Add(time.Hour),
		ModusOperandi: []string{"ATM_HELP_SCAM"}, Status: "REGISTERED",
		Description: "simulated", Types: []string{"FRAUD"},
	}
	first, err := occurrences.Ingest(ctx, sqlDB, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Types = []string{"STREET_ROBBERY", "ROBBERY_HOMICIDE"}
	again, err := occurrences.Ingest(ctx, sqlDB, in)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Inserted || again.Inserted || again.OccurrenceID != first.OccurrenceID {
		t.Errorf("re-ingest: first %+v, again %+v", first, again)
	}
	var types string
	sqlDB.QueryRow(`SELECT string_agg(type_code, ',' ORDER BY type_code) FROM occurrence_classifications WHERE occurrence_id = $1`, first.OccurrenceID).Scan(&types)
	if types != "ROBBERY_HOMICIDE,STREET_ROBBERY" {
		t.Errorf("types after re-ingest: %q", types)
	}

	second := in
	second.Number = "SIM-2024-000002"
	res, err := occurrences.Ingest(ctx, sqlDB, second)
	if err != nil {
		t.Fatal(err)
	}
	if res.LocationID != first.LocationID {
		t.Errorf("same place, two locations: %d, %d", first.LocationID, res.LocationID)
	}

	if _, err := occurrences.Ingest(ctx, sqlDB, occurrences.Input{Agency: "x", Number: "y", Location: in.Location, OccurredFrom: when, RegisteredAt: when, Status: "REGISTERED", Types: []string{"NOT_A_TYPE"}}); err == nil {
		t.Error("unknown type accepted")
	}

	for _, ref := range []*cases.OccurrenceRef{{Agency: "DP-005", Number: "SIM-2024-000001"}, nil} {
		if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "C-1", CaseType: "CRIMINAL", Modality: "FACIAL", Description: "c", Occurrence: ref}); err != nil {
			t.Fatal(err)
		}
		var linked sql.NullInt64
		sqlDB.QueryRow(`SELECT occurrence_id FROM biometric_cases WHERE case_id = 'C-1'`).Scan(&linked)
		if linked.Int64 != first.OccurrenceID {
			t.Errorf("ref %v: case linked to %v, want %d (nil keeps the link)", ref, linked, first.OccurrenceID)
		}
	}
	if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "C-2", CaseType: "CRIMINAL", Modality: "FACIAL", Description: "c", Occurrence: &cases.OccurrenceRef{Agency: "DP-005", Number: "missing"}}); err == nil {
		t.Error("a link to a missing occurrence was accepted")
	}
}
