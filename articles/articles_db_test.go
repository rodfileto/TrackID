package articles_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/articles"
	"github.com/rodfileto/trackid/db"
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

// world is the fixture every article test writes about: two persons with a name each, two
// analysts, a case with a request and an issued laudo, an occurrence under an inquiry, and a
// cluster merged into another.
type world struct {
	db                  *sql.DB
	alice, bruno        int64 // analysts
	occurrence          int64
	request, report     int64
	keptCluster, merged int64
	caseID, inquiry     string
	personA, personB    string
	nameA, nameB        string
	ctx                 context.Context
	exec                func(query string, args ...any) int64
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	sqlDB := testDB(t)
	w := &world{db: sqlDB, ctx: ctx, caseID: "2026.0042", inquiry: "4512/2026",
		personA: "P-A", personB: "P-B", nameA: "JOAO DA SILVA", nameB: "MARIA SOUZA"}
	w.exec = func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRowContext(ctx, query+" RETURNING id", args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}
	user := func(name string) int64 {
		return w.exec(`INSERT INTO users (first_name, last_name, staff_number, job_title, username, email, password_hash, intelligence_analyst)
			VALUES ($1, 'Analyst', $1, 'analyst', $1, $1 || '@x', 'x', true)`, name)
	}
	w.alice, w.bruno = user("alice"), user("bruno")

	for _, p := range []struct{ id, name, doc string }{{w.personA, w.nameA, "RG-1"}, {w.personB, w.nameB, "RG-2"}} {
		person := w.exec(`INSERT INTO persons (person_id) VALUES ($1)`, p.id)
		doc := w.exec(`INSERT INTO identity_documents (person_id, document_number, document_type) VALUES ($1, $2, 'RG')`, person, p.doc)
		w.exec(`INSERT INTO identity_registers (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender)
			VALUES ($1, $2, $3, 'X', 'M', 'Y', 'F')`, doc, p.doc+"-R", p.name)
	}

	q := db.New(sqlDB)
	loc, err := q.UpsertLocation(ctx, db.UpsertLocationParams{LocationID: "LOC-1", Lon: -43.2, Lat: -22.9, Precision: "EXACT",
		MunicipalityCode: "3304557", Uf: "RJ"})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 3, 14, 22, 0, 0, 0, time.UTC)
	w.occurrence = w.exec(`INSERT INTO occurrences (agency, occurrence_number, inquiry_number, location_id, occurred_from, registered_at, status, description)
		VALUES ('DP-01', '123/2026', $1, $2, $3, $3, 'INQUIRY_OPENED', 'bank robbery')`, w.inquiry, loc.ID, when)
	w.exec(`INSERT INTO occurrences (agency, occurrence_number, inquiry_number, location_id, occurred_from, registered_at, status, description)
		VALUES ('DP-01', '124/2026', $1, $2, $3, $3, 'INQUIRY_OPENED', 'second robbery')`, w.inquiry, loc.ID, when.Add(48*time.Hour))

	bc := w.exec(`INSERT INTO biometric_cases (case_id, case_type, modality, description, occurrence_id)
		VALUES ($1, 'CRIMINAL', 'FACIAL', 'robbery faces', $2)`, w.caseID, w.occurrence)
	w.request = w.exec(`INSERT INTO case_requests (authority, document_reference, received_on) VALUES ('DP-01', 'Ofício 77/2026', '2026-03-20')`)
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO case_request_cases (case_request_id, biometric_case_id, sequence) VALUES ($1, $2, 1)`, w.request, bc); err != nil {
		t.Fatal(err)
	}
	analysis := w.exec(`INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type) VALUES ($1, 1, 'FACIAL_COMPARISON')`, bc)
	w.report = w.exec(`INSERT INTO analysis_reports (case_analysis_id, report_type, status, report_year, report_number, issued_by, issued_at, snapshot)
		VALUES ($1, 'FACIAL_COMPARISON_REPORT', 'ISSUED', 2026, 9, $2, NOW(), '{}')`, analysis, w.alice)

	w.keptCluster = w.exec(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'REVIEWED')`)
	// A cluster merged away: its row is gone, its merge recorded.
	w.merged = w.keptCluster + 100
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO cluster_merges (basis, from_cluster_id, to_cluster_id) VALUES ('REVIEWED', $1, $2)`,
		w.merged, w.keptCluster); err != nil {
		t.Fatal(err)
	}
	return w
}

// doc builds an article body: one paragraph of text and mentions, written "text" or
// "@KIND:ref".
func doc(parts ...string) json.RawMessage {
	var inline []map[string]any
	for _, p := range parts {
		if strings.HasPrefix(p, "@") {
			kind, ref, _ := strings.Cut(p[1:], ":")
			inline = append(inline, map[string]any{"type": "mention", "attrs": map[string]any{"kind": kind, "ref": ref, "label": ref}})
			continue
		}
		inline = append(inline, map[string]any{"type": "text", "text": p})
	}
	body, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": inline}}})
	return body
}

func TestSaveAndRead(t *testing.T) {
	w := newWorld(t)
	ctx, sqlDB := w.ctx, w.db
	subject := articles.PersonSubject(w.personA)

	empty, err := articles.Get(ctx, sqlDB, subject)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Revision != nil || len(empty.Mentions) != 0 {
		t.Errorf("unwritten article: %+v", empty)
	}
	if _, err := articles.Get(ctx, sqlDB, articles.PersonSubject("NOBODY")); !errors.Is(err, articles.ErrSubjectNotFound) {
		t.Errorf("Get of an unknown person: %v", err)
	}

	first, created, err := articles.Save(ctx, sqlDB, articles.SaveInput{
		Subject: subject, AuthorID: w.alice, EditSummary: "  first draft  ",
		Body: doc("Seen in ", "@CASE:"+w.caseID, ", ", fmt.Sprintf("@OCCURRENCE:%d", w.occurrence), ", ", "@INQUIRY:"+w.inquiry,
			", with ", "@PERSON:"+w.personB, ", in ", fmt.Sprintf("@CLUSTER:%d", w.keptCluster),
			", per ", fmt.Sprintf("@REQUEST:%d", w.request), " and ", fmt.Sprintf("@REPORT:%d", w.report), "."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.Number != 1 || first.ParentID != 0 || first.EditSummary != "first draft" || first.Author.Username != "alice" {
		t.Errorf("first revision: created=%v %+v", created, first)
	}
	if first.Chars == 0 || len(first.SHA256) != 64 || !strings.Contains(first.Text, "@"+w.caseID) {
		t.Errorf("first revision body: %+v", first)
	}

	got, err := articles.Get(ctx, sqlDB, subject)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision == nil || got.Revision.ID != first.ID {
		t.Fatalf("current revision: %+v", got.Revision)
	}
	labels := map[articles.Kind]string{}
	for _, m := range got.Mentions {
		if !m.Found || m.Target == nil {
			t.Errorf("mention %+v not found", m.MentionKey)
			continue
		}
		labels[m.Kind] = m.Target.Label
	}
	for kind, want := range map[articles.Kind]string{
		articles.KindCase:       w.caseID,
		articles.KindOccurrence: "DP-01 123/2026",
		articles.KindInquiry:    w.inquiry,
		articles.KindPerson:     w.nameB,
		articles.KindCluster:    fmt.Sprintf("#%d", w.keptCluster),
		articles.KindRequest:    "Ofício 77/2026",
		articles.KindReport:     "9/2026",
	} {
		if labels[kind] != want {
			t.Errorf("%s label = %q, want %q", kind, labels[kind], want)
		}
	}
	for _, m := range got.Mentions {
		switch m.Kind {
		case articles.KindInquiry:
			if m.Target.Count != 2 {
				t.Errorf("inquiry occurrences = %d, want 2", m.Target.Count)
			}
		case articles.KindRequest, articles.KindReport:
			if len(m.Target.CaseIDs) != 1 || m.Target.CaseIDs[0] != w.caseID {
				t.Errorf("%s cases = %v", m.Kind, m.Target.CaseIDs)
			}
		}
	}

	// The same body again is no edit.
	same, created, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.bruno, BaseRevisionID: first.ID,
		Body: doc("Seen in ", "@CASE:"+w.caseID, ", ", fmt.Sprintf("@OCCURRENCE:%d", w.occurrence), ", ", "@INQUIRY:"+w.inquiry,
			", with ", "@PERSON:"+w.personB, ", in ", fmt.Sprintf("@CLUSTER:%d", w.keptCluster),
			", per ", fmt.Sprintf("@REQUEST:%d", w.request), " and ", fmt.Sprintf("@REPORT:%d", w.report), ".")})
	if err != nil {
		t.Fatal(err)
	}
	if created || same.ID != first.ID {
		t.Errorf("unchanged save wrote a revision: created=%v %+v", created, same)
	}

	// A second analyst edits it; history, revisions and the first text stay.
	second, created, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.bruno, BaseRevisionID: first.ID,
		Body: doc("Acts with ", "@PERSON:"+w.personB, ".")})
	if err != nil || !created {
		t.Fatalf("second save: %v, created=%v", err, created)
	}
	if second.Number != 2 || second.ParentID != first.ID || second.Author.Username != "bruno" {
		t.Errorf("second revision: %+v", second)
	}
	history, err := articles.History(ctx, sqlDB, subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Number != 2 || history[1].Number != 1 {
		t.Fatalf("history: %+v", history)
	}
	if history[0].SizeDelta != second.Chars-first.Chars || history[1].SizeDelta != first.Chars || history[0].Body != nil {
		t.Errorf("history sizes/bodies: %+v", history)
	}
	old, err := articles.GetRevision(ctx, sqlDB, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Text != first.Text || len(old.Body) == 0 {
		t.Errorf("revision 1 read back: %+v", old)
	}
	if _, err := articles.GetRevision(ctx, sqlDB, subject, 3); !errors.Is(err, articles.ErrRevisionNotFound) {
		t.Errorf("revision 3: %v", err)
	}

	// Only the current revision's mentions back-link.
	links, err := articles.Backlinks(ctx, sqlDB, []articles.MentionKey{{Kind: articles.KindCase, Ref: w.caseID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Errorf("case still back-linked after the edit removed it: %+v", links)
	}
	links, err = articles.Backlinks(ctx, sqlDB, []articles.MentionKey{{Kind: articles.KindPerson, Ref: w.personB}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Subject != subject || links[0].SubjectLabel != w.nameA || links[0].Mentions[0].Count != 1 {
		t.Errorf("backlinks to person B: %+v", links)
	}
	// An article does not back-link to itself.
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.alice, BaseRevisionID: second.ID,
		Body: doc("@PERSON:" + w.personA)}); err != nil {
		t.Fatal(err)
	}
	links, err = articles.Backlinks(ctx, sqlDB, []articles.MentionKey{{Kind: articles.KindPerson, Ref: w.personA}}, &subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Errorf("self backlink: %+v", links)
	}
}

func TestSaveConflictAndMentionChecks(t *testing.T) {
	w := newWorld(t)
	ctx, sqlDB := w.ctx, w.db
	subject := articles.PersonSubject(w.personA)

	first, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.alice, Body: doc("v1")})
	if err != nil {
		t.Fatal(err)
	}
	// Both start from revision 1; the second to save conflicts.
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.alice, BaseRevisionID: first.ID, Body: doc("alice")}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.bruno, BaseRevisionID: first.ID, Body: doc("bruno")}); !errors.Is(err, articles.ErrConflict) {
		t.Errorf("stale save: %v, want ErrConflict", err)
	}
	// A first save for an article that exists conflicts too.
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.bruno, Body: doc("new")}); !errors.Is(err, articles.ErrConflict) {
		t.Errorf("first save over an existing article: %v, want ErrConflict", err)
	}
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: articles.PersonSubject("NOBODY"), AuthorID: w.alice, Body: doc("x")}); !errors.Is(err, articles.ErrSubjectNotFound) {
		t.Errorf("save for an unknown person: %v", err)
	}
	var bodyErr *articles.BodyError
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: articles.PersonSubject(w.personB), AuthorID: w.alice,
		Body: json.RawMessage(`{"type":"doc","content":[{"type":"image"}]}`)}); !errors.As(err, &bodyErr) {
		t.Errorf("invalid body: %v", err)
	}

	// Unknown targets are refused, a draft laudo among them; a merged cluster is accepted.
	current, err := articles.Get(ctx, sqlDB, subject)
	if err != nil {
		t.Fatal(err)
	}
	bc := w.exec(`INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ('2026.0050', 'CRIMINAL', 'FACIAL', 'x')`)
	analysis := w.exec(`INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type) VALUES ($1, 1, 'FACIAL_REVIEW')`, bc)
	draft := w.exec(`INSERT INTO analysis_reports (case_analysis_id, report_type) VALUES ($1, 'FACIAL_REVIEW_REPORT')`, analysis)
	_, _, err = articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.alice, BaseRevisionID: current.Revision.ID,
		Body: doc("@CASE:1999.0001", "@PERSON:NOBODY", fmt.Sprintf("@REPORT:%d", draft), "@INQUIRY:0000", fmt.Sprintf("@CLUSTER:%d", w.merged))})
	var mentionErr *articles.MentionError
	if !errors.As(err, &mentionErr) || len(mentionErr.Missing) != 4 {
		t.Fatalf("unknown mentions: %v", err)
	}

	// A target that disappears after it was mentioned does not block later edits, and reads as
	// removed; the merged cluster reads as merged.
	rev, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.alice, BaseRevisionID: current.Revision.ID,
		Body: doc("@CASE:2026.0050", fmt.Sprintf("@CLUSTER:%d", w.merged))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `DELETE FROM biometric_cases WHERE case_id = '2026.0050'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.bruno, BaseRevisionID: rev.ID,
		Body: doc("still ", "@CASE:2026.0050", fmt.Sprintf("@CLUSTER:%d", w.merged))}); err != nil {
		t.Fatalf("edit keeping a removed target: %v", err)
	}
	got, err := articles.Get(ctx, sqlDB, subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got.Mentions {
		switch m.Kind {
		case articles.KindCase:
			if m.Found || m.Target != nil {
				t.Errorf("removed case reads as found: %+v", m)
			}
		case articles.KindCluster:
			if m.Found || m.MergedInto != fmt.Sprint(w.keptCluster) || m.Target == nil || m.Target.Ref != fmt.Sprint(w.keptCluster) {
				t.Errorf("merged cluster: %+v", m)
			}
		}
	}
}

