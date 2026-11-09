package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/rodfileto/trackid/auth"
)

// A person page's address carries persons.id: the responses that link to a person give the number,
// and it leads back to the person_id, even for a person_id that is itself all digits and names
// someone else's number.
func TestPersonNumberLeadsToPersonID(t *testing.T) {
	sqlDB := testDB(t)
	ctx := t.Context()
	analyst, err := auth.NewRepository(sqlDB).Create(ctx, auth.NewUser{FirstName: "Ana", LastName: "Lista", StaffNumber: "a1",
		Username: "analyst", Password: "password1", Access: auth.Access{AccountRole: auth.AccountRoleUser, IsActive: true, IntelligenceAnalyst: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	var first, second int64
	if err := sqlDB.QueryRowContext(ctx, `INSERT INTO persons (person_id) VALUES ('P-A') RETURNING id`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(ctx, `INSERT INTO persons (person_id) VALUES ($1) RETURNING id`, strconv.FormatInt(first, 10)).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `WITH d AS (INSERT INTO identity_documents (person_id, document_number, document_type) VALUES ($1, 'D1', 'RG') RETURNING id)
		INSERT INTO identity_registers (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender)
		SELECT id, 'R1', 'JOAO DA SILVA', 'X', 'M', 'Y', 'F' FROM d`, first); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	token, err := IssueToken(analyst, testJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var out map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &out)
		return recorder.Code, out
	}

	code, body := get("/api/persons/search?name=SILVA")
	results, _ := body["results"].([]any)
	if code != http.StatusOK || len(results) != 1 {
		t.Fatalf("search: %d %v", code, body)
	}
	if number := results[0].(map[string]any)["personNumber"]; number != float64(first) {
		t.Errorf("search personNumber = %v, want %d", number, first)
	}

	code, body = get("/api/persons/by-number/" + strconv.FormatInt(first, 10))
	if code != http.StatusOK || body["personId"] != "P-A" {
		t.Errorf("by number %d: %d %v", first, code, body)
	}
	code, body = get("/api/persons/by-number/" + strconv.FormatInt(second, 10))
	if code != http.StatusOK || body["personId"] != strconv.FormatInt(first, 10) {
		t.Errorf("by number %d: %d %v", second, code, body)
	}
	for _, missing := range []string{"999999", "P-A"} {
		if code, _ := get("/api/persons/by-number/" + missing); code != http.StatusNotFound {
			t.Errorf("by number %s: %d, want 404", missing, code)
		}
	}

	code, body = get("/api/persons/P-A/identity")
	if code != http.StatusOK || body["personNumber"] != float64(first) {
		t.Errorf("identity: %d %v", code, body)
	}

	// Deactivation is told apart from any other 403, since the frontend ends the session on it.
	if _, err := sqlDB.ExecContext(ctx, `UPDATE users SET is_active = false WHERE id = $1`, analyst.ID); err != nil {
		t.Fatal(err)
	}
	code, body = get("/api/persons/P-A/identity")
	if code != http.StatusForbidden || body["code"] != accountDeactivatedCode {
		t.Errorf("deactivated: %d %v", code, body)
	}
}
