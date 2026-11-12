package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/occurrences"
)

// TestClusterOccurrences: where a cluster appears is the occurrences of the cases it holds traces of, drawn only where
// they have a place; the counts say what could not be drawn.
func TestClusterOccurrences(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	analyst, err := repo.Create(ctx, auth.NewUser{FirstName: "a", LastName: "b", StaffNumber: "an", JobTitle: "t", Username: "an", Password: "password1",
		Access: auth.Access{AccountRole: auth.AccountRoleUser, IntelligenceAnalyst: true, IsActive: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	occurrence := func(inquiry, precision string, lon, lat float64) int64 {
		t.Helper()
		res, err := occurrences.Ingest(ctx, sqlDB, occurrences.Input{Inquiry: inquiry,
			Location:     occurrences.LocationInput{LocationID: "L-" + inquiry, Lon: lon, Lat: lat, Precision: precision, MunicipalityCode: "3106200", UF: "MG"},
			OccurredFrom: when, RegisteredAt: when, Status: "INQUIRY_OPENED", Description: "d", Types: []string{"FRAUD"}})
		if err != nil {
			t.Fatal(err)
		}
		return res.OccurrenceID
	}
	placed := occurrence("2026.0000001", "EXACT", -43.94, -19.92)
	atUnit := occurrence("2026.0000002", occurrences.PrecisionJurisdiction, -43.94, -19.92)

	traceOf := map[string]int64{}
	addCase := func(caseID string, occ int64) {
		t.Helper()
		c, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: "CRIMINAL", Modality: "FACIAL", Description: "x",
			Evidences: []cases.EvidenceInput{{Sequence: 1, Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_CAPTURE"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		traceOf[caseID] = c.Evidences[0].Traces[0].TraceID
		if occ != 0 {
			if err := occurrences.LinkCase(ctx, sqlDB, caseID, &occ); err != nil {
				t.Fatal(err)
			}
		}
	}
	addCase("A", placed) // two cases of the cluster share one occurrence
	addCase("B", placed)
	addCase("C", atUnit) // an occurrence only at its unit's municipality: counted, not drawn
	addCase("D", 0)      // no occurrence
	addCase("E", placed) // not in the cluster: its occurrence is the same, but it is not this cluster's case
	var clusterID int64
	if err := sqlDB.QueryRow(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', 'AUTOMATIC') RETURNING id`).Scan(&clusterID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B", "C", "D"} {
		if _, err := sqlDB.Exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, 'AUTOMATIC', $2)`, clusterID, graph.QuestionedFeatureID(traceOf[id])); err != nil {
			t.Fatal(err)
		}
	}

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	call := func(path string) (int, PresenceMapResponse) {
		token, err := IssueToken(analyst, testJWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var out PresenceMapResponse
		json.Unmarshal(recorder.Body.Bytes(), &out)
		return recorder.Code, out
	}
	code, got := call(fmt.Sprintf("/api/clusters/%d/occurrences", clusterID))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if got.Cases != 4 || got.WithOccurrence != 3 || got.Occurrences != 2 || got.Unplaced != 1 {
		t.Errorf("counts: %+v; want 4 cases, 3 with an occurrence, 2 occurrences, 1 unplaced", got)
	}
	if len(got.Features) != 1 || got.Features[0].Properties.ID != placed {
		t.Fatalf("features: %+v; want only the placed occurrence", got.Features)
	}
	// The occurrence lists every case it has, and says which are the cluster's own.
	p := got.Features[0].Properties
	if len(p.Cases) != 3 || len(p.InSet) != 2 || p.InSet[0] != "A" || p.InSet[1] != "B" {
		t.Errorf("cases %+v, in set %v; want A, B, E on it and A, B this cluster's", p.Cases, p.InSet)
	}
	if code, _ := call("/api/clusters/999/occurrences"); code != http.StatusNotFound {
		t.Errorf("an unknown cluster: %d", code)
	}
	if code, _ := call("/api/clusters/x/occurrences"); code != http.StatusBadRequest {
		t.Errorf("a bad id: %d", code)
	}
	if code, _ := call("/api/persons/NOPE/occurrences"); code != http.StatusNotFound {
		t.Errorf("an unknown person: %d", code)
	}
}
