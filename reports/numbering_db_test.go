package reports_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/reports"
)

type numberingDB struct {
	t     *testing.T
	sqlDB *sql.DB
	cases int
}

func (n *numberingDB) units() (first, second int64) {
	n.t.Helper()
	if err := n.sqlDB.QueryRow(`SELECT default_case_unit()`).Scan(&first); err != nil {
		n.t.Fatal(err)
	}
	if err := n.sqlDB.QueryRow(`INSERT INTO units (organization_id, parent_id, acronym, name)
SELECT organization_id, id, 'NID/TESTE', 'Núcleo de teste' FROM units WHERE id = $1 RETURNING id`, first).Scan(&second); err != nil {
		n.t.Fatal(err)
	}
	return first, second
}

// analysis writes a closed analysis, of a case of unit, with its draft report.
func (n *numberingDB) analysis(unit int64, modality, analysisType string) (id int64) {
	n.t.Helper()
	n.cases++
	var caseRow int64
	if err := n.sqlDB.QueryRow(`INSERT INTO biometric_cases (case_id, case_type, modality, description, unit_id)
VALUES ($1, 'CRIMINAL', $2, '', $3) RETURNING id`, fmt.Sprintf("2026.%04d", n.cases), modality, unit).Scan(&caseRow); err != nil {
		n.t.Fatal(err)
	}
	if err := n.sqlDB.QueryRow(`INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type, status, stage, closed_at)
VALUES ($1, 1, $2, 'CLOSED', 'REPORT', NOW()) RETURNING id`, caseRow, analysisType).Scan(&id); err != nil {
		n.t.Fatal(err)
	}
	if _, err := n.sqlDB.Exec(`INSERT INTO analysis_reports (case_analysis_id, report_type) VALUES ($1, $2)`, id, analysisType+"_REPORT"); err != nil {
		n.t.Fatal(err)
	}
	return id
}

func TestReportNumberIsByUnitAndDiscipline(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	n := &numberingDB{t: t, sqlDB: sqlDB}
	first, second := n.units()
	issued := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	record := func(analysis, unit int64, number int32) string {
		t.Helper()
		got, err := reports.RecordImported(ctx, sqlDB, reports.ImportedReport{AnalysisID: analysis, IssuingUnitID: unit, Year: 2026, Number: number, IssuedAt: issued}, issued)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	review := n.analysis(first, "FACIAL", "FACIAL_REVIEW")
	if got := record(review, 0, 133); got != "2026.0133" {
		t.Errorf("first number = %q", got)
	}
	if got := record(n.analysis(first, "FACIAL", "FACIAL_COMPARISON"), 0, 133); got != "" {
		t.Errorf("a comparison shares its unit's facial sequence with a review: number = %q, want none", got)
	}
	if got := record(n.analysis(second, "FACIAL", "FACIAL_REVIEW"), 0, 133); got != "2026.0133" {
		t.Errorf("another unit's sequence: number = %q", got)
	}
	if got := record(n.analysis(first, "FINGERPRINT", "FINGERPRINT_COMPARISON"), 0, 133); got != "2026.0133" {
		t.Errorf("another discipline's sequence: number = %q", got)
	}

	// The issuing unit is the report's own, which can differ from the case's.
	lent := n.analysis(first, "FACIAL", "FACIAL_REVIEW")
	if got := record(lent, second, 200); got != "2026.0200" {
		t.Errorf("issued by another unit: number = %q", got)
	}
	var unit int64
	if err := sqlDB.QueryRow(`SELECT issuing_unit_id FROM analysis_reports WHERE case_analysis_id = $1`, lent).Scan(&unit); err != nil || unit != second {
		t.Errorf("issuing unit = %d, %v; want %d", unit, err, second)
	}
	if got := record(n.analysis(second, "FACIAL", "FACIAL_COMPARISON"), 0, 200); got != "" {
		t.Errorf("number 200 is the issuing unit's: number = %q, want none", got)
	}

	// Recording a report again keeps its number.
	if got := record(review, 0, 133); got != "2026.0133" {
		t.Errorf("recorded again: number = %q", got)
	}

	q := db.New(sqlDB)
	for _, c := range []struct {
		unit       int64
		reportType string
		want       int32
	}{
		{first, "FACIAL_COMPARISON_REPORT", 133},
		{second, "FACIAL_REVIEW_REPORT", 200},
		{first, "FINGERPRINT_LAB_EXAMINATION_REPORT", 133}, // the fingerprint discipline's, with the comparison's number
		{first, "CRIME_SCENE_EXAMINATION_REPORT", 0},
	} {
		got, err := q.MaxReportNumber(ctx, db.MaxReportNumberParams{IssuingUnitID: c.unit, ReportType: c.reportType, ReportYear: 2026})
		if err != nil || got != c.want {
			t.Errorf("last number of unit %d, %s = %d, %v; want %d", c.unit, c.reportType, got, err, c.want)
		}
	}
}

func TestMigrationGivesNumberedReportsTheirCaseUnit(t *testing.T) {
	sqlDB := testDB(t)
	if err := goose.DownTo(sqlDB, ".", 20); err != nil {
		t.Fatal(err)
	}
	n := &numberingDB{t: t, sqlDB: sqlDB}
	first, _ := n.units()
	issue := func(analysis int64, number int) {
		t.Helper()
		if _, err := sqlDB.Exec(`UPDATE analysis_reports SET status = 'ISSUED', imported = TRUE, issued_at = NOW(), snapshot = '{}',
report_year = 2026, report_number = $2 WHERE case_analysis_id = $1`, analysis, number); err != nil {
			t.Fatal(err)
		}
	}
	// Numbered per type, so a unit's review and comparison can hold the same number.
	review := n.analysis(first, "FACIAL", "FACIAL_REVIEW")
	comparison := n.analysis(first, "FACIAL", "FACIAL_COMPARISON")
	fingerprint := n.analysis(first, "FINGERPRINT", "FINGERPRINT_COMPARISON")
	issue(review, 1)
	issue(comparison, 1)
	issue(fingerprint, 1)
	draft := n.analysis(first, "FACIAL", "FACIAL_REVIEW")

	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name       string
		analysis   int64
		wantUnit   bool
		wantNumber bool
	}{
		{"review", review, true, true},
		{"comparison with the review's number", comparison, false, true},
		{"fingerprint", fingerprint, true, true},
		{"draft", draft, false, false},
	} {
		var unit sql.NullInt64
		var number sql.NullInt32
		if err := sqlDB.QueryRow(`SELECT issuing_unit_id, report_number FROM analysis_reports WHERE case_analysis_id = $1`, c.analysis).Scan(&unit, &number); err != nil {
			t.Fatal(err)
		}
		if unit.Valid != c.wantUnit || number.Valid != c.wantNumber {
			t.Errorf("%s: unit %+v, number %+v; want unit %v, number %v", c.name, unit, number, c.wantUnit, c.wantNumber)
		}
	}
}
