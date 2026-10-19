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
	"github.com/rodfileto/trackid/examination"
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

// TestDecisionFlow walks a FACIAL_REVIEW through every stage with real users: distribution hands
// out each step to a different examiner; the first examiner's drafts can change until submitted;
// verification is blind; a disagreement goes to a third examiner for that pair only; the first
// examiner issues the report. Then a comparison checks the reassignment rules, and a case with no
// faces goes straight to its report.
func TestDecisionFlow(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	q := db.New(sqlDB)
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
	bia := user("bia", false, true, auth.AccountRoleUser)
	caio := user("caio", false, true, auth.AccountRoleUser)
	duda := user("duda", true, false, auth.AccountRoleUser)

	ev := cases.EvidenceInput{Sequence: 1, Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_RECORD"}, {Sequence: 2, TraceType: "FACE_RECORD"}}}
	f1, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "F1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "test",
		Evidences: []cases.EvidenceInput{ev}})
	if err != nil {
		t.Fatal(err)
	}
	trace1 := graph.QuestionedFeatureID(f1.Evidences[0].Traces[0].TraceID)
	trace2 := graph.QuestionedFeatureID(f1.Evidences[0].Traces[1].TraceID)
	pair := func(a, b string) examination.Pair {
		if a > b {
			a, b = b, a
		}
		return examination.Pair{FeatureA: a, FeatureB: b}
	}
	system := func(p examination.Pair, decision string, score float64) {
		t.Helper()
		if _, err := q.InsertBiometricDecision(ctx, db.InsertBiometricDecisionParams{FeatureAID: p.FeatureA, FeatureBID: p.FeatureB,
			Modality: "FACE", Role: "SYSTEM", Decision: decision, SystemSource: sql.NullString{String: "test", Valid: true},
			Confidence: sql.NullFloat64{Float64: score, Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	p1, p2, p3 := pair(trace1, "101"), pair(trace1, "102"), pair(trace2, "103")
	system(p1, "POSITIVE", 0.91)
	system(p2, "INCONCLUSIVE", 0.65)
	system(p3, "INCONCLUSIVE", 0.62)
	system(pair(trace2, "TRACE:999999#feature"), "INCONCLUSIVE", 0.70) // another case's face: not examined here

	check := func(what string, got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}
	distribute := func(want string) {
		t.Helper()
		res, err := distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 1})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, a := range res.Assigned {
			got = append(got, a.CaseID+":"+string(a.Step)+":"+a.Username)
		}
		if fmt.Sprint(got) != want {
			t.Fatalf("distribution = %v, want %s", got, want)
		}
	}
	worksheet := func(u auth.UserProfile, id int64) examination.Worksheet {
		t.Helper()
		ws, err := examination.GetWorksheet(ctx, sqlDB, u, id)
		if err != nil {
			t.Fatal(err)
		}
		return ws
	}
	draft := func(u auth.UserProfile, id int64, p examination.Pair, decision string) error {
		return examination.SaveDraft(ctx, sqlDB, u, id, p, decision, "")
	}

	// EXAMINATION: ana, the least-loaded facial examiner.
	distribute("[F1:VERIFICATOR:ana]")
	review := worksheet(ana, mustReview(t, sqlDB, "F1"))
	if review.YourStep != analyses.StepVerificator || len(review.Pairs) != 3 || review.Pairs[0].Pair != p1 || *review.Pairs[0].Score != 0.91 {
		t.Fatalf("ana's worksheet = %+v; want the 3 candidates of the case's faces, best score first", review)
	}
	id := review.AnalysisID
	var incomplete examination.IncompleteError
	if _, err := examination.Submit(ctx, sqlDB, ana, id); !errors.As(err, &incomplete) || incomplete.Missing != 3 {
		t.Fatalf("submitting with nothing decided: %v", err)
	}
	check("ana drafts p1", draft(ana, id, p1, "POSITIVE"), nil)
	check("ana drafts p2", draft(ana, id, p2, "NEGATIVE"), nil)
	check("ana drafts p3", draft(ana, id, p3, "POSITIVE"), nil)
	check("ana changes her mind on p3", draft(ana, id, p3, "INCONCLUSIVE"), nil)
	check("a pair outside the analysis", draft(ana, id, pair(trace2, "TRACE:999999#feature"), "POSITIVE"), examination.ErrNotInScope)
	check("an invalid decision", draft(ana, id, p1, "MAYBE"), examination.ErrInvalidDecision)
	check("bia drafting on ana's step", draft(bia, id, p1, "POSITIVE"), analyses.ErrCannotDecide)
	check("the admin drafting", draft(chief, id, p1, "POSITIVE"), analyses.ErrCannotDecide)
	if ws := worksheet(chief, id); ws.YourStep != "" || ws.Drafted != 0 || ws.Pairs[0].Draft != nil {
		t.Fatalf("the admin sees ana's drafts: %+v", ws)
	}
	res, err := examination.Submit(ctx, sqlDB, ana, id)
	if err != nil || res.Decisions != 3 || res.Stage != analyses.StageVerification {
		t.Fatalf("ana's submit = %+v, %v", res, err)
	}
	check("ana drafting after submitting", draft(ana, id, p1, "NEGATIVE"), analyses.ErrCannotDecide)

	// VERIFICATION: never ana; bia, blind: the images and nothing else.
	distribute("[F1:REVIEWER:bia]")
	if ws := worksheet(bia, id); ws.YourStep != analyses.StepReviewer || len(ws.Pairs) != 3 {
		t.Fatalf("bia's worksheet = %+v; want 3 pairs", ws)
	}
	assertBlind(t, "bia", worksheet(bia, id))
	blindIn := func(who string, u auth.UserProfile) bool {
		t.Helper()
		blind, err := distribution.BlindIn(ctx, sqlDB, u.ID, "F1")
		if err != nil {
			t.Fatal(err)
		}
		return blind
	}
	if !blindIn("bia", bia) || blindIn("ana", ana) {
		t.Fatalf("BlindIn: bia %v, ana %v; want true, false", blindIn("bia", bia), blindIn("ana", ana))
	}
	if mine, err := distribution.AnalysesOf(ctx, sqlDB, bia.ID); err != nil || len(mine) != 1 || !mine[0].Blind || mine[0].Description != "" || mine[0].CaseType != "" {
		t.Fatalf("bia's analyses = %+v, %v; want one blind row with no case type or description", mine, err)
	}
	if ws := worksheet(chief, id); len(ws.Pairs[0].Decisions) != 1 || ws.Pairs[0].Decisions[0].Username != "ana" {
		t.Fatalf("the admin's view = %+v; want ana's decisions", ws.Pairs[0].Decisions)
	}
	check("bia p1", draft(bia, id, p1, "POSITIVE"), nil)
	check("bia p2", draft(bia, id, p2, "NEGATIVE"), nil)
	check("bia p3", draft(bia, id, p3, "POSITIVE"), nil) // ana said INCONCLUSIVE
	if res, err := examination.Submit(ctx, sqlDB, bia, id); err != nil || res.Stage != analyses.StageInconsistence {
		t.Fatalf("bia's submit = %+v, %v; want INCONSISTENCE", res, err)
	}

	// INCONSISTENCE: neither ana nor bia; caio, on the disputed pair only, blind as bia was: neither
	// decision, nor who made them, can lead him.
	distribute("[F1:INCONSISTENCE:caio]")
	ws := worksheet(caio, id)
	if ws.YourStep != analyses.StepInconsistence || len(ws.Pairs) != 1 || ws.Pairs[0].Pair != p3 {
		t.Fatalf("caio's worksheet = %+v; want p3 only", ws)
	}
	assertBlind(t, "caio", ws)
	if _, err := examination.PairImage(ctx, sqlDB, nil, caio, id, "TRACE:999999#feature"); !errors.Is(err, examination.ErrNotInScope) {
		t.Fatalf("an image outside the analysis: %v", err)
	}
	if !blindIn("caio", caio) || blindIn("bia", bia) {
		t.Fatalf("BlindIn: caio %v, bia %v; want true, false", blindIn("caio", caio), blindIn("bia", bia))
	}
	if ws := worksheet(chief, id); !ws.Blind && (len(ws.Pairs[2].Decisions) != 2 || !ws.Pairs[2].Disputed) {
		t.Fatalf("the admin's view of p3 = %+v; want both decisions, disputed", ws.Pairs[2])
	}
	check("caio p3", draft(caio, id, p3, "POSITIVE"), nil)
	if res, err := examination.Submit(ctx, sqlDB, caio, id); err != nil || res.Decisions != 1 || res.Stage != analyses.StageReport {
		t.Fatalf("caio's submit = %+v, %v; want REPORT", res, err)
	}
	if rows, err := q.ListAnalysisDecisions(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil || len(rows) != 7 {
		t.Fatalf("decisions recorded = %d, %v; want 3 + 3 + 1", len(rows), err)
	}
	check("caio opens F1", distribution.CanOpen(ctx, sqlDB, caio, "F1"), nil)
	if blindIn("caio", caio) {
		t.Fatal("caio still blind after submitting")
	}
	check("duda does not", distribution.CanOpen(ctx, sqlDB, duda, "F1"), distribution.ErrNotAssigned)

	// REPORT: only ana, the first examiner.
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if _, err := analyses.IssueReport(ctx, sqlDB, id, caio, now); !errors.Is(err, analyses.ErrCannotDecide) {
		t.Fatalf("caio issuing the report: %v", err)
	}
	if report, err := analyses.IssueReport(ctx, sqlDB, id, ana, now); err != nil || report.Number != "2026.0001" {
		t.Fatalf("ana's report = %+v, %v", report, err)
	}

	// A comparison: manual assignment, the never-two-steps rule, and a reassignment discarding drafts.
	cmp, err := analyses.Open(ctx, sqlDB, "F1", analyses.FacialComparison, []analyses.Pair{{FeatureA: trace1, FeatureB: "101"}}, chief.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analyses.Open(ctx, sqlDB, "F1", analyses.FacialComparison, []analyses.Pair{{FeatureA: trace1, FeatureB: "TRACE:999999#feature"}}, chief.ID); !errors.Is(err, analyses.ErrInvalidPair) {
		t.Fatalf("a comparison with another case's face: %v", err)
	}
	if _, err := analyses.IssueReport(ctx, sqlDB, cmp, ana, now); !errors.Is(err, analyses.ErrNotReportStage) {
		t.Fatalf("issuing before deciding: %v", err)
	}
	check("comparison to ana", distribution.Assign(ctx, sqlDB, cmp, &ana.ID, chief.ID), nil)
	check("ana decides it", draft(ana, cmp, p1, "POSITIVE"), nil)
	if _, err := examination.Submit(ctx, sqlDB, ana, cmp); err != nil {
		t.Fatal(err)
	}
	check("verification to ana herself", distribution.Assign(ctx, sqlDB, cmp, &ana.ID, chief.ID), distribution.ErrOtherStep)
	check("verification to a fingerprint examiner", distribution.Assign(ctx, sqlDB, cmp, &duda.ID, chief.ID), distribution.ErrNotExaminer)
	check("verification to bia", distribution.Assign(ctx, sqlDB, cmp, &bia.ID, chief.ID), nil)
	check("bia drafts", draft(bia, cmp, p1, "NEGATIVE"), nil)
	check("reassigned to caio", distribution.Assign(ctx, sqlDB, cmp, &caio.ID, chief.ID), nil)
	if ws := worksheet(caio, cmp); ws.YourStep != analyses.StepReviewer || ws.Drafted != 0 {
		t.Fatalf("caio's worksheet after the reassignment = %+v; want bia's draft gone", ws)
	}

	// A case with no faces: its review has nothing to decide and goes straight to REPORT.
	if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "F2", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "empty"}); err != nil {
		t.Fatal(err)
	}
	empty := mustReview(t, sqlDB, "F2")
	check("F2 to bia", distribution.Assign(ctx, sqlDB, empty, &bia.ID, chief.ID), nil)
	if res, err := examination.Submit(ctx, sqlDB, bia, empty); err != nil || res.Decisions != 0 || res.Stage != analyses.StageReport {
		t.Fatalf("submitting an empty review = %+v, %v; want REPORT", res, err)
	}
}

// assertBlind checks that a blind examiner's worksheet holds the two images of each pair and
// nothing else.
func assertBlind(t *testing.T, who string, ws examination.Worksheet) {
	t.Helper()
	if !ws.Blind {
		t.Fatalf("%s's worksheet is not blind", who)
	}
	for _, p := range ws.Pairs {
		if len(p.Decisions) != 0 || p.Disputed || p.Score != nil || p.SystemDecision != "" {
			t.Fatalf("%s sees decisions or the matcher: %+v", who, p)
		}
		for _, l := range []cases.TraceLink{p.Questioned, p.Counterpart} {
			if l.FeatureID == "" || l.Description != "" || l.Name != "" || l.DecidedBy != "" || l.Modality != "" ||
				l.PersonID != "" || l.IdentityFileID != 0 || l.CaseID != "" || l.TraceID != 0 || l.ThumbnailFileID != 0 || l.ThumbnailBox != nil {
				t.Fatalf("%s gets more than the feature id: %+v", who, l)
			}
		}
	}
}

// mustReview returns the id of a case's automatic FACIAL_REVIEW.
func mustReview(t *testing.T, sqlDB *sql.DB, caseID string) int64 {
	t.Helper()
	list, err := analyses.List(context.Background(), sqlDB, caseID)
	if err != nil || len(list) == 0 || list[0].Type != analyses.FacialReview {
		t.Fatalf("%s analyses = %+v, %v", caseID, list, err)
	}
	return list[0].ID
}

// TestAutomaticReviewAndAccess: a FACIAL case gets one review however often it is ingested, a
// FINGERPRINT case none; admins open every case and examiners only those they hold a step of.
func TestAutomaticReviewAndAccess(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	chief, err := repo.Create(ctx, auth.NewUser{Nome: "c", UltimoNome: "c", Matricula: "c", Cargo: "c", Username: "chief", Password: "password-1",
		Access: auth.Access{AccountRole: auth.AccountRoleAdmin, IsActive: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	ana, err := repo.Create(ctx, auth.NewUser{Nome: "a", UltimoNome: "a", Matricula: "a", Cargo: "a", Username: "ana", Password: "password-1",
		Access: auth.Access{AccountRole: auth.AccountRoleUser, ExaminesFacial: true, IsActive: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []cases.CaseInput{
		{CaseID: "F1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "x"},
		{CaseID: "F1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "x"},
		{CaseID: "P1", CaseType: cases.CaseTypeCriminal, Modality: "FINGERPRINT", Description: "x"},
	} {
		if _, err := cases.Ingest(ctx, sqlDB, in); err != nil {
			t.Fatal(err)
		}
	}
	if list, err := analyses.List(ctx, sqlDB, "F1"); err != nil || len(list) != 1 {
		t.Fatalf("F1 analyses = %+v, %v; want one review", list, err)
	}
	if list, err := analyses.List(ctx, sqlDB, "P1"); err != nil || len(list) != 0 {
		t.Fatalf("P1 analyses = %+v, %v; want none", list, err)
	}
	if err := distribution.CanOpen(ctx, sqlDB, chief, "P1"); err != nil {
		t.Fatalf("the admin opening P1: %v", err)
	}
	if err := distribution.CanOpen(ctx, sqlDB, ana, "F1"); !errors.Is(err, distribution.ErrNotAssigned) {
		t.Fatalf("ana opening F1 before holding anything: %v", err)
	}
	if _, err := distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 1}); err != nil {
		t.Fatal(err)
	}
	if err := distribution.CanOpen(ctx, sqlDB, ana, "F1"); err != nil {
		t.Fatalf("ana opening F1 once distributed: %v", err)
	}
	if err := distribution.CanOpen(ctx, sqlDB, chief, "NOPE"); !errors.Is(err, analyses.ErrCaseNotFound) {
		t.Fatalf("an unknown case: %v", err)
	}
}
