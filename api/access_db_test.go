package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/distribution"
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

// TestAccessProfiles walks docs/access-profiles-plan.md's access table through the router: what
// an examiner, an analyst, a coordinator and an admin with no capability may reach, and how blind
// work cuts across an analyst's rights.
func TestAccessProfiles(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(username string, access auth.Access) auth.UserProfile {
		t.Helper()
		access.IsActive = true
		if access.AccountRole == "" {
			access.AccountRole = auth.AccountRoleUser
		}
		u, err := repo.Create(ctx, auth.NewUser{FirstName: username, LastName: "Test", StaffNumber: username, JobTitle: "test",
			Username: username, Password: "password1", Access: access}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	examiner := user("examiner", auth.Access{ExaminesFacial: true})
	analyst := user("analyst", auth.Access{IntelligenceAnalyst: true})
	coordinator := user("coordinator", auth.Access{Coordinator: true})
	admin := user("admin", auth.Access{AccountRole: auth.AccountRoleSuperAdmin})
	blind := user("blind", auth.Access{IntelligenceAnalyst: true, ExaminesFacial: true})

	review := func(caseID string) int64 {
		t.Helper()
		if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "test"}); err != nil {
			t.Fatal(err)
		}
		list, err := analyses.List(ctx, sqlDB, caseID)
		if err != nil || len(list) == 0 {
			t.Fatalf("%s analyses: %+v, %v", caseID, list, err)
		}
		return list[0].ID
	}
	held := review("HELD")    // the examiner holds its review
	review("OTHER")           // nobody holds it
	verify := review("BLIND") // the "blind" analyst verifies it blind
	if err := distribution.Assign(ctx, sqlDB, held, &examiner.ID, coordinator.ID); err != nil {
		t.Fatal(err)
	}
	if err := distribution.Assign(ctx, sqlDB, verify, &examiner.ID, coordinator.ID); err != nil {
		t.Fatal(err)
	}
	// A review with no faces goes straight to REPORT when submitted; put this one at VERIFICATION
	// with "blind" holding its REVIEWER step, as distribution would after a submit with pairs.
	if _, err := sqlDB.ExecContext(ctx, `UPDATE case_analyses SET stage = 'VERIFICATION' WHERE id = $1`, verify); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO analysis_assignments (case_analysis_id, user_id, method, assigned_by, step)
		VALUES ($1, $2, 'MANUAL', $3, 'REVIEWER')`, verify, blind.ID, coordinator.ID); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	call := func(u auth.UserProfile, method, path string) *httptest.ResponseRecorder {
		t.Helper()
		token, err := IssueToken(u, testJWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	everyone := []auth.UserProfile{examiner, analyst, coordinator, admin, blind}
	allowed := func(path string, method string, who ...auth.UserProfile) {
		t.Helper()
		yes := map[int64]bool{}
		for _, u := range who {
			yes[u.ID] = true
		}
		for _, u := range everyone {
			got := call(u, method, path).Code != http.StatusForbidden
			if got != yes[u.ID] {
				t.Errorf("%s %s as %s: allowed=%v, want %v", method, path, u.Username, got, yes[u.ID])
			}
		}
	}
	get, post, put := http.MethodGet, http.MethodPost, http.MethodPut

	allowed("/api/persons/search?name=x", get, examiner, analyst)
	allowed("/api/persons/P1/identity", get, examiner, analyst)
	allowed("/api/persons/P1", get, analyst)
	allowed("/api/persons/P1/cases", get, analyst)
	allowed("/api/clusters", get, analyst)
	allowed("/api/occurrences/map", get, analyst)
	// A coordinator registering a request creates the occurrence it cites: the form's routes only.
	allowed("/api/occurrences/types", get, analyst, coordinator)
	allowed("/api/occurrences", post, analyst)
	allowed("/api/admin/requests", get, coordinator)
	allowed("/api/admin/requests", post, coordinator)
	allowed("/api/cases/HELD", get, examiner, analyst, coordinator)
	allowed("/api/cases/OTHER", get, analyst, coordinator)
	allowed("/api/cases/OTHER/evidences", post)
	allowed("/api/cases/HELD/evidences", post, examiner)
	allowed("/api/cases/HELD/occurrence", put, analyst)
	allowed("/api/admin/distribution", get, coordinator)
	allowed(fmt.Sprintf("/api/admin/analyses/%d/prior-deciders", held), get, coordinator)
	allowed("/api/admin/users", get, admin)
	// Blind work: only the workspace of the analysis, and no intelligence anywhere until they submit.
	// (The examiner examined it first, so they read it too.)
	allowed("/api/cases/BLIND", get, examiner, analyst, coordinator)
	allowed(fmt.Sprintf("/api/cases/BLIND/analyses/%d/pairs", verify), get, examiner, analyst, coordinator, blind)

	// The access log: an analyst's reads of a person and of cases they do not work on, and a
	// coordinator's case reads; not an examiner reading their own case, nor anything refused.
	logged := func(username, kind, subject string) int {
		t.Helper()
		var n int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_log WHERE username = $1 AND kind = $2
			AND ($3 = '' OR subject = $3)`, username, kind, subject).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, c := range []struct {
		username, kind, subject string
		want                    bool
	}{
		{"analyst", "PERSON", "P1", true},
		{"analyst", "CASE", "OTHER", true},
		{"coordinator", "CASE", "OTHER", true},
		{"coordinator", "PERSON", "", false},
		{"examiner", "CASE", "", false},
		{"admin", "CASE", "", false},
		{"blind", "CASE", "", false},
		{"blind", "PERSON", "", false},
	} {
		if got := logged(c.username, c.kind, c.subject) > 0; got != c.want {
			t.Errorf("access_log has %s %s %s: %v, want %v", c.username, c.kind, c.subject, got, c.want)
		}
	}

	listed := func(u auth.UserProfile) int {
		t.Helper()
		r := call(u, get, "/api/cases?page_size=100")
		var body struct {
			Total int `json:"total"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil || r.Code != http.StatusOK {
			t.Fatalf("case list as %s: %d %s", u.Username, r.Code, r.Body.String())
		}
		return body.Total
	}
	for _, c := range []struct {
		u    auth.UserProfile
		want int
	}{{examiner, 2}, {analyst, 3}, {coordinator, 3}, {admin, 0}, {blind, 0}} {
		if got := listed(c.u); got != c.want {
			t.Errorf("%s lists %d cases, want %d", c.u.Username, got, c.want)
		}
	}
}

// TestSetUserPassword resets passwords through the router: the new one signs in, a short one is
// refused, and an ADMIN may not reset an admin's.
func TestSetUserPassword(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(username string, role auth.AccountRole) auth.UserProfile {
		t.Helper()
		u, err := repo.Create(ctx, auth.NewUser{FirstName: username, LastName: "Test", StaffNumber: username, JobTitle: "test",
			Username: username, Password: "password1", Access: auth.Access{AccountRole: role, IsActive: true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	super := user("super", auth.AccountRoleSuperAdmin)
	admin := user("admin", auth.AccountRoleAdmin)
	student := user("student", auth.AccountRoleUser)

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	call := func(u *auth.UserProfile, method, path, body string) int {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if u != nil {
			token, err := IssueToken(*u, testJWTSecret)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	reset := func(u auth.UserProfile, target auth.UserProfile, password string) int {
		return call(&u, http.MethodPut, fmt.Sprintf("/api/admin/users/%d/password", target.ID), `{"password":"`+password+`"}`)
	}
	login := func(staffNumber, password string) int {
		return call(nil, http.MethodPost, "/api/auth/login", `{"staff_number":"`+staffNumber+`","password":"`+password+`"}`)
	}

	for _, c := range []struct {
		name          string
		actor, target auth.UserProfile
		password      string
		want          int
	}{
		{"admin resets a student", admin, student, "newpassword", http.StatusOK},
		{"too short", admin, student, "short", http.StatusBadRequest},
		{"admin resets an admin", admin, super, "newpassword", http.StatusForbidden},
		{"student resets a student", student, student, "newpassword", http.StatusForbidden},
		{"super resets an admin", super, admin, "adminpassword", http.StatusOK},
	} {
		if got := reset(c.actor, c.target, c.password); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
	if got := login("student", "newpassword"); got != http.StatusOK {
		t.Errorf("login with the new password: %d", got)
	}
	if got := login("student", "password1"); got != http.StatusUnauthorized {
		t.Errorf("login with the old password: %d", got)
	}
	if got := login("super", "password1"); got != http.StatusOK {
		t.Errorf("super's password changed: %d", got)
	}
}