func TestRevisionsAreAppendOnly(t *testing.T) {
	w := newWorld(t)
	ctx, sqlDB := w.ctx, w.db
	rev, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: articles.PersonSubject(w.personA), AuthorID: w.alice, Body: doc("text")})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`UPDATE article_revisions SET edit_summary = 'changed' WHERE id = $1`,
		`DELETE FROM article_revisions WHERE id = $1`,
		`UPDATE article_revision_bodies SET body_text = 'changed' WHERE revision_id = $1`,
		`DELETE FROM article_revision_bodies WHERE revision_id = $1`,
	} {
		if _, err := sqlDB.ExecContext(ctx, stmt, rev.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: %v, want refused as append-only", stmt, err)
		}
	}
	for _, table := range []string{"article_revisions", "article_revision_bodies"} {
		if _, err := sqlDB.ExecContext(ctx, `TRUNCATE `+table+` CASCADE`); err == nil {
			t.Errorf("TRUNCATE %s allowed", table)
		}
	}
}

func TestSearch(t *testing.T) {
	w := newWorld(t)
	ctx := w.ctx
	q := db.New(w.db)

	find := func(term string, kinds ...articles.Kind) map[articles.Kind][]string {
		t.Helper()
		targets, err := articles.Search(ctx, q, term, kinds, 0)
		if err != nil {
			t.Fatal(err)
		}
		out := map[articles.Kind][]string{}
		for _, target := range targets {
			out[target.Kind] = append(out[target.Kind], target.Label)
		}
		return out
	}

	all := find("")
	if len(all[articles.KindCase]) != 1 || len(all[articles.KindOccurrence]) != 2 || len(all[articles.KindInquiry]) != 1 ||
		len(all[articles.KindRequest]) != 1 || len(all[articles.KindReport]) != 1 || len(all[articles.KindCluster]) != 1 {
		t.Errorf("empty search: %v", all)
	}
	if len(all[articles.KindPerson]) != 0 {
		t.Errorf("empty search lists persons: %v", all[articles.KindPerson])
	}

	if got := find("souza", articles.KindPerson); len(got[articles.KindPerson]) != 1 || got[articles.KindPerson][0] != w.nameB {
		t.Errorf("person by name: %v", got)
	}
	if got := find(w.personA, articles.KindPerson); len(got[articles.KindPerson]) != 1 || got[articles.KindPerson][0] != w.nameA {
		t.Errorf("person by id: %v", got)
	}
	if got := find("124/", articles.KindOccurrence); len(got[articles.KindOccurrence]) != 1 || got[articles.KindOccurrence][0] != "DP-01 124/2026" {
		t.Errorf("occurrence by number: %v", got)
	}
	if got := find("robbery", articles.KindOccurrence, articles.KindCase); len(got[articles.KindOccurrence]) != 2 || len(got[articles.KindCase]) != 1 {
		t.Errorf("by description: %v", got)
	}
	if got := find("4512"); len(got[articles.KindInquiry]) != 1 {
		t.Errorf("inquiry: %v", got)
	}
	if got := find("ofício 77"); len(got[articles.KindRequest]) != 1 {
		t.Errorf("request: %v", got)
	}
	if got := find("9/2026", articles.KindReport); len(got[articles.KindReport]) != 1 {
		t.Errorf("report: %v", got)
	}
	if got := find("abc", articles.KindCluster); len(got[articles.KindCluster]) != 0 {
		t.Errorf("cluster by non-digits: %v", got)
	}
	if got := find(fmt.Sprint(w.keptCluster), articles.KindCluster); len(got[articles.KindCluster]) != 1 {
		t.Errorf("cluster by id: %v", got)
	}
}

