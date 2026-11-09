package collections_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/rodfileto/trackid/collections"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/occurrences"
	"github.com/rodfileto/trackid/requests"
)

// testDB mirrors requests' helper: a throwaway, migrated database on the server named by
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

type fixture struct {
	db          *sql.DB
	collector   auth.UserProfile
	coordinator auth.UserProfile
	location    int64
	now         time.Time
}

func setup(t *testing.T) fixture {
	t.Helper()
	sqlDB := testDB(t)
	ctx := context.Background()
	repo := auth.NewRepository(sqlDB)
	user := func(name string, access auth.Access) auth.UserProfile {
		t.Helper()
		access.IsActive, access.AccountRole = true, auth.AccountRoleUser
		u, err := repo.Create(ctx, auth.NewUser{FirstName: name, LastName: "Test", StaffNumber: name, JobTitle: "test",
			Username: name, Password: "password1", Access: access}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	f := fixture{db: sqlDB, now: time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC),
		collector:   user("papilo", auth.Access{ExaminesFingerprint: true}),
		coordinator: user("coord", auth.Access{Coordinator: true})}
	if err := sqlDB.QueryRow(`INSERT INTO locations (location_id, geom, precision, municipality_code, uf)
		VALUES ('L-1', ST_GeogFromText('POINT(-47.88 -15.79)'), 'EXACT', '5300108', 'DF') RETURNING id`).Scan(&f.location); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) scene(t *testing.T, in collections.RegisterInput) collections.Registered {
	t.Helper()
	if in.OccurrenceID == nil && in.Occurrence == nil {
		in.Occurrence = &occurrences.CreateInput{Types: []string{"HOME_ROBBERY"}, OccurredFrom: f.now.Add(-6 * time.Hour),
			Description: "Roubo a residência", Place: occurrences.PlaceInput{ExistingID: f.location}}
	}
	if in.CollectedBy == 0 {
		in.CollectedBy = f.collector.ID
	}
	if in.CollectedFrom.IsZero() {
		in.CollectedFrom = f.now.Add(-2 * time.Hour)
	}
	r, err := collections.Register(context.Background(), f.db, in, f.collector.ID, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f fixture) file(t *testing.T, caseID, hash string) int64 {
	t.Helper()
	var id int64
	if err := f.db.QueryRow(`INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename)
		SELECT id, 'evidence', 'image', $2, $2 || '.png' FROM biometric_cases WHERE case_id = $1 RETURNING id`, caseID, hash).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func isValidation(err error) bool {
	var v collections.ValidationError
	var r requests.ValidationError
	return errors.As(err, &v) || errors.As(err, &r)
}

// The examiner examines a scene on call and registers it: a fingerprint case of the occurrence's
// nature, linked to it, with the collection awaiting its request and the vestiges sealed at the
// scene by the collector. The collector works the case although it has no analysis yet. The request
// that formalizes the scene arrives later, joins the case, asks no task yet and names the vestiges
// its questions are about; the comparison that follows answers it.
func TestSceneThenRequest(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	to := f.now.Add(-time.Hour)
	r := f.scene(t, collections.RegisterInput{Description: "Local de roubo", CollectedTo: &to, ReportMode: "together",
		Vestiges: []collections.VestigeInput{{Description: "Fragmentos na janela", SealNumber: "A-0001"}, {Description: "Copo de vidro", SealNumber: " A-0002 "}}})

	var caseType, modality, mode string
	var occurrence sql.NullInt64
	if err := f.db.QueryRow(`SELECT case_type, modality, report_mode, occurrence_id FROM biometric_cases WHERE case_id = $1`, r.CaseID).
		Scan(&caseType, &modality, &mode, &occurrence); err != nil {
		t.Fatal(err)
	}
	if caseType != "CRIMINAL" || modality != "FINGERPRINT" || mode != "TOGETHER" || occurrence.Int64 != r.OccurrenceID {
		t.Errorf("scene case: %s %s %s occurrence %v, want CRIMINAL FINGERPRINT TOGETHER %d", caseType, modality, mode, occurrence, r.OccurrenceID)
	}
	if list, _ := analyses.List(ctx, f.db, r.CaseID); len(list) != 0 {
		t.Errorf("a scene case opens no analysis yet: %+v", list)
	}
	rights, err := distribution.Rights(ctx, f.db, f.collector, r.CaseID)
	if err != nil || !rights.Read || !rights.Write {
		t.Errorf("the collector's rights on the scene case: %+v, %v", rights, err)
	}

	origins, err := collections.List(ctx, f.db, r.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(origins.Collections) != 1 || origins.Collections[0].Request != nil || origins.Collections[0].CollectedBy != "papilo" {
		t.Fatalf("collections: %+v", origins.Collections)
	}
	if len(origins.Vestiges) != 2 || origins.Vestiges[1].Label != "V2" || origins.Vestiges[1].Seals[0].Number != "A-0002" ||
		origins.Vestiges[0].Seals[0].AppliedBy != "papilo" || !origins.Vestiges[0].Seals[0].AppliedAt.Equal(to) {
		t.Fatalf("vestiges: %+v", origins.Vestiges)
	}
	awaiting, err := collections.ListAwaiting(ctx, f.db)
	if err != nil || len(awaiting) != 1 || awaiting[0].CaseID != r.CaseID || awaiting[0].Occurrence == "" {
		t.Fatalf("awaiting: %+v, %v", awaiting, err)
	}

	// The request formalizing the scene: no task, the collection, the first vestige.
	reg, err := requests.Register(ctx, f.db, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF/SR/DF", DocumentReference: "Requisição 45/2026", ReceivedOn: "2026-11-08", OccurrenceID: &r.OccurrenceID},
		Cases: []requests.CaseInput{{CaseID: r.CaseID, CollectionIDs: []int64{r.CollectionID}, VestigeIDs: []int64{r.Vestiges[0].ID}}},
	}, f.coordinator.ID, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if awaiting, _ := collections.ListAwaiting(ctx, f.db); len(awaiting) != 0 {
		t.Errorf("still awaiting after the request: %+v", awaiting)
	}
	origins, _ = collections.List(ctx, f.db, r.CaseID)
	if c := origins.Collections[0]; c.Request == nil || c.Request.ID != reg.RequestID || c.Request.DocumentReference != "Requisição 45/2026" {
		t.Errorf("formalized collection: %+v", c.Request)
	}
	if v := origins.Vestiges; len(v[0].AskedBy) != 1 || v[0].AskedBy[0] != reg.RequestID || len(v[1].AskedBy) != 0 {
		t.Errorf("asked about: %+v / %+v", v[0].AskedBy, v[1].AskedBy)
	}
	if list, _ := analyses.List(ctx, f.db, r.CaseID); len(list) != 0 {
		t.Errorf("a request with no task opened %+v", list)
	}

	// A second request cannot formalize what is formalized already, nor a new case have collections.
	_, err = requests.Register(ctx, f.db, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF/SR/DF", DocumentReference: "Requisição 46/2026", ReceivedOn: "2026-11-08"},
		Cases: []requests.CaseInput{{CaseID: r.CaseID, CollectionIDs: []int64{r.CollectionID}}},
	}, f.coordinator.ID, f.now)
	if !isValidation(err) || !strings.Contains(err.Error(), "awaiting") {
		t.Errorf("formalized twice: %v", err)
	}
	_, err = requests.Register(ctx, f.db, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF/SR/DF", DocumentReference: "Requisição 47/2026", ReceivedOn: "2026-11-08", OccurrenceID: &r.OccurrenceID},
		Cases: []requests.CaseInput{{Task: "FINGERPRINT_COMPARISON", Description: "x", CollectionIDs: []int64{r.CollectionID}}},
	}, f.coordinator.ID, f.now)
	if !isValidation(err) {
		t.Errorf("a new case with collections: %v", err)
	}
	// A request that formalizes nothing still names its task.
	_, err = requests.Register(ctx, f.db, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF/SR/DF", DocumentReference: "Requisição 48/2026", ReceivedOn: "2026-11-08"},
		Cases: []requests.CaseInput{{CaseID: r.CaseID}},
	}, f.coordinator.ID, f.now)
	if !isValidation(err) || !strings.Contains(err.Error(), "task") {
		t.Errorf("no task, no collection: %v", err)
	}

	// The lifts gave something to compare: the comparison answers the request that formalized the scene.
	id, err := analyses.OpenWith(ctx, f.db, r.CaseID, analyses.FingerprintComparison, nil, f.collector.ID, analyses.Options{RequestID: reg.RequestID})
	if err != nil {
		t.Fatal(err)
	}
	var answers sql.NullInt64
	if err := f.db.QueryRow(`SELECT case_request_id FROM case_analyses WHERE id = $1`, id).Scan(&answers); err != nil || answers.Int64 != reg.RequestID {
		t.Errorf("the comparison answers %v (%v), want %d", answers, err, reg.RequestID)
	}
}

// A file has one origin. Material a collection gathered stays the collection's: a request cannot
// claim it, and a file sent by a request cannot be moved to a collection. A vestige must be of the
// file's origin. A frame taken from a collected video comes from that collection too.
func TestOneOrigin(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.scene(t, collections.RegisterInput{Vestiges: []collections.VestigeInput{{Description: "Cartão de levantamento", SealNumber: "B-1"}}})
	reg, err := requests.Register(ctx, f.db, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF", DocumentReference: "Ofício 9/2026", ReceivedOn: "2026-11-08", OccurrenceID: &r.OccurrenceID},
		Cases: []requests.CaseInput{{CaseID: r.CaseID, Task: "FINGERPRINT_COMPARISON", CollectionIDs: []int64{r.CollectionID}}},
	}, f.coordinator.ID, f.now)
	if err != nil {
		t.Fatal(err)
	}
	received, err := collections.AddVestige(ctx, f.db, r.CaseID, collections.AddVestigeInput{
		VestigeInput: collections.VestigeInput{Description: "Arma apreendida", SealNumber: "PF-77"}, RequestID: reg.RequestID}, f.coordinator.ID, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if received.Label != "V2" || received.Seals[0].AppliedBy != "" {
		t.Errorf("received vestige %+v: its seal was applied by the agency that sent it", received)
	}

	lift := f.file(t, r.CaseID, "lift1")
	if err := collections.Attach(ctx, f.db, r.CaseID, lift, collections.Origin{CollectionID: r.CollectionID, VestigeID: received.ID}); !isValidation(err) {
		t.Errorf("a collected file taken from a received vestige: %v", err)
	}
	if err := collections.Attach(ctx, f.db, r.CaseID, lift, collections.Origin{CollectionID: r.CollectionID, VestigeID: r.Vestiges[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := requests.AttachFiles(ctx, f.db, r.CaseID, reg.RequestID, []int64{lift}); !isValidation(err) {
		t.Errorf("a request claimed a collected file: %v", err)
	}
	if err := collections.Attach(ctx, f.db, r.CaseID, lift, collections.Origin{RequestID: reg.RequestID}); !isValidation(err) {
		t.Errorf("a collected file moved to a request: %v", err)
	}
	if err := collections.Attach(ctx, f.db, r.CaseID, lift, collections.Origin{RequestID: reg.RequestID, CollectionID: r.CollectionID}); !isValidation(err) {
		t.Errorf("two origins: %v", err)
	}
	if _, err := f.db.Exec(`UPDATE case_files SET case_request_id = $1 WHERE id = $2`, reg.RequestID, lift); err == nil {
		t.Error("the database let a file have two origins")
	}

	sent := f.file(t, r.CaseID, "gun")
	if err := collections.Attach(ctx, f.db, r.CaseID, sent, collections.Origin{RequestID: reg.RequestID, VestigeID: received.ID}); err != nil {
		t.Fatal(err)
	}
	if err := collections.Attach(ctx, f.db, r.CaseID, sent, collections.Origin{CollectionID: r.CollectionID}); !isValidation(err) {
		t.Errorf("a received file moved to a collection: %v", err)
	}

	detail, err := cases.Get(ctx, f.db, r.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range detail.Evidences {
		switch e.ID {
		case lift:
			if e.Request != nil || e.Collection == nil || e.Collection.Sequence != 1 || e.Vestige == nil || e.Vestige.Label != "V1" {
				t.Errorf("collected file: request %+v collection %+v vestige %+v", e.Request, e.Collection, e.Vestige)
			}
		case sent:
			if e.Request == nil || e.Request.ID != reg.RequestID || e.Collection != nil || e.Vestige == nil || e.Vestige.Label != "V2" {
				t.Errorf("received file: request %+v collection %+v vestige %+v", e.Request, e.Collection, e.Vestige)
			}
		}
	}

	// A frame of a collected video comes from the collection, as the video does.
	var video, frame int64
	if err := f.db.QueryRow(`INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename, collection_id, vestige_id)
		SELECT id, 'video', 'video', 'v1', 'dvr.mp4', $2, $3 FROM biometric_cases WHERE case_id = $1 RETURNING id`,
		r.CaseID, r.CollectionID, r.Vestiges[0].ID).Scan(&video); err != nil {
		t.Fatal(err)
	}
	frame = f.file(t, r.CaseID, "frame")
	// video.ExtractFrame needs storage to make the frame; its last step is this query.
	if err := db.New(f.db).InheritFileOrigin(ctx, db.InheritFileOriginParams{ID: frame, SourceID: video}); err != nil {
		t.Fatal(err)
	}
	var fromCollection, fromVestige sql.NullInt64
	if err := f.db.QueryRow(`SELECT collection_id, vestige_id FROM case_files WHERE id = $1`, frame).Scan(&fromCollection, &fromVestige); err != nil {
		t.Fatal(err)
	}
	if fromCollection.Int64 != r.CollectionID || fromVestige.Int64 != r.Vestiges[0].ID {
		t.Errorf("the frame comes from collection %v vestige %v, want the video's", fromCollection, fromVestige)
	}

	// A request or collection that material came from cannot be deleted from under it.
	if _, err := f.db.Exec(`DELETE FROM case_collections WHERE id = $1`, r.CollectionID); err == nil {
		t.Error("deleted the collection its material came from")
	}
}

// Seals: the lab breaks the seal to examine and seals again under a new number; a vestige has at
// most one unbroken seal, and breaking one it does not have is refused.
func TestSeals(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.scene(t, collections.RegisterInput{Vestiges: []collections.VestigeInput{{Description: "Envelope", SealNumber: "C-1"}, {Description: "Unsealed"}}})
	v := r.Vestiges[0].ID
	later := f.now.Add(24 * time.Hour)
	if err := collections.Reseal(ctx, f.db, r.CaseID, v, "LAB-9", f.coordinator.ID, later); err != nil {
		t.Fatal(err)
	}
	if err := collections.BreakSeal(ctx, f.db, r.CaseID, v, f.collector.ID, later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := collections.BreakSeal(ctx, f.db, r.CaseID, v, f.collector.ID, later.Add(2*time.Hour)); !isValidation(err) {
		t.Errorf("broke a seal twice: %v", err)
	}
	if err := collections.BreakSeal(ctx, f.db, r.CaseID, r.Vestiges[1].ID, f.collector.ID, later); !isValidation(err) {
		t.Errorf("broke the seal of an unsealed vestige: %v", err)
	}
	if err := collections.Reseal(ctx, f.db, r.CaseID, v, " ", f.collector.ID, later); !isValidation(err) {
		t.Errorf("sealed with no number: %v", err)
	}
	origins, _ := collections.List(ctx, f.db, r.CaseID)
	seals := origins.Vestiges[0].Seals
	if len(seals) != 2 || seals[0].Number != "C-1" || seals[0].BrokenBy != "coord" || seals[1].Number != "LAB-9" ||
		seals[1].AppliedBy != "coord" || seals[1].BrokenAt == nil || seals[1].BrokenBy != "papilo" {
		t.Errorf("seals: %+v", seals)
	}
	if _, err := f.db.Exec(`INSERT INTO case_vestige_seals (vestige_id, seal_number, applied_at) VALUES ($1, 'X', NOW()), ($1, 'Y', NOW())`, v); err == nil {
		t.Error("the database let a vestige have two unbroken seals")
	}

	other := f.scene(t, collections.RegisterInput{})
	if err := collections.Reseal(ctx, f.db, other.CaseID, v, "Z", f.collector.ID, later); !isValidation(err) {
		t.Errorf("sealed another case's vestige: %v", err)
	}
}

// A scene examined with nothing collected is registered too; it has no vestiges and takes no
// material. The report mode is the case's until its first laudo.
func TestNothingCollected(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in := collections.RegisterInput{CollectedBy: f.collector.ID, CollectedFrom: f.now.Add(-time.Hour), NothingCollected: true,
		Occurrence: &occurrences.CreateInput{Types: []string{"HOME_ROBBERY"}, Description: "x", Place: occurrences.PlaceInput{ExistingID: f.location}},
		Vestiges:   []collections.VestigeInput{{Description: "y"}}}
	if _, err := collections.Register(ctx, f.db, in, f.collector.ID, f.now); !isValidation(err) {
		t.Errorf("nothing collected, with vestiges: %v", err)
	}
	r := f.scene(t, collections.RegisterInput{NothingCollected: true})
	if err := collections.CheckOrigin(ctx, f.db, r.CaseID, collections.Origin{CollectionID: r.CollectionID}); !isValidation(err) {
		t.Errorf("material from a scene that gave nothing: %v", err)
	}
	for _, bad := range []collections.RegisterInput{
		{CollectedFrom: f.now.Add(time.Hour)},
		{CollectedFrom: f.now.Add(-time.Hour), CollectedTo: ptr(f.now.Add(-2 * time.Hour))},
		{CollectedFrom: f.now.Add(-time.Hour), ReportMode: "BOTH"},
	} {
		bad.CollectedBy = f.collector.ID
		bad.Occurrence = in.Occurrence
		if _, err := collections.Register(ctx, f.db, bad, f.collector.ID, f.now); !isValidation(err) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if err := collections.SetReportMode(ctx, f.db, r.CaseID, "TOGETHER"); err != nil {
		t.Fatal(err)
	}
	// An issued laudo fixes it.
	if _, err := f.db.Exec(`WITH a AS (INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type)
			SELECT id, 1, 'FINGERPRINT_COMPARISON' FROM biometric_cases WHERE case_id = $1 RETURNING id)
		INSERT INTO analysis_reports (case_analysis_id, report_type, status, report_year, report_number, issued_by, issued_at, snapshot)
		SELECT a.id, 'FINGERPRINT_COMPARISON_REPORT', 'ISSUED', 2026, 1, $2, NOW(), '{}' FROM a`, r.CaseID, f.coordinator.ID); err != nil {
		t.Fatal(err)
	}
	if err := collections.SetReportMode(ctx, f.db, r.CaseID, "SEPARATE"); !isValidation(err) {
		t.Errorf("changed the report mode after a laudo: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
