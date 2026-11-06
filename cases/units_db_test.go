package cases_test

import (
	"context"
	"testing"

	"github.com/rodfileto/trackid/cases"
)

// TestCaseUnits covers who owns a case (db/migrations/018_organizations.sql): a new case takes the unit
// named or the one that receives new cases, and running an ingest again leaves its unit alone.
func TestCaseUnits(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)

	var root, organization int64
	if err := sqlDB.QueryRow(`SELECT id, organization_id FROM units WHERE receives_new_cases`).Scan(&root, &organization); err != nil {
		t.Fatal(err)
	}
	var child int64
	if err := sqlDB.QueryRow(`INSERT INTO units (organization_id, parent_id, acronym, name, uf, municipality_code)
		VALUES ($1, $2, 'SR/MG', 'Regional', 'MG', '3106200') RETURNING id`, organization, root).Scan(&child); err != nil {
		t.Fatal(err)
	}
	unitOf := func(caseID string) int64 {
		t.Helper()
		var unit int64
		if err := sqlDB.QueryRow(`SELECT unit_id FROM biometric_cases WHERE case_id = $1`, caseID).Scan(&unit); err != nil {
			t.Fatal(err)
		}
		return unit
	}
	ingest := func(caseID string, unit int64) {
		t.Helper()
		if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", UnitID: unit}); err != nil {
			t.Fatal(err)
		}
	}

	ingest("A", 0)
	if got := unitOf("A"); got != root {
		t.Errorf("no unit named: unit %d, want the receiver %d", got, root)
	}
	ingest("B", child)
	if got := unitOf("B"); got != child {
		t.Errorf("unit named: unit %d, want %d", got, child)
	}
	ingest("A", child)
	ingest("B", 0)
	if unitOf("A") != root || unitOf("B") != child {
		t.Error("ingesting again moved a case to another unit")
	}
	created, err := cases.Create(ctx, sqlDB, cases.CreateInput{CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := unitOf(created.CaseID); got != root {
		t.Errorf("created case: unit %d, want the receiver %d", got, root)
	}

	// Moving the receiver changes where new cases go, and only those.
	if _, err := sqlDB.Exec(`UPDATE units SET receives_new_cases = FALSE`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`UPDATE units SET receives_new_cases = TRUE WHERE id = $1`, child); err != nil {
		t.Fatal(err)
	}
	ingest("C", 0)
	if unitOf("C") != child || unitOf("A") != root {
		t.Error("the receiver moved: new cases should follow it, and old ones stay")
	}
}

// TestUnitRules covers the tree's constraints.
func TestUnitRules(t *testing.T) {
	sqlDB := testDB(t)
	var root, organization int64
	if err := sqlDB.QueryRow(`SELECT id, organization_id FROM units WHERE parent_id IS NULL`).Scan(&root, &organization); err != nil {
		t.Fatal(err)
	}
	var other int64
	if err := sqlDB.QueryRow(`INSERT INTO organizations (acronym, name, kind, uf) VALUES ('PC/CE', 'Polícia Civil', 'STATE', 'CE') RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	refused := []struct {
		name, query string
		args        []any
	}{
		{"a parent of another organization", `INSERT INTO units (organization_id, parent_id, acronym, name) VALUES ($1, $2, 'X', 'X')`, []any{other, root}},
		{"a second root", `INSERT INTO units (organization_id, acronym, name) VALUES ($1, 'X', 'X')`, []any{organization}},
		{"a repeated acronym", `INSERT INTO units (organization_id, parent_id, acronym, name) SELECT organization_id, id, acronym, name FROM units WHERE id = $1`, []any{root}},
		{"a municipality with no state", `INSERT INTO units (organization_id, parent_id, acronym, name, municipality_code) VALUES ($1, $2, 'X', 'X', '2304400')`, []any{organization, root}},
		{"a second receiver", `INSERT INTO units (organization_id, parent_id, acronym, name, receives_new_cases) VALUES ($1, $2, 'X', 'X', TRUE)`, []any{organization, root}},
		{"a state's organization with no state", `INSERT INTO organizations (acronym, name, kind) VALUES ('PC', 'PC', 'STATE')`, nil},
	}
	for _, c := range refused {
		if _, err := sqlDB.Exec(c.query, c.args...); err == nil {
			t.Errorf("%s was accepted", c.name)
		}
	}
	// The same acronym in another organization is another unit.
	if _, err := sqlDB.Exec(`INSERT INTO units (organization_id, acronym, name) SELECT $1, acronym, name FROM units WHERE id = $2`, other, root); err != nil {
		t.Errorf("the same acronym in another organization: %v", err)
	}
}
