package reports_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/examination"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/reports"
	"github.com/rodfileto/trackid/requests"
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

// memStore is object storage in memory.
type memStore map[string][]byte

func (m memStore) Upload(_ context.Context, key string, data []byte, _ string) (string, error) {
	m[key] = append([]byte(nil), data...)
	return key, nil
}

func (m memStore) Download(_ context.Context, ref string) ([]byte, error) {
	b, ok := m[ref]
	if !ok {
		return nil, errors.New("no such object")
	}
	return b, nil
}

// fakePDF records the document it was asked to render.
type fakePDF struct{ last []byte }

func (f *fakePDF) PDF(_ context.Context, docx []byte) ([]byte, error) {
	f.last = docx
	return []byte("%PDF-1.7 fake"), nil
}

// TestLaudoFlow walks a laudo from the request to its delivery: a request with questions, a
// reference sent in, suitability, a comparison decided on the conclusion scale, then the report --
// generated, refused while tampered or incomplete, refreshed after the data changed, issued with
// its number in the document, and delivered.
func TestLaudoFlow(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	q := db.New(sqlDB)
	user := func(username string, facial, coordinator bool) auth.UserProfile {
		t.Helper()
		u, err := repo.Create(ctx, auth.NewUser{FirstName: strings.ToUpper(username[:1]) + username[1:], LastName: "Teste", StaffNumber: "M-" + username,
			JobTitle: "Perito", Username: username, Password: "password-1",
			Access: auth.Access{AccountRole: auth.AccountRoleUser, ExaminesFacial: facial, Coordinator: coordinator, IsActive: true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	chief := user("chief", false, true)
	ana := user("ana", true, false)
	caio := user("caio", true, false)
	check := func(what string, got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}

	ev := []cases.EvidenceInput{
		{Sequence: 1, Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_CAPTURE"}}},
		{Sequence: 2, Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_CAPTURE"}}},
	}
	c, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "L1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "Roubo", Evidences: ev})
	if err != nil {
		t.Fatal(err)
	}
	t1, t2 := c.Evidences[0].Traces[0].TraceID, c.Evidences[1].Traces[0].TraceID
	f1, f2 := graph.QuestionedFeatureID(t1), graph.QuestionedFeatureID(t2)

	// The request: unknown cited number, two questions, asking a facial comparison.
	r, err := requests.Create(ctx, sqlDB, "L1", requests.Input{Authority: "37ª DP", DocumentReference: "Ofício 456/2026", ReceivedOn: "2026-10-20",
		DueOn: "2026-10-30", CitedReference: "IP 999/2026", Questions: []string{"Mesma pessoa?", " ", "Há disfarce?"}, Task: "FACIAL_COMPARISON"}, chief.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Sequence != 1 || r.Occurrence != nil || len(r.Questions) != 2 || r.CitedReference != "IP 999/2026" {
		t.Fatalf("request = %+v; want #1, unlinked, two questions", r)
	}
	if _, err := requests.Create(ctx, sqlDB, "L1", requests.Input{Authority: "x", DocumentReference: "y", ReceivedOn: "2026-10-20", DueOn: "2026-10-01"}, chief.ID); err == nil {
		t.Fatal("a due date before receipt was accepted")
	}
	list, err := analyses.List(ctx, sqlDB, "L1")
	if err != nil || len(list) != 1 || list[0].Type != analyses.FacialComparison || list[0].RequestID != r.ID || len(list[0].Pairs) != 0 {
		t.Fatalf("the request's comparison should be open, empty, answering it: %+v, %v", list, err)
	}
	cmp := list[0].ID

	// The second face is a reference sent in; the coordinator adds the pair to the comparison.
	check("reference", cases.SetTraceRole(ctx, sqlDB, "L1", t2, cases.RoleReference), nil)
	if err := cases.SetTraceRole(ctx, sqlDB, "L1", t2, "SUSPECT"); err == nil {
		t.Fatal("an unknown role was accepted")
	}
	check("a pair of one face", analyses.AddPairs(ctx, sqlDB, "L1", cmp, []analyses.Pair{{FeatureA: f1, FeatureB: f1}}), analyses.ErrInvalidPair)
	check("add the pair", analyses.AddPairs(ctx, sqlDB, "L1", cmp, []analyses.Pair{{FeatureA: f1, FeatureB: f2}}), nil)
	check("add it again", analyses.AddPairs(ctx, sqlDB, "L1", cmp, []analyses.Pair{{FeatureA: f2, FeatureB: f1}}), nil)
	if list, err := analyses.List(ctx, sqlDB, "L1"); err != nil || len(list[0].Pairs) != 1 {
		t.Fatalf("pairs after adding: %+v, %v; want one", list, err)
	}
	check("ana takes it", distribution.Assign(ctx, sqlDB, cmp, &ana.ID, chief.ID), nil)
	check("assess the reference", examination.AssessTrace(ctx, sqlDB, ana, cmp, t2, examination.Suitable, ""), examination.ErrNotQuestioned)
	check("caio assessing", examination.AssessTrace(ctx, sqlDB, caio, cmp, t1, examination.Suitable, ""), analyses.ErrCannotDecide)
	check("ana assesses", examination.AssessTrace(ctx, sqlDB, ana, cmp, t1, examination.Suitable, "frontal"), nil)
	check("role locked? not yet", cases.SetTraceRole(ctx, sqlDB, "L1", t2, cases.RoleReference), nil)

	p := examination.Pair{FeatureA: f1, FeatureB: f2}
	if p.FeatureA > p.FeatureB {
		p.FeatureA, p.FeatureB = p.FeatureB, p.FeatureA
	}
	check("no level", examination.SaveDraft(ctx, sqlDB, ana, cmp, p, "POSITIVE", "", ""), examination.ErrNeedsConclusion)
	check("a level of the other side", examination.SaveDraft(ctx, sqlDB, ana, cmp, p, "POSITIVE", "", cluster.ConclusionStrongSupportDifferent), examination.ErrNeedsConclusion)
	check("ana", examination.SaveDraft(ctx, sqlDB, ana, cmp, p, "POSITIVE", "", cluster.ConclusionStrongSupportCommon), nil)
	if _, err := examination.Submit(ctx, sqlDB, ana, cmp); err != nil {
		t.Fatal(err)
	}
	check("caio verifies", distribution.Assign(ctx, sqlDB, cmp, &caio.ID, chief.ID), nil)
	check("caio", examination.SaveDraft(ctx, sqlDB, caio, cmp, p, "POSITIVE", "", cluster.ConclusionStrongSupportCommon), nil)
	if res, err := examination.Submit(ctx, sqlDB, caio, cmp); err != nil || res.Stage != analyses.StageReport {
		t.Fatalf("caio's submit = %+v, %v; want REPORT", res, err)
	}
	check("role is locked now", cases.SetTraceRole(ctx, sqlDB, "L1", t2, cases.RoleQuestioned), cases.ErrRoleLocked)

	store := memStore{}
	pdf := &fakePDF{}
	images := func(_ context.Context, featureID string) (reports.Image, error) {
		return face(color.RGBA{uint8(len(featureID) * 10), 120, 100, 255}), nil
	}
	now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	svc := reports.Service{DB: sqlDB, Store: store, Images: images, Renderer: pdf, Now: func() time.Time { return now }}

	_, _, err = svc.Draft(ctx, caio, cmp)
	check("caio's draft", err, analyses.ErrCannotDecide)
	v, draft, err := svc.Draft(ctx, ana, cmp)
	if err != nil || v.Version != 1 || !v.Generated || v.Stale {
		t.Fatalf("ana's first draft = %+v, %v", v, err)
	}
	var missing reports.MissingSectionsError
	if _, err := svc.Issue(ctx, ana, cmp); !errors.As(err, &missing) || len(missing.Sections) != 3 {
		t.Fatalf("issuing an empty laudo: %v; want three missing sections (one exam, two answers)", err)
	}
	tampered, err := reports.TamperForTest(draft, "Roubo", "Furto")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Save(ctx, ana, cmp, tampered)
	check("tampered save", err, reports.ErrLockedChanged)
	// Regions of a laudo with one pair and two questions: objective, material, method, exam 1,
	// discussion, answer 1, answer 2.
	filled, err := reports.FillRegionsForTest(draft, map[int]string{4: "Nariz e orelhas concordantes.", 6: "Sim.", 7: "Não."})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := svc.Save(ctx, ana, cmp, filled); err != nil || v.Version != 2 || v.Generated {
		t.Fatalf("ana's save = %+v, %v", v, err)
	}

	// The data changes after the draft: issuing waits for a refresh, which keeps the text.
	check("re-assess", examination.AssessTrace(ctx, sqlDB, ana, cmp, t1, examination.LeadOnly, "perfil"), nil)
	_, err = svc.Issue(ctx, ana, cmp)
	check("stale", err, reports.ErrStale)
	if v, _, err := svc.Draft(ctx, ana, cmp); err != nil || !v.Stale {
		t.Fatalf("the draft should say it is stale: %+v, %v", v, err)
	}
	if v, err := svc.Refresh(ctx, ana, cmp); err != nil || v.Version != 3 || !v.Generated {
		t.Fatalf("refresh = %+v, %v", v, err)
	}
	o, err := svc.Get(ctx, cmp)
	if err != nil || o.Status != "DRAFT" || !o.Summary.Preliminary || len(o.Summary.Results) != 1 || o.Summary.Results[0].ResultCode != reports.ResultIdentified {
		t.Fatalf("the draft's overview = %+v, %v", o, err)
	}

	rep, err := svc.Issue(ctx, ana, cmp)
	if err != nil || rep.Number != "2026.0001" || rep.Type != "FACIAL_COMPARISON_REPORT" {
		t.Fatalf("issue = %+v, %v", rep, err)
	}
	text, err := reports.TextForTest(pdf.last)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "2026.0001") || !strings.Contains(text, "26/10/2026") || strings.Contains(text, "atribuído na emissão") ||
		!strings.Contains(text, "Nariz e orelhas concordantes.") {
		t.Fatal("the rendered document should carry the number, the date and the examiner's text, and no placeholder")
	}
	_, err = svc.Issue(ctx, ana, cmp)
	check("issue twice", err, analyses.ErrNotDraft)
	o, err = svc.Get(ctx, cmp)
	if err != nil || o.Status != "ISSUED" || o.Number != "2026.0001" || o.Summary.Preliminary || o.PDFSHA256 == "" {
		t.Fatalf("the issued overview = %+v, %v", o, err)
	}
	data, name, ct, err := svc.Document(ctx, cmp, "pdf")
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) || ct != "application/pdf" || !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("the PDF = %q %q, %v", name, ct, err)
	}
	if err := svc.Deliver(ctx, ana, cmp, reports.Delivery{DeliveredOn: "2026-10-27", Method: "SEI", Recipient: "37ª DP"}); err != nil {
		t.Fatal(err)
	}
	if o, err := svc.Get(ctx, cmp); err != nil || len(o.Deliveries) != 1 || o.Deliveries[0].RecordedBy != "ana" {
		t.Fatalf("deliveries = %+v, %v", o.Deliveries, err)
	}
	if a, err := analyses.List(ctx, sqlDB, "L1"); err != nil || a[0].Status != analyses.StatusClosed || a[0].RequestID != r.ID {
		t.Fatalf("the comparison should be closed and answer the request: %+v, %v", a, err)
	}

	// A laudo complementar of it.
	cmp2, err := analyses.OpenWith(ctx, sqlDB, "L1", analyses.FacialComparison, []analyses.Pair{{FeatureA: f1, FeatureB: f2}}, chief.ID,
		analyses.Options{Complements: cmp})
	if err != nil {
		t.Fatal(err)
	}
	if a, err := analyses.List(ctx, sqlDB, "L1"); err != nil || a[1].ID != cmp2 || a[1].Report.Complements != "2026.0001" {
		t.Fatalf("the complement should name the report it complements: %+v, %v", a, err)
	}
	if _, err := analyses.OpenWith(ctx, sqlDB, "L1", analyses.FacialComparison, []analyses.Pair{{FeatureA: f1, FeatureB: f2}}, chief.ID,
		analyses.Options{Complements: cmp2}); !errors.Is(err, analyses.ErrBadComplement) {
		t.Fatalf("complementing a draft: %v", err)
	}
	_ = q
}
