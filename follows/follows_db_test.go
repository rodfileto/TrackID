package follows_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/follows"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/notifications"
)

// testDB is a throwaway, migrated database on the server named by TRACKID_TEST_DATABASE_URL, skipped when unset.
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

type fixture struct {
	t   *testing.T
	db  *sql.DB
	ctx context.Context
}

func (f fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.ExecContext(f.ctx, query, args...); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
}

func (f fixture) id(query string, args ...any) int64 {
	f.t.Helper()
	var id int64
	if err := f.db.QueryRowContext(f.ctx, query, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
	return id
}

func (f fixture) user(name string, analyst bool) int64 {
	f.t.Helper()
	u, err := auth.NewRepository(f.db).Create(f.ctx, auth.NewUser{FirstName: name, LastName: "Test", StaffNumber: name,
		JobTitle: "test", Username: name, Password: "password-1",
		Access: auth.Access{AccountRole: auth.AccountRoleUser, IsActive: true, IntelligenceAnalyst: analyst}}, "")
	if err != nil {
		f.t.Fatal(err)
	}
	return u.ID
}

// person enrolls a person with one document and returns their row id.
func (f fixture) person(personID string) int64 {
	f.t.Helper()
	return f.id(`INSERT INTO persons (person_id) VALUES ($1) RETURNING id`, personID)
}

// photo adds a document, register and photo to a person and returns the photo's KNOWN feature id.
func (f fixture) photo(personRow int64, number string) string {
	f.t.Helper()
	doc := f.id(`INSERT INTO identity_documents (person_id, document_number, document_type) VALUES ($1, $2, 'RG') RETURNING id`, personRow, number)
	reg := f.id(`INSERT INTO identity_registers (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender)
		VALUES ($1, $2, 'Fulano de Tal', 'Mãe', 'F', 'Pai', 'M') RETURNING id`, doc, number)
	file := f.id(`INSERT INTO identity_files (register_id, file_type, source_path, storage_ref) VALUES ($1, 'photo', 'x', 'x') RETURNING id`, reg)
	return graph.KnownFeatureID(file)
}

// trace adds a QUESTIONED face trace to a case, creating the case if needed, and returns its feature id.
func (f fixture) trace(caseID string) string {
	f.t.Helper()
	f.exec(`INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ($1, 'CRIMINAL', 'FACIAL', 'test')
		ON CONFLICT (case_id) DO NOTHING`, caseID)
	evidence := f.id(`INSERT INTO case_evidences (biometric_case_id, sequence)
		SELECT id, (SELECT COUNT(*) + 1 FROM case_evidences e WHERE e.biometric_case_id = bc.id) FROM biometric_cases bc WHERE case_id = $1 RETURNING id`, caseID)
	trace := f.id(`INSERT INTO case_traces (evidence_id, sequence, trace_type) VALUES ($1, 1, 'FACE_CAPTURE') RETURNING id`, evidence)
	return graph.QuestionedFeatureID(trace)
}

func (f fixture) cluster(members ...string) int64 {
	f.t.Helper()
	id := f.id(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'AUTOMATIC') RETURNING id`)
	f.add(id, members...)
	return id
}

func (f fixture) add(cluster int64, members ...string) {
	f.t.Helper()
	for _, m := range members {
		f.exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, 'AUTOMATIC', $2)`, cluster, m)
	}
}

