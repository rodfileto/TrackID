package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rodfileto/trackid/auth"
)

// TestPersonArticleRoutes walks a person's article through the API: a first save, an edit, a
// stale edit answered with 409 and the article as it now is, refused bodies and mentions (422),
// the history, one revision, backlinks, the @ search, and the access log of the reads.
func TestPersonArticleRoutes(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	analyst, err := auth.NewRepository(sqlDB).Create(ctx, auth.NewUser{FirstName: "Ana", LastName: "Lista", StaffNumber: "a1", JobTitle: "analyst",
		Username: "analyst", Password: "password1", Access: auth.Access{AccountRole: auth.AccountRoleUser, IsActive: true, IntelligenceAnalyst: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct{ id, name string }{{"P-A", "JOAO DA SILVA"}, {"P-B", "MARIA SOUZA"}} {
		if _, err := sqlDB.ExecContext(ctx, `WITH p AS (INSERT INTO persons (person_id) VALUES ($1) RETURNING id),
			d AS (INSERT INTO identity_documents (person_id, document_number, document_type) SELECT id, $1, 'RG' FROM p RETURNING id)
			INSERT INTO identity_registers (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender)
			SELECT id, $1, $2, 'X', 'M', 'Y', 'F' FROM d`, p.id, p.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO biometric_cases (case_id, case_type, modality, description)
		VALUES ('2026.0042', 'CRIMINAL', 'FACIAL', 'robbery')`); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	token, err := IssueToken(analyst, testJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var reader *strings.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = strings.NewReader(string(raw))
		} else {
			reader = strings.NewReader("")
		}
		request := httptest.NewRequest(method, path, reader)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var out map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &out)
		return recorder.Code, out
	}
	mention := func(kind, ref string) map[string]any {
		return map[string]any{"type": "mention", "attrs": map[string]any{"kind": kind, "ref": ref, "label": ref}}
	}
	doc := func(inline ...map[string]any) map[string]any {
		return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": inline}}}
	}
	text := func(s string) map[string]any { return map[string]any{"type": "text", "text": s} }
	revisionID := func(body map[string]any) float64 {
		t.Helper()
		article, _ := body["article"].(map[string]any)
		rev, _ := article["revision"].(map[string]any)
		id, _ := rev["id"].(float64)
		if id == 0 {
			t.Fatalf("no revision in %v", body)
		}
		return id
	}

	code, body := call(http.MethodGet, "/api/persons/P-A/article", nil)
	if code != http.StatusOK || body["revision"] != nil {
		t.Fatalf("unwritten article: %d %v", code, body)
	}
	if code, _ := call(http.MethodGet, "/api/persons/NOBODY/article", nil); code != http.StatusNotFound {
		t.Errorf("unknown person: %d", code)
	}

	// A generated draft: logged, returned, not saved.
	code, body = call(http.MethodPost, "/api/persons/P-A/article/template", map[string]any{"language": "en"})
	templateID, _ := body["id"].(float64)
	if code != http.StatusCreated || templateID == 0 || body["body"] == nil {
		t.Fatalf("template: %d %v", code, body)
	}
	if code, _ := call(http.MethodPost, "/api/persons/NOBODY/article/template", nil); code != http.StatusNotFound {
		t.Errorf("template for an unknown person: %d", code)
	}
	if code, body := call(http.MethodGet, "/api/persons/P-A/article", nil); code != http.StatusOK || body["revision"] != nil {
		t.Errorf("a draft saved a revision: %v", body)
	}
	if code, _ := call(http.MethodPut, "/api/persons/P-B/article", map[string]any{"body": doc(text("x")), "templateId": templateID}); code != http.StatusUnprocessableEntity {
		t.Errorf("save with another person's draft: %d", code)
	}

	code, body = call(http.MethodPut, "/api/persons/P-A/article", map[string]any{
		"body": doc(text("Seen in "), mention("CASE", "2026.0042"), text(" with "), mention("PERSON", "P-B")), "editSummary": "first",
		"templateId": templateID,
	})
	if code != http.StatusOK || body["created"] != true {
		t.Fatalf("first save: %d %v", code, body)
	}
	first := revisionID(body)
	if rev := body["article"].(map[string]any)["revision"].(map[string]any); rev["templateId"] != templateID {
		t.Errorf("first revision's draft: %v, want %v", rev["templateId"], templateID)
	}

	code, body = call(http.MethodPut, "/api/persons/P-A/article", map[string]any{
		"baseRevisionId": first, "body": doc(text("Acts with "), mention("PERSON", "P-B")),
	})
	if code != http.StatusOK {
		t.Fatalf("edit: %d %v", code, body)
	}
	second := revisionID(body)

	code, body = call(http.MethodPut, "/api/persons/P-A/article", map[string]any{"baseRevisionId": first, "body": doc(text("stale"))})
	if code != http.StatusConflict || revisionID(body) != second {
		t.Errorf("stale edit: %d %v", code, body)
	}
	code, body = call(http.MethodPut, "/api/persons/P-A/article", map[string]any{"baseRevisionId": second,
		"body": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "image"}}}})
	if code != http.StatusUnprocessableEntity {
		t.Errorf("refused body: %d %v", code, body)
	}
	code, body = call(http.MethodPut, "/api/persons/P-A/article", map[string]any{"baseRevisionId": second,
		"body": doc(mention("CASE", "1999.0001"))})
	if missing, _ := body["missing"].([]any); code != http.StatusUnprocessableEntity || len(missing) != 1 {
		t.Errorf("unknown mention: %d %v", code, body)
	}
	if code, _ := call(http.MethodPut, "/api/persons/P-A/article", map[string]any{"body": nil}); code != http.StatusBadRequest {
		t.Errorf("no body: %d", code)
	}

	code, body = call(http.MethodGet, "/api/persons/P-A/link-counts", nil)
	if cases, _ := body["cases"].(map[string]any); code != http.StatusOK || cases == nil || cases["confirmed"] != float64(0) {
		t.Errorf("link counts: %d %v", code, body)
	}
	if code, _ := call(http.MethodGet, "/api/persons/NOBODY/link-counts", nil); code != http.StatusNotFound {
		t.Errorf("link counts for an unknown person: %d", code)
	}

	code, body = call(http.MethodGet, "/api/persons/P-A/article/history", nil)
	if revisions, _ := body["revisions"].([]any); code != http.StatusOK || len(revisions) != 2 {
		t.Errorf("history: %d %v", code, body)
	}
	code, body = call(http.MethodGet, "/api/persons/P-A/article/revisions/1", nil)
	if code != http.StatusOK || !strings.Contains(body["text"].(string), "@2026.0042") {
		t.Errorf("revision 1: %d %v", code, body)
	}
	if code, _ := call(http.MethodGet, "/api/persons/P-A/article/revisions/x", nil); code != http.StatusBadRequest {
		t.Errorf("bad revision number: %d", code)
	}
	if code, _ := call(http.MethodGet, "/api/persons/P-A/article/revisions/9", nil); code != http.StatusNotFound {
		t.Errorf("missing revision: %d", code)
	}

	code, body = call(http.MethodGet, "/api/persons/P-B/article/backlinks", nil)
	if links, _ := body["backlinks"].([]any); code != http.StatusOK || len(links) != 1 {
		t.Errorf("backlinks to P-B: %d %v", code, body)
	}
	// The case was mentioned only in revision 1.
	code, body = call(http.MethodGet, "/api/cases/2026.0042/article-backlinks", nil)
	if links, _ := body["backlinks"].([]any); code != http.StatusOK || len(links) != 0 {
		t.Errorf("case backlinks: %d %v", code, body)
	}
	if code, _ := call(http.MethodGet, "/api/cases/NOPE/article-backlinks", nil); code != http.StatusNotFound {
		t.Errorf("unknown case backlinks: %d", code)
	}

	code, body = call(http.MethodGet, "/api/mentions/search?q=souza&kinds=person,case", nil)
	if results, _ := body["results"].([]any); code != http.StatusOK || len(results) != 1 {
		t.Errorf("search: %d %v", code, body)
	}
	if code, _ := call(http.MethodGet, "/api/mentions/search?q=x&kinds=vehicle", nil); code != http.StatusBadRequest {
		t.Errorf("unknown kind: %d", code)
	}

	var reads int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_log WHERE username = 'analyst' AND kind = 'PERSON'
		AND subject = 'P-A' AND route LIKE '/api/persons/:personId/article%'`).Scan(&reads); err != nil {
		t.Fatal(err)
	}
	if reads == 0 {
		t.Error("article reads not in access_log")
	}
}
