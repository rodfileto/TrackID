package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rodfileto/trackid/auth"
)

// TestTraceFaceSearch: an intelligence analyst searches the enrolled persons and the other cases' faces from a case's
// face, at a threshold they choose; it is logged as a face search on the case, and refused to anyone else.
func TestTraceFaceSearch(t *testing.T) {
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
	analyst := user("analyst", auth.Access{IntelligenceAnalyst: true})
	examiner := user("examiner", auth.Access{ExaminesFacial: true})

	id := func(query string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := sqlDB.QueryRow(query, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return v
	}
	// vector is a unit vector whose cosine with axis 0 is cos, the rest on its own axis.
	vector := func(cos float64, axis int) string {
		values := make([]string, 512)
		for i := range values {
			values[i] = "0"
		}
		values[0] = fmt.Sprint(cos)
		if axis != 0 {
			values[axis] = fmt.Sprint(math.Sqrt(1 - cos*cos))
		}
		return "[" + strings.Join(values, ",") + "]"
	}
	embed := func(feature int64, v string) {
		t.Helper()
		if _, err := sqlDB.Exec(`INSERT INTO feature_embeddings (biometric_feature_id, embedding_type, embedding) VALUES ($1, 'FACE_AURAFACE_512', $2::vector)`, feature, v); err != nil {
			t.Fatal(err)
		}
	}
	face := func(caseID string, embedded bool, v string) int64 {
		t.Helper()
		c := id(`INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ($1, 'CRIMINAL', 'FACIAL', 'test') RETURNING id`, caseID)
		file := id(`INSERT INTO case_files (biometric_case_id, category, hash_id, storage_ref, content_type) VALUES ($1, 'evidence', $2, 's', 'image/jpeg') RETURNING id`, c, caseID)
		evidence := id(`INSERT INTO case_evidences (biometric_case_id, sequence, case_file_id) VALUES ($1, 1, $2) RETURNING id`, c, file)
		trace := id(`INSERT INTO case_traces (evidence_id, sequence, trace_type, trace_role, box_x1, box_y1, box_x2, box_y2)
			VALUES ($1, 1, 'FACE_CAPTURE', 'QUESTIONED', 1, 2, 3, 4) RETURNING id`, evidence)
		feature := id(`INSERT INTO biometric_features (feature_type, provenance, case_trace_id) VALUES ('FACE_CAPTURE', 'QUESTIONED', $1) RETURNING id`, trace)
		if embedded {
			embed(feature, v)
		}
		return trace
	}
	enrolled := func(n int, v string) {
		t.Helper()
		person := id(`INSERT INTO persons (person_id) VALUES ($1) RETURNING id`, fmt.Sprintf("P-%d", n))
		document := id(`INSERT INTO identity_documents (person_id, document_number, document_type) VALUES ($1, $2, 'RG') RETURNING id`, person, fmt.Sprint(n))
		register := id(`INSERT INTO identity_registers (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender)
			VALUES ($1, $2, $3, '', 'U', '', 'U') RETURNING id`, document, fmt.Sprintf("R-%d", n), fmt.Sprintf("Pessoa %d", n))
		file := id(`INSERT INTO identity_files (register_id, file_type, source_path, storage_ref) VALUES ($1, 'photo', 'p', 's') RETURNING id`, register)
		embed(id(`INSERT INTO biometric_features (feature_type, provenance, identity_file_id) VALUES ('FACE_RECORD', 'KNOWN', $1) RETURNING id`, file), v)
	}
	trace := face("2026.0001", true, vector(1, 0))
	face("2026.0002", true, vector(0.45, 2)) // another case's face, 0.45
	unembedded := face("2026.0003", false, "")
	enrolled(1, vector(0.5, 1)) // 0.5
	enrolled(2, vector(0.3, 3)) // 0.3

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	search := func(u auth.UserProfile, caseID string, traceID int64, body string) *httptest.ResponseRecorder {
		t.Helper()
		token, err := IssueToken(u, testJWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/cases/%s/traces/%d/face-search", caseID, traceID), bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	type result struct {
		MinSimilarity    float64 `json:"minSimilarity"`
		MatcherThreshold float64 `json:"matcherThreshold"`
		Persons          []struct {
			PersonID   string  `json:"personId"`
			Similarity float64 `json:"similarity"`
		} `json:"persons"`
		Cases []struct {
			CaseID string `json:"caseId"`
		} `json:"cases"`
	}
	read := func(r *httptest.ResponseRecorder) result {
		t.Helper()
		if r.Code != http.StatusOK {
			t.Fatalf("%d %s", r.Code, r.Body)
		}
		var out result
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// At 0.4: the person at 0.5 and the other case at 0.45, not the person at 0.3, nor the face itself.
	got := read(search(analyst, "2026.0001", trace, `{"minSimilarity": 0.4}`))
	if got.MinSimilarity != 0.4 || len(got.Persons) != 1 || got.Persons[0].PersonID != "P-1" || math.Abs(got.Persons[0].Similarity-0.5) > 1e-4 ||
		len(got.Cases) != 1 || got.Cases[0].CaseID != "2026.0002" {
		t.Fatalf("at 0.4: %+v", got)
	}
	// Lower, the person at 0.3 too; with no threshold given, the matcher's (the default 0.6): nothing.
	if got := read(search(analyst, "2026.0001", trace, `{"minSimilarity": 0.25}`)); len(got.Persons) != 2 {
		t.Fatalf("at 0.25: %+v", got)
	}
	if got := read(search(analyst, "2026.0001", trace, ``)); got.MinSimilarity != 0.6 || got.MatcherThreshold != 0.6 || len(got.Persons) != 0 || len(got.Cases) != 0 {
		t.Fatalf("the matcher's threshold: %+v", got)
	}

	if r := search(analyst, "2026.0001", trace, `{"minSimilarity": 1.5}`); r.Code != http.StatusBadRequest {
		t.Errorf("similarity 1.5: %d", r.Code)
	}
	if r := search(analyst, "2026.0003", unembedded, `{}`); r.Code != http.StatusConflict {
		t.Errorf("a face with no embedding: %d", r.Code)
	}
	if r := search(analyst, "2026.0002", trace, `{}`); r.Code != http.StatusConflict {
		t.Errorf("a face of another case: %d", r.Code)
	}
	if r := search(examiner, "2026.0001", trace, `{}`); r.Code != http.StatusForbidden {
		t.Errorf("an examiner who isn't an analyst: %d", r.Code)
	}
	var logged int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM access_log WHERE kind = 'FACE_SEARCH' AND subject = '2026.0001' AND username = 'analyst'`).Scan(&logged); err != nil || logged != 4 {
		t.Fatalf("logged %d face searches on the case, %v", logged, err)
	}
}
