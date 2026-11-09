package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/collections"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/occurrences"
	"github.com/rodfileto/trackid/requests"
)

// TestSceneCollectionAccess: who may register a scene examined on call, and what the collector
// may then do on its case (docs/scene-collection-plan.md). A case is no longer opened by itself.
func TestSceneCollectionAccess(t *testing.T) {
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
	facial := user("facial", auth.Access{ExaminesFacial: true})
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
	scene := func(collectedBy int64) map[string]any {
		return map[string]any{"occurrenceId": occurrence.ID, "collectedFrom": time.Now().Add(-time.Hour), "collectedById": collectedBy,
			"vestiges": []map[string]string{{"description": "Fragmentos", "sealNumber": "A-1"}}}
	}

	if got := call(facial, http.MethodPost, "/api/collections", scene(0)).Code; got != http.StatusForbidden {
		t.Errorf("a facial examiner registered a scene: %d", got)
	}
	if got := call(papilo, http.MethodPost, "/api/collections", scene(other.ID)).Code; got != http.StatusForbidden {
		t.Errorf("an examiner registered someone else's scene: %d", got)
	}
	if got := call(coordinator, http.MethodPost, "/api/collections", scene(facial.ID)).Code; got != http.StatusBadRequest {
		t.Errorf("a scene collected by a facial examiner: %d", got)
	}
	created := call(papilo, http.MethodPost, "/api/collections", scene(0))
	if created.Code != http.StatusCreated {
		t.Fatalf("the examiner's own scene: %d %s", created.Code, created.Body)
	}
	var r collections.Registered
	if err := json.Unmarshal(created.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if got := call(coordinator, http.MethodPost, "/api/collections", scene(other.ID)).Code; got != http.StatusCreated {
		t.Errorf("a coordinator registering another examiner's scene: %d", got)
	}
	if got := call(papilo, http.MethodPost, "/api/cases", map[string]string{"caseType": "CRIMINAL", "modality": "FACIAL", "description": "x"}).Code; got != http.StatusNotFound {
		t.Errorf("a case opened by itself: %d, want no such route", got)
	}

	// The collector works the case; another examiner does not, until given an analysis.
	casePath := "/api/cases/" + r.CaseID
	if got := call(papilo, http.MethodGet, casePath, nil).Code; got != http.StatusOK {
		t.Errorf("the collector reading their scene case: %d", got)
	}
	if got := call(other, http.MethodGet, casePath, nil).Code; got != http.StatusForbidden {
		t.Errorf("another examiner reading the scene case: %d", got)
	}
	if got := call(papilo, http.MethodGet, casePath+"/collections", nil).Code; got != http.StatusOK {
		t.Errorf("the collector reading the collections: %d", got)
	}
	vestige := map[string]any{"description": "Copo", "sealNumber": "A-2", "collectionId": r.CollectionID}
	if got := call(papilo, http.MethodPost, casePath+"/vestiges", vestige).Code; got != http.StatusCreated {
		t.Errorf("the collector adding a vestige: %d", got)
	}
	if got := call(other, http.MethodPost, casePath+"/vestiges", vestige).Code; got != http.StatusForbidden {
		t.Errorf("another examiner adding a vestige: %d", got)
	}
	if got := call(papilo, http.MethodPost, fmt.Sprintf("%s/vestiges/%d/seals/break", casePath, r.Vestiges[0].ID), nil).Code; got != http.StatusNoContent {
		t.Errorf("the collector breaking a seal: %d", got)
	}
	// The occurrence form is open to fingerprint examiners now; its creation stays with intelligence.
	if got := call(papilo, http.MethodGet, "/api/occurrences/types", nil).Code; got != http.StatusOK {
		t.Errorf("a fingerprint examiner reading occurrence types: %d", got)
	}
	if got := call(papilo, http.MethodPost, "/api/occurrences", map[string]any{}).Code; got != http.StatusForbidden {
		t.Errorf("a fingerprint examiner creating an occurrence by itself: %d", got)
	}

	// Coordinators list what awaits a request and set the report mode; the collector does not.
	if got := call(papilo, http.MethodGet, "/api/admin/collections/awaiting", nil).Code; got != http.StatusForbidden {
		t.Errorf("the examiner listing awaiting collections: %d", got)
	}
	awaiting := call(coordinator, http.MethodGet, "/api/admin/collections/awaiting", nil)
	var list []collections.Awaiting
	if err := json.Unmarshal(awaiting.Body.Bytes(), &list); err != nil || len(list) != 2 {
		t.Errorf("awaiting: %d %s", awaiting.Code, awaiting.Body)
	}
	if got := call(coordinator, http.MethodPut, "/api/admin/cases/"+r.CaseID+"/report-mode", map[string]string{"reportMode": "TOGETHER"}).Code; got != http.StatusNoContent {
		t.Errorf("setting the report mode: %d", got)
	}

	// The collector opens the comparison the lifts call for, once a request answers it, and takes it.
	if got := call(papilo, http.MethodPost, casePath+"/analyses", map[string]any{"type": "FINGERPRINT_COMPARISON"}).Code; got != http.StatusBadRequest {
		t.Errorf("a follow-up naming no request: %d", got)
	}
	if got := call(papilo, http.MethodPost, casePath+"/analyses", map[string]any{"type": "FACIAL_REVIEW", "requestId": 1}).Code; got != http.StatusBadRequest {
		t.Errorf("a follow-up that is not a comparison: %d", got)
	}
	reg, err := requests.Register(ctx, sqlDB, requests.RegisterInput{
		Input: requests.Input{Authority: "DPF", DocumentReference: "Requisição 1/2026", ReceivedOn: time.Now().Format("2006-01-02")},
		Cases: []requests.CaseInput{{CaseID: r.CaseID, CollectionIDs: []int64{r.CollectionID}}},
	}, coordinator.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	followUp := map[string]any{"type": "FINGERPRINT_COMPARISON", "requestId": reg.RequestID}
	if got := call(other, http.MethodPost, casePath+"/analyses", followUp).Code; got != http.StatusForbidden {
		t.Errorf("an examiner off the case opening its comparison: %d", got)
	}
	if got := call(papilo, http.MethodPost, casePath+"/analyses", followUp); got.Code != http.StatusCreated {
		t.Fatalf("the collector opening the comparison: %d %s", got.Code, got.Body)
	}
	mine, err := distribution.AnalysesOf(ctx, sqlDB, papilo.ID)
	if err != nil || len(mine) != 1 || mine[0].CaseID != r.CaseID {
		t.Errorf("the collector's analyses: %+v, %v", mine, err)
	}
}