// linkToCase puts a person's enrolled face and a face trace of the case in one REVIEWED cluster,
// as a confirmed identification would.
func (w *world) linkToCase(t *testing.T, personID, caseID string) int64 {
	t.Helper()
	var register int64
	if err := w.db.QueryRowContext(w.ctx, `SELECT r.id FROM identity_registers r JOIN identity_documents d ON d.id = r.document_id
		JOIN persons p ON p.id = d.person_id WHERE p.person_id = $1`, personID).Scan(&register); err != nil {
		t.Fatal(err)
	}
	file := w.exec(`INSERT INTO identity_files (register_id, file_type, sequence, source_path, storage_ref, content_type)
		VALUES ($1, 'photo', (SELECT COUNT(*) + 1 FROM identity_files WHERE register_id = $1), 'x', 'x', 'image/jpeg')`, register)
	w.exec(`INSERT INTO biometric_features (feature_type, provenance, identity_file_id) VALUES ('FACE_RECORD', 'KNOWN', $1)`, file)
	var bc int64
	if err := w.db.QueryRowContext(w.ctx, `SELECT id FROM biometric_cases WHERE case_id = $1`, caseID).Scan(&bc); err != nil {
		t.Fatal(err)
	}
	evidence := w.exec(`INSERT INTO case_evidences (biometric_case_id, sequence) VALUES ($1, (SELECT COUNT(*) + 1 FROM case_evidences WHERE biometric_case_id = $1))`, bc)
	trace := w.exec(`INSERT INTO case_traces (evidence_id, sequence, trace_type) VALUES ($1, 1, 'FACE_CAPTURE')`, evidence)
	w.exec(`INSERT INTO biometric_features (feature_type, provenance, case_trace_id) VALUES ('FACE_CAPTURE', 'QUESTIONED', $1)`, trace)
	cluster := w.exec(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'REVIEWED')`)
	automatic := w.exec(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'AUTOMATIC')`)
	for _, feature := range []string{fmt.Sprint(file), fmt.Sprintf("TRACE:%d#feature", trace)} {
		for id, basis := range map[int64]string{cluster: "REVIEWED", automatic: "AUTOMATIC"} {
			if _, err := w.db.ExecContext(w.ctx, `INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, $2, $3)`, id, basis, feature); err != nil {
				t.Fatal(err)
			}
		}
	}
	return cluster
}

