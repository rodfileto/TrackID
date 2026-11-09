package requests_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/occurrences"
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

// A request arrives first: registering it creates the occurrence it cites and opens a facial and
// a fingerprint case, both linked to the occurrence, the facial review answering the request.
// A later request joins the facial case as its second, and finds the occurrence by its number.
func TestRegister(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	var locationID int64
	if err := sqlDB.QueryRow(`INSERT INTO locations (location_id, geom, precision, municipality_code, uf)
		VALUES ('L-1', ST_GeogFromText('POINT(-43.18 -22.91)'), 'EXACT', '3304557', 'RJ') RETURNING id`).Scan(&locationID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	occurrence := &occurrences.CreateInput{Types: []string{"STREET_ROBBERY"}, OccurredFrom: now.Add(-72 * time.Hour),
		Description: "Roubo a transeunte", Agency: "37ª DP", AgencyNumber: "037-01234/2026", Place: occurrences.PlaceInput{ExistingID: locationID}}
	in := requests.RegisterInput{
		Input: requests.Input{Authority: "37ª DP", DocumentReference: "Ofício 123/2026", ReceivedOn: "2026-10-27", DueOn: "2026-11-26",
			CitedReference: "037-01234/2026", Questions: []string{"A pessoa da imagem é o suspeito?"}},
		Occurrence: occurrence,
		Cases: []requests.CaseInput{{Task: "FACIAL_REVIEW", Description: "Câmeras do local"},
			{Task: "FINGERPRINT_COMPARISON", Description: "Fragmentos no veículo"}},
	}
	r, err := requests.Register(ctx, sqlDB, in, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Occurrence == nil || r.Occurrence.OccurrenceNumber != "037-01234/2026" || len(r.Cases) != 2 {
		t.Fatalf("registered %+v", r)
	}
	for _, c := range r.Cases {
		if c.CaseType != "CRIMINAL" {
			t.Errorf("case %s: nature %s, want the occurrence's CRIMINAL", c.CaseID, c.CaseType)
		}
		var linked sql.NullInt64
		if err := sqlDB.QueryRow(`SELECT occurrence_id FROM biometric_cases WHERE case_id = $1`, c.CaseID).Scan(&linked); err != nil {
			t.Fatal(err)
		}
		if linked.Int64 != r.Occurrence.ID {
			t.Errorf("case %s linked to %v, want occurrence %d", c.CaseID, linked, r.Occurrence.ID)
		}
		list, err := requests.List(ctx, sqlDB, c.CaseID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != r.RequestID || list[0].Sequence != 1 || len(list[0].Cases) != 2 || len(list[0].Questions) != 1 {
			t.Errorf("case %s requests: %+v", c.CaseID, list)
		}
	}
	facial := r.Cases[0].CaseID
	var answering sql.NullInt64
	if err := sqlDB.QueryRow(`SELECT ca.case_request_id FROM case_analyses ca JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
		WHERE bc.case_id = $1 AND ca.analysis_type = 'FACIAL_REVIEW'`, facial).Scan(&answering); err != nil {
		t.Fatal(err)
	}
	if answering.Int64 != r.RequestID {
		t.Errorf("the facial review answers request %v, want %d", answering, r.RequestID)
	}

	// Material sent with the request: the case lists each file with the request that sent it.
	var fileID int64
	if err := sqlDB.QueryRow(`INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename)
		SELECT id, 'evidence', 'image', 'abc123', 'cctv.jpg' FROM biometric_cases WHERE case_id = $1 RETURNING id`, facial).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	unsent := int64(0)
	if err := sqlDB.QueryRow(`INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename)
		SELECT id, 'evidence', 'image', 'def456', 'found.jpg' FROM biometric_cases WHERE case_id = $1 RETURNING id`, facial).Scan(&unsent); err != nil {
		t.Fatal(err)
	}
	if err := requests.AttachFiles(ctx, sqlDB, facial, r.RequestID, []int64{fileID}); err != nil {
		t.Fatal(err)
	}
	detail, err := cases.Get(ctx, sqlDB, facial)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range detail.Evidences {
		switch f.ID {
		case fileID:
			if f.Request == nil || f.Request.ID != r.RequestID || f.Request.DocumentReference != "Ofício 123/2026" || f.Request.ReceivedOn != "2026-10-27" {
				t.Errorf("sent file's request %+v", f.Request)
			}
		case unsent:
			if f.Request != nil {
				t.Errorf("a file no request sent names request %+v", f.Request)
			}
		}
	}

	// The same occurrence again is refused, not duplicated.
	again := in
	again.CitedReference = ""
	if _, err := requests.Register(ctx, sqlDB, again, 0, now); err == nil {
		t.Error("registered a second occurrence with the same agency number")
	} else if v := (requests.ValidationError("")); !errors.As(err, &v) {
		t.Errorf("duplicate occurrence: %v, want a validation error", err)
	}

	// The fingerprint case's task: a fingerprint comparison, opened empty, answering the request.
	var fpType string
	var fpRequest sql.NullInt64
	var fpPairs int
	if err := sqlDB.QueryRow(`SELECT ca.analysis_type, ca.case_request_id, (SELECT count(*) FROM case_analysis_pairs p WHERE p.case_analysis_id = ca.id)
		FROM case_analyses ca JOIN biometric_cases bc ON bc.id = ca.biometric_case_id WHERE bc.case_id = $1`, r.Cases[1].CaseID).Scan(&fpType, &fpRequest, &fpPairs); err != nil {
		t.Fatal(err)
	}
	if fpType != "FINGERPRINT_COMPARISON" || fpRequest.Int64 != r.RequestID || fpPairs != 0 {
		t.Errorf("fingerprint case's analysis: %s answering %v with %d pairs", fpType, fpRequest, fpPairs)
	}

	// A task must be named, offered, and of the case's modality.
	for task, want := range map[string]string{"": "needs its task", "FINGERPRINT_LAB_EXAMINATION": "not available yet", "FINGERPRINT_COMPARISON": "modality"} {
		_, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
			Input: requests.Input{Authority: "MPRJ", DocumentReference: "Ofício 8/2026", ReceivedOn: "2026-10-28", CitedReference: "037-01234/2026"},
			Cases: []requests.CaseInput{{CaseID: facial, Task: task}},
		}, 0, now)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("task %q on the facial case: %v; want %q", task, err, want)
		}
	}

	// A later ofício on the facial case asks for a comparison: found by its number, numbered 2 on
	// that case, its comparison answering it.
	later, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input: requests.Input{Authority: "MPRJ", DocumentReference: "Ofício 9/2026", ReceivedOn: "2026-10-28", CitedReference: "037-01234/2026"},
		Cases: []requests.CaseInput{{CaseID: facial, Task: "FACIAL_COMPARISON"}},
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if later.Occurrence == nil || later.Occurrence.ID != r.Occurrence.ID {
		t.Errorf("later request's occurrence %+v, want %d", later.Occurrence, r.Occurrence.ID)
	}
	list, err := requests.List(ctx, sqlDB, facial)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].ID != later.RequestID || list[1].Sequence != 2 {
		t.Errorf("facial case requests: %+v", list)
	}
	if fmt.Sprint(list[0].Tasks) != "[FACIAL_REVIEW]" || fmt.Sprint(list[1].Tasks) != "[FACIAL_COMPARISON]" {
		t.Errorf("tasks on the facial case: %v, %v", list[0].Tasks, list[1].Tasks)
	}
	all, err := requests.ListAll(ctx, sqlDB, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != r.RequestID || len(all[0].CaseIDs) != 2 || all[0].OpenAnalyses != 2 ||
		fmt.Sprint(all[0].Tasks) != "[FACIAL_REVIEW FINGERPRINT_COMPARISON]" {
		t.Errorf("all requests: %+v", all)
	}

	// A civil request with no occurrence needs its cases' nature.
	civil := requests.RegisterInput{Input: requests.Input{Authority: "Vara de Família", DocumentReference: "Ofício 1/2026", ReceivedOn: "2026-10-27"},
		Cases: []requests.CaseInput{{Task: "FACIAL_REVIEW", Description: "Pessoa desaparecida"}}}
	if _, err := requests.Register(ctx, sqlDB, civil, 0, now); err == nil {
		t.Error("registered a case with no nature")
	}
	civil.CaseType = "CIVIL"
	got, err := requests.Register(ctx, sqlDB, civil, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Occurrence != nil || got.Cases[0].CaseType != "CIVIL" {
		t.Errorf("civil request: %+v", got)
	}
}

// An inquiry can be the occurrence's only document: a request citing just its IPL creates the
// occurrence with the inquiry number in its own field, a later request citing the IPL finds it,
// and cited numbers naming two different occurrences are refused.
func TestRegisterByInquiry(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	var locationID int64
	if err := sqlDB.QueryRow(`INSERT INTO locations (location_id, geom, precision, municipality_code, uf)
		VALUES ('L-1', ST_GeogFromText('POINT(-43.18 -22.91)'), 'EXACT', '3304557', 'RJ') RETURNING id`).Scan(&locationID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	first, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input: requests.Input{Authority: "DRF", DocumentReference: "Ofício 1/2026", ReceivedOn: "2026-10-27", CitedInquiry: "IPL 912-00045/2026"},
		Occurrence: &occurrences.CreateInput{Types: []string{"FRAUD"}, OccurredFrom: now.Add(-720 * time.Hour),
			Description: "Estelionato apurado por portaria", InquiryNumber: "IPL 912-00045/2026", Place: occurrences.PlaceInput{ExistingID: locationID}},
		Cases: []requests.CaseInput{{Task: "FACIAL_REVIEW", Description: "Imagens do caixa"}},
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Occurrence == nil || first.Occurrence.InquiryNumber != "IPL 912-00045/2026" || first.Occurrence.OccurrenceNumber != "" {
		t.Fatalf("registered occurrence %+v, want the inquiry number alone", first.Occurrence)
	}

	later, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input: requests.Input{Authority: "DRF", DocumentReference: "Ofício 2/2026", ReceivedOn: "2026-10-28", CitedInquiry: "IPL 912-00045/2026"},
		Cases: []requests.CaseInput{{CaseID: first.Cases[0].CaseID, Task: "FACIAL_COMPARISON"}},
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if later.Occurrence == nil || later.Occurrence.ID != first.Occurrence.ID {
		t.Errorf("later request's occurrence %+v, want %d", later.Occurrence, first.Occurrence.ID)
	}
	list, err := requests.List(ctx, sqlDB, first.Cases[0].CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].CitedInquiry != "IPL 912-00045/2026" || list[1].CitedReference != "" {
		t.Errorf("requests: %+v", list)
	}

	other, err := occurrences.Create(ctx, sqlDB, occurrences.CreateInput{Types: []string{"FRAUD"}, OccurredFrom: now.Add(-24 * time.Hour),
		Description: "Outro estelionato", Agency: "12ª DP", AgencyNumber: "012-00999/2026", Place: occurrences.PlaceInput{ExistingID: locationID}}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input: requests.Input{Authority: "DRF", DocumentReference: "Ofício 3/2026", ReceivedOn: "2026-10-28",
			CitedReference: "012-00999/2026", CitedInquiry: "IPL 912-00045/2026"},
		Cases: []requests.CaseInput{{CaseID: first.Cases[0].CaseID, Task: "FACIAL_COMPARISON"}},
	}, 0, now)
	if err == nil || !strings.Contains(err.Error(), "different occurrences") {
		t.Errorf("numbers naming occurrences %d and %d: %v", other.ID, first.Occurrence.ID, err)
	}
}

// TestRegisterConcluded: an import of an old report records its analysis closed when it ended, even
// of a type not offered yet, so nothing reaches the queues or can be worked. Without ConcludedAt the
// type stays unavailable, and a second report on the same case joins it as its next request.
func TestRegisterConcluded(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	concluded := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	in := requests.RegisterInput{
		Input:      requests.Input{Authority: "DPF", DocumentReference: "Despacho 1234567/2026", ReceivedOn: "2026-02-01", CitedInquiry: "2026.0001234", Notes: "Tarefa 1/2026-1"},
		Occurrence: &occurrences.CreateInput{Types: []string{"UNDETERMINED"}, Description: "scene", InquiryNumber: "2026.0001234"},
		Cases:      []requests.CaseInput{{Task: "CRIME_SCENE_EXAMINATION", Description: "Laudo local"}},
	}
	if _, err := requests.Register(ctx, sqlDB, in, 0, now); err == nil || !strings.Contains(err.Error(), "not available yet") {
		t.Errorf("a scene examination that is not an import: %v", err)
	}

	in.ConcludedAt = &concluded
	first, err := requests.Register(ctx, sqlDB, in, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Occurrence == nil || len(first.Cases) != 1 || first.Cases[0].Modality != "FINGERPRINT" || first.Cases[0].CaseType != "CRIMINAL" {
		t.Fatalf("registered %+v", first)
	}
	var analysisID int64
	var analysisType, status, stage string
	var closedAt time.Time
	if err := sqlDB.QueryRow(`SELECT id, analysis_type, status, stage, closed_at FROM case_analyses WHERE case_request_id = $1`, first.RequestID).
		Scan(&analysisID, &analysisType, &status, &stage, &closedAt); err != nil {
		t.Fatal(err)
	}
	if analysisType != "CRIME_SCENE_EXAMINATION" || status != "CLOSED" || stage != "REPORT" || !closedAt.Equal(concluded) {
		t.Errorf("analysis %s %s %s closed %v", analysisType, status, stage, closedAt)
	}
	target, err := analyses.GetTarget(ctx, db.New(sqlDB), analysisID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analyses.CanWork(auth.UserProfile{ID: 1}, target); err == nil {
		t.Error("a concluded analysis can still be worked")
	}

	// A lab report under the same inquiry joins the same occurrence and, by number, the same case.
	second, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input:       requests.Input{Authority: "DPF", DocumentReference: "Despacho 7654321/2026", ReceivedOn: "2026-03-01", CitedInquiry: "2026.0001234"},
		Cases:       []requests.CaseInput{{CaseID: first.Cases[0].CaseID, Task: "FINGERPRINT_LAB_EXAMINATION"}},
		ConcludedAt: &concluded,
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if second.Occurrence == nil || second.Occurrence.ID != first.Occurrence.ID {
		t.Errorf("second occurrence %+v, first %+v", second.Occurrence, first.Occurrence)
	}
	var requestCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM case_request_cases crc JOIN biometric_cases bc ON bc.id = crc.biometric_case_id WHERE bc.case_id = $1`,
		first.Cases[0].CaseID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Errorf("requests on the case: %d", requestCount)
	}
}

// TestReferences: a request keeps its labelled references, without repeats, shows them with the
// case and in the list, and can be found by part of a number. Each needs both its label and number.
func TestReferences(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	in := requests.RegisterInput{
		Input: requests.Input{Authority: "DPF", DocumentReference: "Despacho 1/2026", ReceivedOn: "2026-02-01", References: []requests.Reference{
			{Label: " Tarefa E-POL ", Number: " 1234567/2026-1 "}, {Label: "Tarefa E-POL", Number: "1234567/2026-1"},
			{Label: "Processo SEI", Number: "12345.678901/2026-01"},
		}},
		CaseType: "CRIMINAL",
		Cases:    []requests.CaseInput{{Task: "FINGERPRINT_COMPARISON", Description: "x"}},
	}
	registered, err := requests.Register(ctx, sqlDB, in, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []requests.Reference{{Label: "Tarefa E-POL", Number: "1234567/2026-1"}, {Label: "Processo SEI", Number: "12345.678901/2026-01"}}
	onCase, err := requests.List(ctx, sqlDB, registered.Cases[0].CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(onCase) != 1 || !slices.Equal(onCase[0].References, want) {
		t.Errorf("on the case: %+v", onCase)
	}

	other := in
	other.References = nil
	other.DocumentReference = "Despacho 2/2026"
	if _, err := requests.Register(ctx, sqlDB, other, 0, now); err != nil {
		t.Fatal(err)
	}
	all, err := requests.ListAll(ctx, sqlDB, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("listed %d", len(all))
	}
	found, err := requests.ListAll(ctx, sqlDB, "678901", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != registered.RequestID || !slices.Equal(found[0].References, want) {
		t.Errorf("found %+v", found)
	}

	bad := in
	bad.References = []requests.Reference{{Label: "", Number: "1"}}
	if _, err := requests.Register(ctx, sqlDB, bad, 0, now); err == nil || !strings.Contains(err.Error(), "label and a number") {
		t.Errorf("a reference with no label: %v", err)
	}
}

// TestUpdateAndMove: a registered request can be corrected, its references replaced, and moved to
// the case it belongs to with its analysis, leaving the other case without it.
func TestUpdateAndMove(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	concluded := now.AddDate(0, -1, 0)
	register := func(document string) requests.Registered {
		t.Helper()
		r, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
			Input:       requests.Input{Authority: "A", DocumentReference: document, ReceivedOn: "2026-02-01", References: []requests.Reference{{Label: "L", Number: "1"}}},
			CaseType:    "CRIMINAL",
			Cases:       []requests.CaseInput{{Task: "CRIME_SCENE_EXAMINATION", Description: "x"}},
			ConcludedAt: &concluded,
		}, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first, second := register("Doc 1"), register("Doc 2")
	if err := requests.Update(ctx, sqlDB, first.RequestID, requests.UpdateInput{
		Authority: "B", DocumentReference: "Doc 1b", ReceivedOn: "2026-02-02", DocumentDate: "2026-01-30",
		References: []requests.Reference{{Label: "L", Number: "2"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := requests.Update(ctx, sqlDB, first.RequestID, requests.UpdateInput{Authority: "", DocumentReference: "x", ReceivedOn: "2026-02-02"}); err == nil {
		t.Error("a request with no authority")
	}

	from, to := first.Cases[0].CaseID, second.Cases[0].CaseID
	if err := requests.MoveToCase(ctx, sqlDB, first.RequestID, from, to); err != nil {
		t.Fatal(err)
	}
	onTo, err := requests.List(ctx, sqlDB, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(onTo) != 2 || onTo[1].ID != first.RequestID || onTo[1].Sequence != 2 || onTo[1].Authority != "B" ||
		onTo[1].DocumentDate != "2026-01-30" || !slices.Equal(onTo[1].References, []requests.Reference{{Label: "L", Number: "2"}}) || len(onTo[1].Tasks) != 1 {
		t.Errorf("on the case it moved to: %+v", onTo)
	}
	onFrom, err := requests.List(ctx, sqlDB, from)
	if err != nil {
		t.Fatal(err)
	}
	if len(onFrom) != 0 {
		t.Errorf("left behind: %+v", onFrom)
	}
	if err := requests.MoveToCase(ctx, sqlDB, first.RequestID, from, to); err == nil {
		t.Error("moved a request that is not on that case")
	}
}

// TestRecordImported: an imported report reads as issued, with or without a number, a taken number is
// left out, recording again refreshes it, and a concluded analysis's report can't be issued over it.
func TestRecordImported(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	concluded := now.AddDate(0, -1, 0)
	analysisOf := func(document string) int64 {
		t.Helper()
		r, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
			Input:       requests.Input{Authority: "A", DocumentReference: document, ReceivedOn: "2026-02-01"},
			CaseType:    "CRIMINAL",
			Cases:       []requests.CaseInput{{Task: "CRIME_SCENE_EXAMINATION", Description: "x"}},
			ConcludedAt: &concluded,
		}, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := sqlDB.QueryRow(`SELECT id FROM case_analyses WHERE case_request_id = $1`, r.RequestID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	service := reports.Service{DB: sqlDB}
	first, second := analysisOf("Doc 1"), analysisOf("Doc 2")

	number, err := reports.RecordImported(ctx, sqlDB, reports.ImportedReport{AnalysisID: first, Year: 2026, Number: 12, IssuedAt: concluded}, now)
	if err != nil || number != "2026.0012" {
		t.Fatalf("number %q: %v", number, err)
	}
	overview, err := service.Get(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "ISSUED" || overview.Summary.Preliminary || !overview.Summary.Imported || overview.Number != "2026.0012" {
		t.Errorf("overview %+v", overview)
	}

	number, err = reports.RecordImported(ctx, sqlDB, reports.ImportedReport{AnalysisID: second, Year: 2026, Number: 12, IssuedAt: concluded}, now)
	if err != nil || number != "" {
		t.Errorf("a taken number: %q, %v", number, err)
	}
	overview, err = service.Get(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "ISSUED" || overview.Summary.Preliminary || overview.Number != "" {
		t.Errorf("no number: %+v", overview)
	}
	if _, err := reports.RecordImported(ctx, sqlDB, reports.ImportedReport{AnalysisID: second, Year: 2026, Number: 13, IssuedAt: concluded}, now); err != nil {
		t.Fatal(err)
	}
	if overview, _ = service.Get(ctx, second); overview.Number != "2026.0013" {
		t.Errorf("recorded again: %+v", overview)
	}
	if _, _, _, err := service.Document(ctx, first, "pdf"); err != reports.ErrNoDocument {
		t.Errorf("no PDF attached: %v", err)
	}
}

// TestRequestingUnit covers a request from a unit that doesn't own what it opens, such as a state's
// police asking the federal one.
func TestRequestingUnit(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	var organization, unit, owner int64
	if err := sqlDB.QueryRow(`INSERT INTO organizations (acronym, name, kind, uf) VALUES ('PC/CE', 'Polícia Civil do Ceará', 'STATE', 'CE') RETURNING id`).Scan(&organization); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`INSERT INTO units (organization_id, acronym, name) VALUES ($1, 'PC/CE', 'Polícia Civil do Ceará') RETURNING id`, organization).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`SELECT id FROM units WHERE receives_new_cases`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	input := func(requesting int64) requests.RegisterInput {
		return requests.RegisterInput{
			Input:    requests.Input{Authority: "PC/CE", RequestingUnitID: &requesting, DocumentReference: "Ofício 1/2026", ReceivedOn: "2026-10-27"},
			CaseType: "CRIMINAL",
			Cases:    []requests.CaseInput{{Task: "FACIAL_REVIEW", Description: "Câmeras"}},
		}
	}
	r, err := requests.Register(ctx, sqlDB, input(unit), 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var caseUnit int64
	if err := sqlDB.QueryRow(`SELECT unit_id FROM biometric_cases WHERE case_id = $1`, r.Cases[0].CaseID).Scan(&caseUnit); err != nil {
		t.Fatal(err)
	}
	if caseUnit != owner {
		t.Errorf("case's unit %d, want %d: asking doesn't make the requester its owner", caseUnit, owner)
	}
	list, err := requests.List(ctx, sqlDB, r.Cases[0].CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if got := list[0].RequestingUnitID; got == nil || *got != unit {
		t.Errorf("requesting unit %v, want %d", got, unit)
	}
	var invalid requests.ValidationError
	if _, err := requests.Register(ctx, sqlDB, input(unit+1000), 0, time.Now()); !errors.As(err, &invalid) {
		t.Errorf("unknown requesting unit: %v, want a validation error", err)
	}
}
