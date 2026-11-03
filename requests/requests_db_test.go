package requests_test

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

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/occurrences"
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
	all, err := requests.ListAll(ctx, sqlDB, 10)
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
