package distribution_test

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

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/graph"
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

// TestAnalysesDistributionAndAccess walks the whole flow: every FACIAL case gets its review (once,
// even when re-ingested; a FINGERPRINT case gets none); a distribution hands out only the
// requested number, balanced, and leaves the rest pending; an admin opens a comparison on pairs
// of a case and it is distributed in turn; access follows the analyses a user holds, an admin
// opens every case but decides only on what they hold; issuing a report numbers it and closes the
// analysis; a manual reassignment checks the examiner's modality and keeps the history.
func TestAnalysesDistributionAndAccess(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(username string, fp, facial bool, role auth.AccountRole) auth.UserProfile {
		t.Helper()
		u, err := repo.Create(ctx, auth.NewUser{Nome: username, UltimoNome: "Test", Matricula: username, Cargo: "Perito",
			Username: username, Password: "password-1",
			Access: auth.Access{AccountRole: role, ExaminesFingerprint: fp, ExaminesFacial: facial, IsActive: true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	chief := user("chief", false, false, auth.AccountRoleSuperAdmin)
	ana := user("ana", false, true, auth.AccountRoleUser)
	bia := user("bia", true, true, auth.AccountRoleUser)
	caio := user("caio", true, false, auth.AccountRoleUser)

	ingest := func(caseID, modality string, traces int) cases.Result {
		t.Helper()
		ev := cases.EvidenceInput{Sequence: 1}
		for i := 1; i <= traces; i++ {
			ev.Traces = append(ev.Traces, cases.TraceInput{Sequence: int16(i), TraceType: "FACE_RECORD"})
		}
		r, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: cases.CaseTypeCriminal, Modality: modality,
			Description: "test", Evidences: []cases.EvidenceInput{ev}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	f1 := ingest("F1", "FACIAL", 2)
	ingest("F1", "FACIAL", 2) // re-ingesting opens no second review
	ingest("F2", "FACIAL", 1)
	ingest("F3", "FACIAL", 1)
	ingest("P1", "FINGERPRINT", 0)

	status, err := distribution.GetStatus(ctx, sqlDB)
	if err != nil || status.Pending[analyses.FacialReview] != 3 || status.Pending[analyses.FacialComparison] != 0 {
		t.Fatalf("status = %+v, %v; want 3 pending reviews", status, err)
	}
	if list, err := analyses.List(ctx, sqlDB, "P1"); err != nil || len(list) != 0 {
		t.Fatalf("fingerprint case analyses = %+v, %v; want none", list, err)
	}

	if _, err := distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 0}); !errors.Is(err, distribution.ErrInvalidCount) {
		t.Fatalf("count 0: %v", err)
	}
	if dry, err := distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 2, DryRun: true}); err != nil || len(dry.Assigned) != 2 {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	res, err := distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	// F1 and F2's reviews: ana and bia both at 0, ana has the lower id; then bia. F3 stays pending.
	if got := fmt.Sprint(names(res.Assigned)); got != "[F1:ana F2:bia]" || res.StillPending != 1 {
		t.Fatalf("distribution = %s (still pending %d), want [F1:ana F2:bia] with 1 pending", got, res.StillPending)
	}

	// The admin opens a comparison on F1's two faces against an enrolled face.
	trace1 := graph.QuestionedFeatureID(f1.Evidences[0].Traces[0].TraceID)
	trace2 := graph.QuestionedFeatureID(f1.Evidences[0].Traces[1].TraceID)
	if _, err := analyses.Open(ctx, sqlDB, "F1", analyses.FacialComparison, nil, chief.ID); !errors.Is(err, analyses.ErrNeedsPairs) {
		t.Fatalf("comparison without pairs: %v", err)
	}
	if _, err := analyses.Open(ctx, sqlDB, "F1", analyses.FacialComparison, []analyses.Pair{{FeatureA: "3401", FeatureB: "77"}}, chief.ID); !errors.Is(err, analyses.ErrInvalidPair) {
		t.Fatalf("pair with no face of the case: %v", err)
	}
	if _, err := analyses.Open(ctx, sqlDB, "P1", analyses.FacialReview, nil, chief.ID); !errors.Is(err, analyses.ErrWrongModality) {
		t.Fatalf("facial review on a fingerprint case: %v", err)
	}
	comparison, err := analyses.Open(ctx, sqlDB, "F1", analyses.FacialComparison,
		[]analyses.Pair{{FeatureA: trace1, FeatureB: "3401"}, {FeatureA: "3401", FeatureB: trace1}, {FeatureA: trace2, FeatureB: "3401"}}, chief.ID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := analyses.List(ctx, sqlDB, "F1")
	if err != nil || len(list) != 2 || list[1].Sequence != 2 || list[1].Type != analyses.FacialComparison ||
		len(list[1].Pairs) != 2 || list[1].RequestedBy != "chief" || list[1].Report.Type != "FACIAL_COMPARISON_REPORT" ||
		list[1].Report.Status != analyses.ReportDraft || list[0].Examiner != "ana" {
		t.Fatalf("F1 analyses = %+v, %v", list, err)
	}

	// The rest, oldest first: F3's review to ana (ana and bia at 1, ana has the lower id), then
	// the comparison to bia. caio examines only fingerprints and gets neither.
	res, err = distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 5})
	if err != nil || fmt.Sprint(names(res.Assigned)) != "[F3:ana F1:bia]" || res.StillPending != 0 {
		t.Fatalf("second distribution = %v, %v", names(res.Assigned), err)
	}

	check := func(what string, got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Errorf("%s: got %v, want %v", what, got, want)
		}
	}
	target := func(id int64) analyses.Target {
		t.Helper()
		tg, err := analyses.GetTarget(ctx, db.New(sqlDB), id)
		if err != nil {
			t.Fatal(err)
		}
		return tg
	}
	check("ana opens F1 (its review)", distribution.CanOpen(ctx, sqlDB, ana, "F1"), nil)
	check("bia opens F1 (its comparison)", distribution.CanOpen(ctx, sqlDB, bia, "F1"), nil)
	check("caio opens F1", distribution.CanOpen(ctx, sqlDB, caio, "F1"), distribution.ErrNotAssigned)
	check("caio opens the fingerprint case", distribution.CanOpen(ctx, sqlDB, caio, "P1"), distribution.ErrNotAssigned)
	check("the admin opens anything", distribution.CanOpen(ctx, sqlDB, chief, "P1"), nil)
	check("unknown case", distribution.CanOpen(ctx, sqlDB, chief, "NOPE"), analyses.ErrCaseNotFound)
	check("bia decides on the comparison", analyses.CanDecide(bia, target(comparison)), nil)
	check("ana does not decide on the comparison", analyses.CanDecide(ana, target(comparison)), analyses.ErrCannotDecide)
	check("the admin does not decide on it either", analyses.CanDecide(chief, target(comparison)), analyses.ErrCannotDecide)

	// Manual reassignment: not to a fingerprint-only examiner; to ana, keeping the history.
	check("comparison to caio", distribution.Assign(ctx, sqlDB, comparison, &caio.ID, chief.ID), distribution.ErrNotExaminer)
	check("comparison to bia again", distribution.Assign(ctx, sqlDB, comparison, &bia.ID, chief.ID), distribution.ErrUnchanged)
	check("comparison to ana", distribution.Assign(ctx, sqlDB, comparison, &ana.ID, chief.ID), nil)
	check("bia lost F1", distribution.CanOpen(ctx, sqlDB, bia, "F1"), distribution.ErrNotAssigned)
	history, err := distribution.History(ctx, sqlDB, comparison)
	if err != nil || len(history) != 2 || history[0].Username != "bia" || history[0].Method != distribution.MethodAutomatic ||
		history[1].Username != "ana" || history[1].AssignedBy != "chief" {
		t.Fatalf("comparison history = %+v, %v", history, err)
	}

	// Issuing: only the examiner; numbered per report type and year; closes the analysis.
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if _, err := analyses.IssueReport(ctx, sqlDB, comparison, chief, now); !errors.Is(err, analyses.ErrCannotDecide) {
		t.Fatalf("the admin issuing ana's report: %v", err)
	}
	report, err := analyses.IssueReport(ctx, sqlDB, comparison, ana, now)
	if err != nil || report.Number != "2026.0001" || report.Status != analyses.ReportIssued || report.IssuedBy != "ana" {
		t.Fatalf("issued comparison report = %+v, %v", report, err)
	}
	reviewF1 := list[0].ID
	report, err = analyses.IssueReport(ctx, sqlDB, reviewF1, ana, now)
	if err != nil || report.Number != "2026.0001" || report.Type != "FACIAL_REVIEW_REPORT" {
		t.Fatalf("issued review report = %+v, %v; want its own numbering", report, err)
	}
	check("deciding on a closed analysis", analyses.CanDecide(ana, target(comparison)), analyses.ErrCannotDecide)
	check("reassigning a closed analysis", distribution.Assign(ctx, sqlDB, comparison, &bia.ID, chief.ID), distribution.ErrClosed)
	check("ana still opens F1 after closing it", distribution.CanOpen(ctx, sqlDB, ana, "F1"), nil)

	mine, err := distribution.AnalysesOf(ctx, sqlDB, ana.ID)
	if err != nil || len(mine) != 3 || mine[0].Status != analyses.StatusOpen || mine[0].CaseID != "F3" {
		t.Fatalf("ana's analyses = %+v, %v; want F3's open review first, then F1's two closed ones", mine, err)
	}
	status, err = distribution.GetStatus(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range status.Examiners {
		if e.Username == "ana" && e.OpenAnalyses != 1 {
			t.Errorf("ana's open analyses = %d, want 1 (closed ones do not count)", e.OpenAnalyses)
		}
	}
}

func names(as []distribution.Assignment) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.CaseID+":"+a.Username)
	}
	return out
}