// faceTrace adds a case with one face trace and returns the trace's feature id.
func (w *world) faceTrace(t *testing.T, caseID, description string) string {
	t.Helper()
	bc := w.exec(`INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ($1, 'CRIMINAL', 'FACIAL', $2)`, caseID, description)
	evidence := w.exec(`INSERT INTO case_evidences (biometric_case_id, sequence) VALUES ($1, 1)`, bc)
	trace := w.exec(`INSERT INTO case_traces (evidence_id, sequence, trace_type) VALUES ($1, 1, 'FACE_CAPTURE')`, evidence)
	w.exec(`INSERT INTO biometric_features (feature_type, provenance, case_trace_id) VALUES ('FACE_CAPTURE', 'QUESTIONED', $1)`, trace)
	return fmt.Sprintf("TRACE:%d#feature", trace)
}

func TestPersonTemplate(t *testing.T) {
	w := newWorld(t)
	ctx, sqlDB := w.ctx, w.db
	cluster := w.linkToCase(t, w.personA, w.caseID)
	w.linkToCase(t, w.personB, w.caseID)
	subject := articles.PersonSubject(w.personA)

	// Links no examiner reviewed: a matcher candidate pending review, with its score, and a case
	// the AUTOMATIC layer reaches through A's cluster alone.
	pending := w.faceTrace(t, "2026.0060", "pending robbery")
	known := ""
	if err := sqlDB.QueryRowContext(ctx, `SELECT f.id::text FROM identity_files f JOIN identity_registers r ON r.id = f.register_id
		JOIN identity_documents d ON d.id = r.document_id JOIN persons p ON p.id = d.person_id WHERE p.person_id = $1`, w.personA).Scan(&known); err != nil {
		t.Fatal(err)
	}
	a, b := known, pending
	if b < a {
		a, b = b, a
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, system_source, confidence, threshold)
		VALUES ($1, $2, 'FACIAL', 'SYSTEM', 'POSITIVE', 'test-matcher', 0.83, 0.7)`, a, b); err != nil {
		t.Fatal(err)
	}
	indirect := w.faceTrace(t, "2026.0061", "indirect theft")
	var automaticOfA int64
	if err := sqlDB.QueryRowContext(ctx, `SELECT cluster_id FROM cluster_members WHERE basis = 'AUTOMATIC' AND feature_id = $1`, known).Scan(&automaticOfA); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, 'AUTOMATIC', $2)`, automaticOfA, indirect); err != nil {
		t.Fatal(err)
	}

	tpl, err := articles.GenerateTemplate(ctx, sqlDB, subject, "pt-BR", w.alice)
	if err != nil {
		t.Fatal(err)
	}
	if tpl.ID == 0 || tpl.Language != "pt-BR" || tpl.Generator != articles.TemplateGenerator {
		t.Errorf("template: %+v", tpl)
	}
	body, err := articles.ParseBody(tpl.Body)
	if err != nil {
		t.Fatalf("generated draft refused by the schema: %v", err)
	}
	for _, want := range []string{w.nameA, "Identificação", "Casos e ocorrências", "@" + w.caseID, "@DP-01 123/2026",
		"inquérito @" + w.inquiry, "Requisição: @Ofício 77/2026", "Laudo: @9/2026", "Pessoas associadas",
		"@" + w.nameB + " — também ligada a @" + w.caseID + " (confirmado)", fmt.Sprintf("@#%d — cluster revisado, facial, 2 membros.", cluster),
		"Peritos confirmaram a ligação desta pessoa a 1 caso(s); outros 2 dependem",
		"Grau de certeza:", "Confirmados por peritos", "Não revisados (ligações do comparador)",
		"@2026.0060 — pending robbery. Candidato do comparador, escore 0,83, aguardando revisão.",
		fmt.Sprintf("@2026.0061 — indirect theft. Ligado pelo cluster automático @#%d.", automaticOfA),
		fmt.Sprintf("@#%d — cluster automático (ligações do comparador, não revisadas), facial, 3 membros.", automaticOfA),
		"Análise"} {
		if !strings.Contains(body.Text, want) {
			t.Errorf("draft lacks %q:\n%s", want, body.Text)
		}
	}
	mentioned := map[articles.Kind]bool{}
	for _, m := range body.Mentions {
		mentioned[m.Kind] = true
	}
	for _, kind := range []articles.Kind{articles.KindCase, articles.KindOccurrence, articles.KindInquiry, articles.KindPerson,
		articles.KindCluster, articles.KindRequest, articles.KindReport} {
		if !mentioned[kind] {
			t.Errorf("draft mentions no %s", kind)
		}
	}

	// The infobox's counts read the same links.
	counts, err := articles.PersonLinkCounts(ctx, sqlDB, w.personA)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Cases["confirmed"] != 1 || counts.Cases["unreviewed"] != 2 || counts.Cases["uncertain"] != 0 || counts.Cases["excluded"] != 0 ||
		counts.Clusters.Reviewed != 1 || counts.Clusters.Automatic != 1 ||
		counts.Associates.Confirmed != 1 || counts.Associates.Unreviewed != 0 ||
		counts.OtherIdentities.Confirmed != 0 || counts.OtherIdentities.Unreviewed != 0 {
		t.Errorf("link counts: %+v", counts)
	}
	if _, err := articles.PersonLinkCounts(ctx, sqlDB, "NOBODY"); !errors.Is(err, articles.ErrSubjectNotFound) {
		t.Errorf("counts for an unknown person: %v", err)
	}

	english, err := articles.GenerateTemplate(ctx, sqlDB, subject, "en", w.alice)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(english.Body), "Cases and occurrences") {
		t.Errorf("English draft: %s", english.Body)
	}

	// Every draft generated is logged, saved or not, and the log is append-only.
	var logged int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM article_templates WHERE subject_ref = $1 AND generated_by = $2`,
		w.personA, w.alice).Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 2 {
		t.Errorf("logged drafts = %d, want 2", logged)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE article_templates SET language = 'x' WHERE id = $1`, tpl.ID); err == nil {
		t.Error("article_templates updated")
	}

	// A save names the draft it started from; another person's draft is refused.
	if _, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: articles.PersonSubject(w.personB), AuthorID: w.alice,
		Body: tpl.Body, TemplateID: tpl.ID}); !errors.Is(err, articles.ErrTemplateNotFound) {
		t.Errorf("save with another person's draft: %v", err)
	}
	rev, _, err := articles.Save(ctx, sqlDB, articles.SaveInput{Subject: subject, AuthorID: w.bruno, Body: tpl.Body, TemplateID: tpl.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rev.TemplateID != tpl.ID {
		t.Errorf("revision template = %d, want %d", rev.TemplateID, tpl.ID)
	}
	history, err := articles.History(ctx, sqlDB, subject)
	if err != nil || len(history) != 1 || history[0].TemplateID != tpl.ID {
		t.Errorf("history: %+v %v", history, err)
	}

	// A person with no linked data still gets a valid draft, and an unknown one none.
	if _, err := articles.GenerateTemplate(ctx, sqlDB, articles.PersonSubject("NOBODY"), "en", w.alice); !errors.Is(err, articles.ErrSubjectNotFound) {
		t.Errorf("draft for an unknown person: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO persons (person_id) VALUES ('P-EMPTY')`); err != nil {
		t.Fatal(err)
	}
	empty, err := articles.GenerateTemplate(ctx, sqlDB, articles.PersonSubject("P-EMPTY"), "en", w.alice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := articles.ParseBody(empty.Body); err != nil || !strings.Contains(string(empty.Body), "No case linked yet") {
		t.Errorf("empty person's draft: %v %s", err, empty.Body)
	}
}
