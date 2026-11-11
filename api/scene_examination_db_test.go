package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/collections"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/occurrences"
	"github.com/rodfileto/trackid/reports"
	"github.com/rodfileto/trackid/requests"
)

// memReportStore keeps a report's files in memory.
type memReportStore map[string][]byte

func (m memReportStore) Upload(_ context.Context, key string, content []byte, _ string) (string, error) {
	m[key] = content
	return key, nil
}

func (m memReportStore) Download(_ context.Context, ref string) ([]byte, error) {
	return m[ref], nil
}

// TestSceneExamination (docs/scene-examination-plan.md, phase 3): the collector opens the scene
// examination its request asks for, brings a reference document into it, and lists the pairs
// itself; a facial comparison's pairs stay the coordinator's, and a case with no scene has no scene
// examination.
func TestSceneExamination(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(username string, access auth.Access) auth.UserProfile {
		t.Helper()
		access.IsActive, access.AccountRole = true, auth.AccountRoleUser
		u, err := repo.Create(ctx, auth.NewUser{FirstName: username, LastName: "Test", StaffNumber: username, JobTitle: "test",
			Username: username, Password: "password1", Access: access}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	papilo := user("papilo", auth.Access{ExaminesFingerprint: true})
	other := user("other", auth.Access{ExaminesFingerprint: true})
	coordinator := user("coordinator", auth.Access{Coordinator: true})

	var location int64
	if err := sqlDB.QueryRow(`INSERT INTO locations (location_id, geom, precision, municipality_code, uf)
		VALUES ('L-1', ST_GeogFromText('POINT(-47.88 -15.79)'), 'EXACT', '5300108', 'DF') RETURNING id`).Scan(&location); err != nil {
		t.Fatal(err)
	}
	occurrence, err := occurrences.Create(ctx, sqlDB, occurrences.CreateInput{Types: []string{"HOME_ROBBERY"}, Description: "Roubo",
		Place: occurrences.PlaceInput{ExistingID: location}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	scene, err := collections.Register(ctx, sqlDB, collections.RegisterInput{OccurrenceID: &occurrence.ID, CollectedBy: papilo.ID,
		CollectedFrom: time.Now().Add(-2 * time.Hour), Vestiges: []collections.VestigeInput{{Description: "Fragmentos na janela", SealNumber: "A-1"}}},
		papilo.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	reg, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF", DocumentReference: "Requisição 1/2026", ReceivedOn: time.Now().Format("2006-01-02")},
		Cases: []requests.CaseInput{{CaseID: scene.CaseID, CollectionIDs: []int64{scene.CollectionID}}},
	}, coordinator.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	call := func(u auth.UserProfile, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		token, err := IssueToken(u, testJWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest(method, path, &payload)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	casePath := "/api/cases/" + scene.CaseID
	opened := call(papilo, http.MethodPost, casePath+"/analyses", map[string]any{"type": "CRIME_SCENE_EXAMINATION", "requestId": reg.RequestID})
	if opened.Code != http.StatusCreated {
		t.Fatalf("the collector opening the scene examination: %d %s", opened.Code, opened.Body)
	}
	var analysisID int64
	if err := sqlDB.QueryRow(`SELECT id FROM case_analyses WHERE analysis_type = 'CRIME_SCENE_EXAMINATION'`).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}

	// A lift on the scene's evidence, and a reference document the examiner brought in.
	file := func(name string) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRow(`INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename)
			SELECT id, 'evidence', 'image', $2, $2 FROM biometric_cases WHERE case_id = $1 RETURNING id`, scene.CaseID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	trace := func(fileID int64, sequence int, role string) string {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRow(`WITH e AS (INSERT INTO case_evidences (biometric_case_id, sequence, case_file_id)
				SELECT biometric_case_id, $2, id FROM case_files WHERE id = $1 RETURNING id)
			INSERT INTO case_traces (evidence_id, sequence, trace_type, trace_role) SELECT id, 1, 'FINGERPRINT_LIFT', $3 FROM e RETURNING id`,
			fileID, sequence, role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("TRACE:%d#feature", id)
	}
	lift := file("lift.png")
	if err := collections.Attach(ctx, sqlDB, scene.CaseID, lift, collections.Origin{CollectionID: scene.CollectionID, VestigeID: scene.Vestiges[0].ID}); err != nil {
		t.Fatal(err)
	}
	card := file("card.png")
	var validation collections.ValidationError
	if err := collections.Attach(ctx, sqlDB, scene.CaseID, card, collections.Origin{AnalysisID: analysisID, VestigeID: scene.Vestiges[0].ID}); !errors.As(err, &validation) {
		t.Errorf("a brought-in document with a vestige: %v", err)
	}
	if err := collections.Attach(ctx, sqlDB, scene.CaseID, card, collections.Origin{AnalysisID: analysisID}); err != nil {
		t.Fatal(err)
	}
	var origin int64
	if err := sqlDB.QueryRow(`SELECT case_analysis_id FROM case_files WHERE id = $1`, card).Scan(&origin); err != nil || origin != analysisID {
		t.Errorf("the card's origin: %d %v", origin, err)
	}
	if err := collections.Attach(ctx, sqlDB, scene.CaseID, card, collections.Origin{RequestID: reg.RequestID}); !errors.As(err, &validation) {
		t.Errorf("a second origin: %v", err)
	}
	questioned, reference := trace(lift, 1, "QUESTIONED"), trace(card, 2, "REFERENCE")
	pair := map[string]any{"pairs": []analyses.Pair{{FeatureA: questioned, FeatureB: reference}}}
	pairsPath := fmt.Sprintf("%s/analyses/%d/examiner-pairs", casePath, analysisID)
	if got := call(other, http.MethodPost, pairsPath, pair).Code; got != http.StatusForbidden {
		t.Errorf("an examiner off the case listing pairs: %d", got)
	}
	if got := call(papilo, http.MethodPost, pairsPath, pair); got.Code != http.StatusNoContent {
		t.Fatalf("the examiner listing a pair: %d %s", got.Code, got.Body)
	}
	if got := call(papilo, http.MethodPost, pairsPath, pair).Code; got != http.StatusNoContent {
		t.Errorf("the same pair again: %d", got)
	}
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM case_analysis_pairs WHERE case_analysis_id = $1`, analysisID).Scan(&n); err != nil || n != 1 {
		t.Errorf("pairs: %d %v", n, err)
	}
	// The worksheet holds it, and a draft decision keeps it from being taken back.
	sheet := call(papilo, http.MethodGet, fmt.Sprintf("%s/analyses/%d/pairs", casePath, analysisID), nil)
	if sheet.Code != http.StatusOK || !bytes.Contains(sheet.Body.Bytes(), []byte(questioned)) {
		t.Errorf("the worksheet: %d %s", sheet.Code, sheet.Body)
	}
	draft := map[string]any{"featureA": questioned, "featureB": reference, "decision": "POSITIVE", "notes": "12 pontos coincidentes"}
	if got := call(papilo, http.MethodPut, fmt.Sprintf("%s/analyses/%d/decisions", casePath, analysisID), draft); got.Code != http.StatusOK && got.Code != http.StatusNoContent {
		t.Fatalf("a draft decision: %d %s", got.Code, got.Body)
	}
	one := map[string]string{"featureA": reference, "featureB": questioned}
	if got := call(papilo, http.MethodDelete, pairsPath, one).Code; got != http.StatusConflict {
		t.Errorf("taking back a pair with a draft: %d", got)
	}
	if _, err := sqlDB.Exec(`DELETE FROM analysis_decision_drafts`); err != nil {
		t.Fatal(err)
	}
	if got := call(papilo, http.MethodDelete, pairsPath, one).Code; got != http.StatusNoContent {
		t.Errorf("taking back an undecided pair: %d", got)
	}
	// With no pairs, examining goes straight to the report.
	submitted := call(papilo, http.MethodPost, fmt.Sprintf("%s/analyses/%d/submit", casePath, analysisID), nil)
	if submitted.Code != http.StatusOK || !bytes.Contains(submitted.Body.Bytes(), []byte(`"REPORT"`)) {
		t.Errorf("submitting with nothing to decide: %d %s", submitted.Code, submitted.Body)
	}
	if got := call(papilo, http.MethodPost, pairsPath, pair).Code; got != http.StatusConflict {
		t.Errorf("listing a pair after examining: %d", got)
	}

	// The laudo's data: the scene, and the lift marked though it is in no pair.
	data, _, err := reports.Build(ctx, db.New(sqlDB), analysisID, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if data.Scene == nil || len(data.Scene.Collections) != 1 || data.Scene.Collections[0].Collector.Username != "papilo" ||
		len(data.Scene.Vestiges) != 1 || data.Scene.Vestiges[0].Seals[0].Number != "A-1" || data.Scene.Occurrence == nil {
		t.Errorf("the scene: %+v", data.Scene)
	}
	if len(data.Questioned) != 1 || data.Questioned[0].FeatureID != questioned {
		t.Errorf("the lifts marked: %+v", data.Questioned)
	}
	// Core lays no scene laudo out; a deployment's layout is used once registered.
	service := reports.Service{DB: sqlDB, Store: memReportStore{}}
	if _, _, err := service.Draft(ctx, papilo, analysisID); !errors.Is(err, reports.ErrNoLayout) {
		t.Errorf("a scene laudo with no layout: %v", err)
	}
	reports.RegisterLayout("CRIME_SCENE_EXAMINATION_REPORT", func(doc *reports.Document, d reports.Data, _ map[string]reports.Image) error {
		doc.Title("LAUDO DE PERÍCIA PAPILOSCÓPICA EM LOCAL")
		doc.Para("Laudo nº " + d.NumberText() + ", de " + d.IssueDateText())
		doc.Region("history", d.Scene.Collections[0].Notes)
		return nil
	})
	defer reports.RegisterLayout("CRIME_SCENE_EXAMINATION_REPORT", nil)
	if _, docx, err := service.Draft(ctx, papilo, analysisID); err != nil || !bytes.Contains(docx, []byte("PK")) {
		t.Errorf("a scene laudo with a layout: %v", err)
	}

	// A fingerprint case with no scene has no scene examination.
	plain, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input:    requests.Input{Authority: "DPF", DocumentReference: "Ofício 2/2026", ReceivedOn: time.Now().Format("2006-01-02")},
		CaseType: "CRIMINAL", Cases: []requests.CaseInput{{Task: "FINGERPRINT_COMPARISON", Description: "Confronto"}},
	}, coordinator.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analyses.OpenWith(ctx, sqlDB, plain.Cases[0].CaseID, analyses.CrimeSceneExamination, nil, coordinator.ID,
		analyses.Options{RequestID: plain.RequestID}); !errors.Is(err, analyses.ErrNoCollection) {
		t.Errorf("a scene examination with no scene: %v", err)
	}
}