// merge moves every member of from into to and retires from, as cluster.apply does, in one transaction.
func (f fixture) merge(from, to int64) {
	f.t.Helper()
	tx, err := f.db.BeginTx(f.ctx, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(f.ctx, `DELETE FROM cluster_members WHERE cluster_id = $1 RETURNING feature_id`, from)
	if err != nil {
		f.t.Fatal(err)
	}
	var moved []string
	for rows.Next() {
		var m string
		rows.Scan(&m)
		moved = append(moved, m)
	}
	rows.Close()
	for _, m := range moved {
		if _, err := tx.ExecContext(f.ctx, `INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, 'AUTOMATIC', $2)`, to, m); err != nil {
			f.t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(f.ctx, `DELETE FROM clusters WHERE id = $1`, from); err != nil {
		f.t.Fatal(err)
	}
	if _, err := tx.ExecContext(f.ctx, `INSERT INTO cluster_merges (from_cluster_id, to_cluster_id, reason, basis) VALUES ($1, $2, 'decision_merge', 'AUTOMATIC')`, from, to); err != nil {
		f.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		f.t.Fatal(err)
	}
}

func (f fixture) follow(user int64, kind, id string) {
	f.t.Helper()
	s, err := follows.Resolve(f.ctx, f.db, kind, id)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := follows.Add(f.ctx, f.db, user, s); err != nil {
		f.t.Fatal(err)
	}
}

func (f fixture) fanout() {
	f.t.Helper()
	if _, err := follows.Fanout(f.ctx, f.db); err != nil {
		f.t.Fatal(err)
	}
}

// changes returns, for each of the user's notifications, its subject and the changes it reports.
func (f fixture) changes(user int64) map[string]follows.Summary {
	f.t.Helper()
	items, _, err := notifications.List(f.ctx, f.db, user)
	if err != nil {
		f.t.Fatal(err)
	}
	summaries, err := follows.Summarize(f.ctx, f.db, items)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]follows.Summary{}
	for _, item := range items {
		if s, ok := summaries[item.ID]; ok {
			out[s.SubjectKind+":"+s.SubjectID] = s
		}
	}
	return out
}

func count(s follows.Summary, change string) (int, []string) {
	for _, c := range s.Changes {
		if c.Change == change {
			return c.Count, c.Refs
		}
	}
	return 0, nil
}

func setup(t *testing.T) fixture {
	return fixture{t: t, db: testDB(t), ctx: context.Background()}
}

func TestPersonFollowerIsToldOfDocumentsCasesAndClusters(t *testing.T) {
	f := setup(t)
	analyst := f.user("analyst", true)
	p := f.person("P1")
	photo := f.photo(p, "100")
	a := f.cluster(photo)
	f.fanout() // what happened before the follow is not reported
	f.follow(analyst, follows.KindPerson, "P1")

	f.photo(p, "101")
	f.add(a, f.trace("K1"))
	f.fanout()
	s := f.changes(analyst)["PERSON:P1"]
	if n, _ := count(s, follows.ChangeDocumentAdded); n != 1 {
		t.Errorf("documents added = %d, want 1: %+v", n, s)
	}
	if n, refs := count(s, follows.ChangeCaseLinked); n != 1 || !slices.Equal(refs, []string{"K1"}) {
		t.Errorf("cases linked = %d %v, want K1: %+v", n, refs, s)
	}
	if s.Name != "Fulano de Tal" || s.PersonNumber != p {
		t.Errorf("label = %q #%d", s.Name, s.PersonNumber)
	}

	// A second trace of the same case links nothing new, and extends the unread notification.
	f.add(a, f.trace("K1"))
	f.fanout()
	s = f.changes(analyst)["PERSON:P1"]
	if n, _ := count(s, follows.ChangeCaseLinked); n != 1 {
		t.Errorf("cases linked after a second trace = %d, want 1", n)
	}
	items, unread, err := notifications.List(f.ctx, f.db, analyst)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || unread != 1 {
		t.Errorf("notifications = %d (%d unread), want one grouped", len(items), unread)
	}

	// Read, the next change starts a new notification.
	if err := notifications.MarkAllRead(f.ctx, f.db, analyst); err != nil {
		t.Fatal(err)
	}
	b := f.cluster(f.trace("K2"))
	f.add(b, f.photo(p, "102"))
	f.fanout()
	items, unread, _ = notifications.List(f.ctx, f.db, analyst)
	if len(items) != 2 || unread != 1 {
		t.Fatalf("notifications = %d (%d unread), want a new one", len(items), unread)
	}
	summaries, _ := follows.Summarize(f.ctx, f.db, items[:1]) // the unread one, listed first
	s = summaries[items[0].ID]
	if n, refs := count(s, follows.ChangeClusterJoined); n != 1 || refs[0] != strconv.FormatInt(b, 10) {
		t.Errorf("clusters joined = %d %v, want %d", n, refs, b)
	}
	if n, refs := count(s, follows.ChangeCaseLinked); n != 1 || refs[0] != "K2" {
		t.Errorf("cases linked = %d %v, want K2", n, refs)
	}
}

func TestMergeReportsOnlyWhatIsNew(t *testing.T) {
	f := setup(t)
	analyst := f.user("analyst", true)
	p := f.person("P1")
	a := f.cluster(f.photo(p, "100"), f.trace("K1"))
	b := f.cluster(f.trace("K1"), f.trace("K2"))
	f.fanout()
	f.follow(analyst, follows.KindPerson, "P1")
	f.follow(analyst, follows.KindCluster, strconv.FormatInt(b, 10))

	// The person's cluster survives: K2 is new, K1 is not, and no cluster was joined.
	f.merge(b, a)
	f.fanout()
	got := f.changes(analyst)
	person := got["PERSON:P1"]
	if n, refs := count(person, follows.ChangeCaseLinked); n != 1 || refs[0] != "K2" {
		t.Errorf("cases linked = %d %v, want only K2", n, refs)
	}
	if n, _ := count(person, follows.ChangeClusterJoined); n != 0 {
		t.Errorf("clusters joined = %d, want 0", n)
	}
	merged := got["CLUSTER:"+strconv.FormatInt(b, 10)]
	if merged.MergedInto != a {
		t.Errorf("merged into %d, want %d: %+v", merged.MergedInto, a, merged)
	}
	if n, _ := count(merged, follows.ChangeMemberLeft); n != 0 {
		t.Errorf("a merged cluster reports its members leaving: %+v", merged)
	}
	following, err := follows.IsFollowing(f.ctx, f.db, analyst, follows.Subject{Kind: follows.KindCluster, ID: strconv.FormatInt(a, 10)})
	if err != nil || !following {
		t.Errorf("the follow did not move to the surviving cluster: %v", err)
	}
}

func TestPersonsClusterRetiredByMergeLinksNothingOld(t *testing.T) {
	f := setup(t)
	analyst := f.user("analyst", true)
	p := f.person("P1")
	a := f.cluster(f.trace("K9"))
	b := f.cluster(f.photo(p, "100"), f.trace("K1"))
	f.fanout()
	f.follow(analyst, follows.KindPerson, "P1")

	f.merge(b, a) // the person's own cluster is the one retired
	f.fanout()
	s := f.changes(analyst)["PERSON:P1"]
	if n, refs := count(s, follows.ChangeCaseLinked); n != 1 || refs[0] != "K9" {
		t.Errorf("cases linked = %d %v, want only K9", n, refs)
	}
	if n, _ := count(s, follows.ChangeClusterJoined); n != 0 {
		t.Errorf("a merge reported as a cluster joined: %+v", s)
	}
}

func TestOnlyActiveAnalystsAreTold(t *testing.T) {
	f := setup(t)
	analyst := f.user("analyst", true)
	examiner := f.user("examiner", false)
	p := f.person("P1")
	f.follow(analyst, follows.KindPerson, "P1")
	f.follow(examiner, follows.KindPerson, "P1")
	f.photo(p, "100")
	f.fanout()
	if len(f.changes(analyst)) != 1 {
		t.Error("the analyst was not told")
	}
	if _, unread, _ := notifications.List(f.ctx, f.db, examiner); unread != 0 {
		t.Error("a user without intelligence access was told")
	}
	var pending int
	f.db.QueryRow(`SELECT COUNT(*) FROM subject_changes WHERE fanned_out_at IS NULL`).Scan(&pending)
	if pending != 0 {
		t.Errorf("%d changes left pending", pending)
	}
}

func TestReviewedClusterFollowsItsAutomaticCluster(t *testing.T) {
	f := setup(t)
	photo := f.photo(f.person("P1"), "100")
	a := f.cluster(photo)
	r := f.id(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'REVIEWED') RETURNING id`)
	f.exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, 'REVIEWED', $2)`, r, photo)
	s, err := follows.Resolve(f.ctx, f.db, follows.KindCluster, strconv.FormatInt(r, 10))
	if err != nil || s.ID != strconv.FormatInt(a, 10) {
		t.Errorf("resolved %+v, %v; want cluster %d", s, err, a)
	}
	if _, err := follows.Resolve(f.ctx, f.db, follows.KindPerson, "nobody"); err != follows.ErrNotFound {
		t.Errorf("unknown person: %v", err)
	}
}

func TestRecordFeatureReachesClusterAndPersons(t *testing.T) {
	f := setup(t)
	analyst := f.user("analyst", true)
	trace := f.trace("K1")
	a := f.cluster(f.photo(f.person("P1"), "100"), trace)
	f.fanout()
	f.follow(analyst, follows.KindPerson, "P1")
	f.follow(analyst, follows.KindCluster, strconv.FormatInt(a, 10))

	tx, err := f.db.BeginTx(f.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := follows.RecordFeature(f.ctx, db.New(tx), trace, "CAMERA_HITS", "search:1", 4); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	f.fanout()
	got := f.changes(analyst)
	for _, key := range []string{"PERSON:P1", "CLUSTER:" + strconv.FormatInt(a, 10)} {
		if n, _ := count(got[key], "CAMERA_HITS"); n != 4 {
			t.Errorf("%s: camera hits = %d, want 4: %+v", key, n, got[key])
		}
	}
}
