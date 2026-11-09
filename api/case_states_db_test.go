package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/distribution"
)

// A case is in the state of the unit responsible for it: the unit's own, its nearest parent's, or for the root of a
// state's organization that organization's. What can be listed by state stays what the user may list.
func TestCasesByState(t *testing.T) {
	ctx := t.Context()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(username string, access auth.Access) auth.UserProfile {
		t.Helper()
		access.AccountRole, access.IsActive = auth.AccountRoleUser, true
		u, err := repo.Create(ctx, auth.NewUser{FirstName: username, LastName: "Test", StaffNumber: username, JobTitle: "test",
			Username: username, Password: "password1", Access: access}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	coordinator := user("coordinator", auth.Access{Coordinator: true})
	examiner := user("examiner", auth.Access{ExaminesFacial: true})

	var root, organization int64
	if err := sqlDB.QueryRow(`SELECT id, organization_id FROM units WHERE receives_new_cases`).Scan(&root, &organization); err != nil {
		t.Fatal(err)
	}
	unit := func(parent int64, acronym string, state any) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRow(`INSERT INTO units (organization_id, parent_id, acronym, name, uf) VALUES ($1, $2, $3, $3, $4) RETURNING id`,
			organization, parent, acronym, state).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	regional := unit(root, "SR/MG", "MG")
	sector := unit(unit(regional, "DPF/X", nil), "SECTOR", nil) // two levels with no state of their own
	saoPaulo := unit(root, "SR/SP", "SP")
	var stateRoot int64
	if err := sqlDB.QueryRow(`WITH o AS (INSERT INTO organizations (acronym, name, kind, uf) VALUES ('PCCE', 'Civil', 'STATE', 'CE') RETURNING id)
		INSERT INTO units (organization_id, acronym, name) SELECT id, 'PCCE', 'Civil' FROM o RETURNING id`).Scan(&stateRoot); err != nil {
		t.Fatal(err)
	}
	for caseID, unitID := range map[string]int64{"NONE": root, "MG1": regional, "MG2": sector, "SP1": saoPaulo, "CE1": stateRoot} {
		if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "test", UnitID: unitID}); err != nil {
			t.Fatal(err)
		}
	}
	openTask(t, sqlDB, "SP1", analyses.FacialReview)
	list, err := analyses.List(ctx, sqlDB, "SP1")
	if err != nil || len(list) == 0 {
		t.Fatalf("analyses: %+v, %v", list, err)
	}
	if err := distribution.Assign(ctx, sqlDB, list[0].ID, &examiner.ID, coordinator.ID); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	get := func(as auth.UserProfile, path string, out any) {
		t.Helper()
		token, err := IssueToken(as, testJWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, recorder.Code, recorder.Body)
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
	listed := func(as auth.UserProfile, query string) []string {
		t.Helper()
		var body ListResponse
		get(as, "/api/cases?"+query, &body)
		ids := []string{}
		for _, item := range body.Items {
			ids = append(ids, item.CaseID)
		}
		sort.Strings(ids)
		if body.Total != len(ids) {
			t.Errorf("%s: total %d, %d listed", query, body.Total, len(ids))
		}
		return ids
	}
	states := func(as auth.UserProfile) []string {
		t.Helper()
		var body struct {
			States []string `json:"states"`
		}
		get(as, "/api/cases/states", &body)
		return body.States
	}

	for query, want := range map[string][]string{
		"state=MG":                      {"MG1", "MG2"},
		"state=mg":                      {"MG1", "MG2"},
		"state=CE":                      {"CE1"},
		"state=SP&modality=FACIAL":      {"SP1"},
		"state=SP&modality=FINGERPRINT": {},
		"state=ZZ":                      {},
		"state=MGX":                     {},
		"state=":                        {"CE1", "MG1", "MG2", "NONE", "SP1"},
	} {
		if got := listed(coordinator, query); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
	}
	if got, want := states(coordinator), []string{"CE", "MG", "SP"}; !reflect.DeepEqual(got, want) {
		t.Errorf("states: %v, want %v", got, want)
	}

	// An examiner lists only the case held, and learns of no other state.
	if got, want := listed(examiner, "state=SP"), []string{"SP1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("examiner in SP: %v, want %v", got, want)
	}
	if got := listed(examiner, "state=MG"); len(got) != 0 {
		t.Errorf("examiner in MG: %v", got)
	}
	if got, want := states(examiner), []string{"SP"}; !reflect.DeepEqual(got, want) {
		t.Errorf("examiner's states: %v, want %v", got, want)
	}
}
