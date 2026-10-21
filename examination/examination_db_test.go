package examination_test

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

// TestAgreementAndComparison covers what distribution's TestDecisionFlow does not: a review whose
// two examiners agree goes straight to REPORT, where nothing more can be drafted or submitted; a
// comparison examines pairs the matcher never proposed (two faces of the case, a face and an
// enrolled person) next to one it did; and the third examiner may decide, and see the images of,
// only the disputed pair.
func TestAgreementAndComparison(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	q := db.New(sqlDB)
	user := func(username string, role auth.AccountRole, facial bool) auth.UserProfile {
		t.Helper()
		u, err := repo.Create(ctx, auth.NewUser{FirstName: username, LastName: "Test", StaffNumber: username, JobTitle: "Perito",
			Username: username, Password: "password-1",
			Access: auth.Access{AccountRole: role, ExaminesFacial: facial, IsActive: true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	chief := user("chief", auth.AccountRoleSuperAdmin, false)
	ana := user("ana", auth.AccountRoleUser, true)
	bia := user("bia", auth.AccountRoleUser, true)
	caio := user("caio", auth.AccountRoleUser, true)

	ev := cases.EvidenceInput{Sequence: 1, Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_CAPTURE"}, {Sequence: 2, TraceType: "FACE_CAPTURE"}}}
	c1, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "C1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "test",
		Evidences: []cases.EvidenceInput{ev}})
	if err != nil {
		t.Fatal(err)
	}
	t1 := graph.QuestionedFeatureID(c1.Evidences[0].Traces[0].TraceID)
	t2 := graph.QuestionedFeatureID(c1.Evidences[0].Traces[1].TraceID)
	pair := func(a, b string) examination.Pair {
		if a > b {
			a, b = b, a
		}
		return examination.Pair{FeatureA: a, FeatureB: b}
	}
	pA, pB := pair(t1, "201"), pair(t2, "202")
	for _, s := range []struct {
		p        examination.Pair
		decision string
		score    float64
	}{{pA, examination.Positive, 0.91}, {pB, examination.Inconclusive, 0.65}} {
		if _, err := q.InsertBiometricDecision(ctx, db.InsertBiometricDecisionParams{FeatureAID: s.p.FeatureA, FeatureBID: s.p.FeatureB,
			Modality: "FACIAL", Role: "SYSTEM", Decision: s.decision, SystemSource: sql.NullString{String: "test", Valid: true},
			Confidence: sql.NullFloat64{Float64: s.score, Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}

	check := func(what string, got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}
	assign := func(id int64, u auth.UserProfile) {
		t.Helper()
		check("assigning "+u.Username, distribution.Assign(ctx, sqlDB, id, &u.ID, chief.ID), nil)
	}
	draft := func(u auth.UserProfile, id int64, p examination.Pair, decision string) error {
		return examination.SaveDraft(ctx, sqlDB, u, id, p, decision, "")
	}
	submit := func(u auth.UserProfile, id int64, decisions int, stage analyses.Stage) {
		t.Helper()
		res, err := examination.Submit(ctx, sqlDB, u, id)
		if err != nil || res.Decisions != decisions || res.Stage != stage {
			t.Fatalf("%s's submit = %+v, %v; want %d decisions, %s", u.Username, res, err, decisions, stage)
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

	// A review where both examiners agree: no third examiner.
	list, err := analyses.List(ctx, sqlDB, "C1")
	if err != nil || len(list) != 1 {
		t.Fatalf("C1 analyses = %+v, %v; want its review", list, err)
	}
	review := list[0].ID
	assign(review, ana)
	check("ana drafts pA, given the other way round", draft(ana, review, examination.Pair{FeatureA: pA.FeatureB, FeatureB: pA.FeatureA}, examination.Positive), nil)
	check("ana drafts pB", draft(ana, review, pB, examination.Negative), nil)
	submit(ana, review, 2, analyses.StageVerification)
	assign(review, bia)
	check("bia agrees on pA", draft(bia, review, pA, examination.Positive), nil)
	check("bia agrees on pB", draft(bia, review, pB, examination.Negative), nil)
	submit(bia, review, 2, analyses.StageReport)

	check("ana drafting at REPORT", draft(ana, review, pA, examination.Negative), examination.ErrNothingToSubmit)
	if _, err := examination.Submit(ctx, sqlDB, ana, review); !errors.Is(err, examination.ErrNothingToSubmit) {
		t.Fatalf("ana submitting at REPORT: %v", err)
	}
	check("bia drafting at REPORT", draft(bia, review, pA, examination.Negative), analyses.ErrCannotDecide)
	ws := worksheet(ana, review)
	if ws.YourStep != "" || ws.Blind || len(ws.Pairs) != 2 {
		t.Fatalf("ana's worksheet at REPORT = %+v; want both pairs, nothing to decide, not blind", ws)
	}
	for _, p := range ws.Pairs {
		if p.Disputed || len(p.Decisions) != 2 || p.Decisions[0].Step != analyses.StepVerificator || p.Decisions[1].Step != analyses.StepReviewer {
			t.Fatalf("pair %+v at REPORT; want both decisions in step order, undisputed", p)
		}
	}
	if ws := worksheet(bia, review); ws.Blind {
		t.Fatal("bia still blind once the analysis reached REPORT")
	}

	// A comparison: two pairs the matcher never proposed, and pA, which it did.
	within, enrolled := pair(t1, t2), pair(t2, "203")
	cmp, err := analyses.Open(ctx, sqlDB, "C1", analyses.FacialComparison,
		[]analyses.Pair{{FeatureA: t1, FeatureB: t2}, {FeatureA: t2, FeatureB: "203"}, {FeatureA: t1, FeatureB: "201"}}, chief.ID)
	if err != nil {
		t.Fatal(err)
	}
	assign(cmp, ana)
	ws = worksheet(ana, cmp)
	got := map[examination.Pair]examination.PairView{}
	for _, p := range ws.Pairs {
		got[p.Pair] = p
	}
	if len(got) != 3 || got[pA].Score == nil || *got[pA].Score != 0.91 || got[pA].SystemDecision != examination.Positive ||
		got[within].Score != nil || got[within].SystemDecision != "" || got[enrolled].Score != nil {
		t.Fatalf("ana's comparison = %+v; want the 3 listed pairs, the matcher's score on pA only", ws.Pairs)
	}
	if v := got[enrolled]; v.Questioned.FeatureID != t2 || v.Counterpart.FeatureID != "203" {
		t.Fatalf("the enrolled pair shows %q against %q; want the case's face as the questioned side", v.Questioned.FeatureID, v.Counterpart.FeatureID)
	}
	check("ana drafts the two faces", draft(ana, cmp, within, examination.Negative), nil)
	check("ana drafts the enrolled person", draft(ana, cmp, enrolled, examination.Positive), nil)
	check("ana drafts pA", draft(ana, cmp, pA, examination.Positive), nil)
	check("a pair the comparison does not list", draft(ana, cmp, pB, examination.Positive), examination.ErrNotInScope)
	submit(ana, cmp, 3, analyses.StageVerification)

	// caio disagrees on one pair only; the third examiner gets that one.
	assign(cmp, caio)
	check("caio, the two faces", draft(caio, cmp, within, examination.Negative), nil)
	check("caio, the enrolled person", draft(caio, cmp, enrolled, examination.Inconclusive), nil)
	check("caio, pA", draft(caio, cmp, pA, examination.Positive), nil)
	submit(caio, cmp, 3, analyses.StageInconsistence)
	assign(cmp, bia)
	if ws := worksheet(bia, cmp); !ws.Blind || len(ws.Pairs) != 1 || ws.Pairs[0].Pair != enrolled {
		t.Fatalf("bia's worksheet = %+v; want the disputed pair only, blind", ws)
	}
	check("bia on a settled pair", draft(bia, cmp, within, examination.Positive), examination.ErrNotInScope)
	if _, err := examination.PairImage(ctx, sqlDB, nil, bia, cmp, t1); !errors.Is(err, examination.ErrNotInScope) {
		t.Fatalf("bia asking for a face of the settled pairs: %v", err)
	}
	if _, err := examination.PairImage(ctx, sqlDB, nil, bia, cmp, "203"); !errors.Is(err, examination.ErrNoImage) {
		t.Fatalf("bia asking for the disputed pair's enrolled side, which has no file: %v; want ErrNoImage", err)
	}
	check("bia settles it", draft(bia, cmp, enrolled, examination.Positive), nil)
	submit(bia, cmp, 1, analyses.StageReport)

	if rows, err := q.ListAnalysisDecisions(ctx, sql.NullInt64{Int64: cmp, Valid: true}); err != nil || len(rows) != 7 {
		t.Fatalf("comparison decisions = %d, %v; want 3 + 3 + 1", len(rows), err)
	}
}
