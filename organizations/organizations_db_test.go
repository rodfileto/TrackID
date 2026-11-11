package organizations_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/organizations"
)

// testDB mirrors requests/requests_db_test.go's helper: a throwaway, migrated database on the server named by
// TRACKID_TEST_DATABASE_URL, skipped when unset.
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

func user(t *testing.T, sqlDB *sql.DB, username string) int64 {
	t.Helper()
	var id int64
	if err := sqlDB.QueryRow(`INSERT INTO users (first_name, last_name, staff_number, job_title, username, email, password_hash)
		VALUES ('A', 'B', $1, 'Perito', $1, $1 || '@x', 'x') RETURNING id`, username).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestUnitsAndAssignments(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	s := organizations.Service{DB: sqlDB}
	q := db.New(sqlDB)

	org, err := s.CreateOrganization(ctx, organizations.OrganizationInput{Acronym: "PC/MG", Name: "Polícia Civil de Minas Gerais", Kind: "STATE", UF: "mg"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrganization(ctx, organizations.OrganizationInput{Acronym: "PC/MG", Name: "Outra", Kind: "STATE", UF: "MG"}); !errors.Is(err, organizations.ErrDuplicate) {
		t.Fatalf("duplicate organization: %v", err)
	}
	if _, err := s.CreateOrganization(ctx, organizations.OrganizationInput{Acronym: "X", Name: "X", Kind: "STATE"}); err == nil {
		t.Fatal("a state's organization with no UF was accepted")
	}

	units, err := s.ListUnits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var root organizations.Unit
	for _, u := range units {
		if u.OrganizationID == org && u.ParentID == nil {
			root = u
		}
	}
	if root.ID == 0 || root.Acronym != "PC/MG" {
		t.Fatalf("root unit of the new organization: %+v", units)
	}
	if err := s.UpdateUnit(ctx, root.ID, organizations.UnitInput{Acronym: root.Acronym, Name: root.Name,
		LaudoHeader: []string{" POLÍCIA CIVIL DE MINAS GERAIS ", "", "Instituto de Identificação"}}); err != nil {
		t.Fatal(err)
	}
	sr, err := s.CreateUnit(ctx, organizations.UnitInput{ParentID: &root.ID, Acronym: "DRBH", Name: "Departamento de Belo Horizonte", UF: "MG", MunicipalityCode: "3106200"})
	if err != nil {
		t.Fatal(err)
	}
	nucleo, err := s.CreateUnit(ctx, organizations.UnitInput{ParentID: &sr, Acronym: "NID/DRBH", Name: "Núcleo de Identificação",
		LaudoName: "Núcleo de Identificação do Departamento de Belo Horizonte"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUnit(ctx, organizations.UnitInput{ParentID: &root.ID, Acronym: "DRUDI", Name: "Departamento de Uberlândia",
		LaudoHeader: []string{"POLÍCIA CIVIL DE MINAS GERAIS", "Departamento de Uberlândia"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUnit(ctx, organizations.UnitInput{ParentID: &root.ID, Acronym: "DRBH", Name: "Again"}); !errors.Is(err, organizations.ErrDuplicate) {
		t.Fatalf("duplicate unit acronym: %v", err)
	}
	// A unit never goes below itself, nor becomes a second root.
	if err := s.UpdateUnit(ctx, sr, organizations.UnitInput{ParentID: &nucleo, Acronym: "DRBH", Name: "Departamento de Belo Horizonte"}); err == nil {
		t.Fatal("a unit was placed below its own child")
	}
	if err := s.UpdateUnit(ctx, sr, organizations.UnitInput{Acronym: "DRBH", Name: "Departamento de Belo Horizonte"}); err == nil {
		t.Fatal("a second root was accepted")
	}

	ana := user(t, sqlDB, "ana")
	posted := func(unit int64, start, end string) (int64, error) {
		return s.CreateAssignment(ctx, organizations.AssignmentInput{UserID: ana, UnitID: unit, Kind: organizations.Posting, StartsOn: start, EndsOn: end}, 0)
	}
	if _, err := posted(nucleo, "2026-01-01", ""); err != nil {
		t.Fatal(err)
	}
	// A later posting is a transfer: the open one ends the day before.
	if _, err := posted(other, "2026-06-01", ""); err != nil {
		t.Fatal(err)
	}
	mission, err := s.CreateAssignment(ctx, organizations.AssignmentInput{UserID: ana, UnitID: nucleo, Kind: organizations.Mission,
		StartsOn: "2026-07-01", EndsOn: "2026-07-15", Note: "Ordem de missão 12/2026"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAssignment(ctx, organizations.AssignmentInput{UserID: ana, UnitID: sr, Kind: organizations.Mission,
		StartsOn: "2026-07-10", EndsOn: "2026-07-20"}, 0); !errors.Is(err, organizations.ErrOverlap) {
		t.Fatalf("overlapping mission: %v", err)
	}
	if _, err := posted(sr, "2026-03-01", "2026-03-10"); !errors.Is(err, organizations.ErrOverlap) {
		t.Fatalf("overlapping posting: %v", err)
	}
	list, err := s.ListAssignments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[2].EndsOn != "2026-05-31" || list[1].EndsOn != "" {
		t.Fatalf("assignments after the transfer: %+v", list)
	}

	for _, c := range []struct {
		day     string
		unit    int64
		mission bool
		name    string
		header  []string
	}{
		{"2025-12-31", 0, false, "", nil},
		{"2026-03-01", nucleo, false, "Núcleo de Identificação do Departamento de Belo Horizonte", []string{"POLÍCIA CIVIL DE MINAS GERAIS", "Instituto de Identificação"}},
		{"2026-06-15", other, false, "Departamento de Uberlândia", []string{"POLÍCIA CIVIL DE MINAS GERAIS", "Departamento de Uberlândia"}},
		{"2026-07-15", nucleo, true, "Núcleo de Identificação do Departamento de Belo Horizonte", []string{"POLÍCIA CIVIL DE MINAS GERAIS", "Instituto de Identificação"}},
		{"2026-07-16", other, false, "Departamento de Uberlândia", []string{"POLÍCIA CIVIL DE MINAS GERAIS", "Departamento de Uberlândia"}},
	} {
		p, err := organizations.UnitOn(ctx, q, ana, day(c.day))
		if err != nil {
			t.Fatal(err)
		}
		if c.unit == 0 {
			if p != nil {
				t.Errorf("%s: unit %+v, want none", c.day, p)
			}
			continue
		}
		if p == nil || p.UnitID != c.unit || p.Mission != c.mission || p.LaudoName != c.name || !slices.Equal(p.Header, c.header) {
			t.Errorf("%s: %+v, want unit %d mission %v %q %q", c.day, p, c.unit, c.mission, c.name, c.header)
		}
	}

	// With no header in its line, a unit's header is its organization's name.
	if err := s.UpdateUnit(ctx, root.ID, organizations.UnitInput{Acronym: root.Acronym, Name: root.Name}); err != nil {
		t.Fatal(err)
	}
	if p, err := organizations.UnitOn(ctx, q, ana, day("2026-03-01")); err != nil || !slices.Equal(p.Header, []string{"Polícia Civil de Minas Gerais"}) {
		t.Fatalf("header with none set: %+v %v", p, err)
	}

	// Deleting: a unit someone worked in, or one with units below it, is deactivated instead.
	if err := s.DeleteUnit(ctx, nucleo); !errors.Is(err, organizations.ErrInUse) {
		t.Fatalf("deleting a unit with assignments: %v", err)
	}
	if err := s.DeleteUnit(ctx, sr); !errors.Is(err, organizations.ErrInUse) {
		t.Fatalf("deleting a unit with units below it: %v", err)
	}
	if err := s.DeleteUnit(ctx, root.ID); err == nil {
		t.Fatal("deleted a root unit")
	}
	if err := s.DeleteOrganization(ctx, org); !errors.Is(err, organizations.ErrInUse) {
		t.Fatalf("deleting an organization with units: %v", err)
	}
	if err := s.DeleteAssignment(ctx, mission); err != nil {
		t.Fatal(err)
	}

	// New cases go to one unit; giving it to another takes it from the first.
	if err := s.UpdateUnit(ctx, other, organizations.UnitInput{ParentID: &root.ID, Acronym: "DRUDI", Name: "Departamento de Uberlândia", ReceivesNewCases: true}); err != nil {
		t.Fatal(err)
	}
	var receivers int
	var receiver int64
	if err := sqlDB.QueryRow(`SELECT COUNT(*), MAX(id) FROM units WHERE receives_new_cases`).Scan(&receivers, &receiver); err != nil {
		t.Fatal(err)
	}
	if receivers != 1 || receiver != other {
		t.Fatalf("receiving units: %d, %d; want only %d", receivers, receiver, other)
	}
	if err := s.UpdateUnit(ctx, other, organizations.UnitInput{ParentID: &root.ID, Acronym: "DRUDI", Name: "Departamento de Uberlândia", Active: new(bool)}); err == nil {
		t.Fatal("deactivated the unit receiving new cases")
	}

	// An organization with only its root unit goes, root and all.
	empty, err := s.CreateOrganization(ctx, organizations.OrganizationInput{Acronym: "PC/ES", Name: "Polícia Civil do Espírito Santo", Kind: "STATE", UF: "ES"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOrganization(ctx, empty); err != nil {
		t.Fatal(err)
	}
}
