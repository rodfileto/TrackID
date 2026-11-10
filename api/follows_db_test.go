package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/follows"
)

// TestFollowThroughTheRouter follows a person and a cluster as an analyst, sees the person's change in the bell, and
// checks that only analysts reach the follow routes or see what changed.
func TestFollowThroughTheRouter(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(name string, access auth.Access) auth.UserProfile {
		t.Helper()
		access.IsActive, access.AccountRole = true, auth.AccountRoleUser
		u, err := repo.Create(ctx, auth.NewUser{FirstName: name, LastName: "Test", StaffNumber: name, JobTitle: "test", Username: name,
			Password: "password1", Access: access}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	analyst := user("analyst", auth.Access{IntelligenceAnalyst: true})
	examiner := user("examiner", auth.Access{ExaminesFacial: true})

	const secret = "test-secret"
	server := httptest.NewServer(NewRouter(Dependencies{DB: sqlDB, JWTSecret: secret}))
	defer server.Close()
	call := func(as auth.UserProfile, method, path string, out any) int {
		t.Helper()
		token, err := IssueToken(as, secret)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(ctx, method, server.URL+"/api"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if out != nil && response.StatusCode == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode
	}
	exec := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}

	person := exec(`INSERT INTO persons (person_id) VALUES ('P1') RETURNING id`)
	cluster := exec(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'AUTOMATIC') RETURNING id`)
	clusterPath := "/follows/cluster/" + strconv.FormatInt(cluster, 10)

	var state followState
	if status := call(analyst, http.MethodPut, "/follows/person/P1", &state); status != http.StatusOK || !state.Following || state.Subject.ID != "P1" {
		t.Fatalf("follow a person: %d %+v", status, state)
	}
	if status := call(analyst, http.MethodPut, clusterPath, &state); status != http.StatusOK || !state.Following {
		t.Fatalf("follow a cluster: %d %+v", status, state)
	}
	if status := call(analyst, http.MethodGet, "/follows/person/nobody", nil); status != http.StatusNotFound {
		t.Errorf("unknown person: status %d", status)
	}
	if status := call(analyst, http.MethodGet, "/follows/case/1", nil); status != http.StatusBadRequest {
		t.Errorf("unknown kind: status %d", status)
	}
	if status := call(examiner, http.MethodPut, "/follows/person/P1", nil); status != http.StatusForbidden {
		t.Errorf("an examiner following: status %d", status)
	}
	var list struct {
		Items []follows.Follow `json:"items"`
	}
	if status := call(analyst, http.MethodGet, "/follows", &list); status != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("list: %d %+v", status, list)
	}
	if status := call(analyst, http.MethodDelete, clusterPath, &state); status != http.StatusOK || state.Following {
		t.Fatalf("unfollow: %d %+v", status, state)
	}

	exec(`INSERT INTO identity_documents (person_id, document_number, document_type) VALUES ($1, '1', 'RG') RETURNING id`, person)
	if _, err := follows.Fanout(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	var bell notificationsResponse
	if status := call(analyst, http.MethodGet, "/notifications", &bell); status != http.StatusOK || bell.Unread != 1 || len(bell.Items) != 1 {
		t.Fatalf("bell: %d %+v", status, bell)
	}
	got := bell.Items[0]
	if got.Kind != "PERSON_CHANGED" || got.Changes == nil || got.Changes.PersonNumber != person ||
		len(got.Changes.Changes) != 1 || got.Changes.Changes[0].Change != follows.ChangeDocumentAdded {
		t.Fatalf("notification: %+v %+v", got, got.Changes)
	}
}
