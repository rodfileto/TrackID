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

// TestClusterPresence: where a cluster appears is the occurrences of the cases it holds traces of, facial and
// fingerprint together, drawn only where they have a place; the counts say what could not be drawn, and an occurrence
// both kinds of case share is fused.
func TestClusterPresence(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	analyst, err := repo.Create(ctx, auth.NewUser{FirstName: "a", LastName: "b", StaffNumber: "an", JobTitle: "t", Username: "an", Password: "password1",
		Access: auth.Access{AccountRole: auth.AccountRoleUser, IntelligenceAnalyst: true, IsActive: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	occurrence := func(inquiry, precision string, east, north float64) int64 {
		t.Helper()
		res, err := occurrences.Ingest(ctx, sqlDB, occurrences.Input{Inquiry: inquiry,
			Location: occurrences.LocationInput{LocationID: "L-" + inquiry, Lon: -43.94 + east/105000, Lat: -19.92 + north/111000, Precision: precision,
				MunicipalityCode: "3106200", UF: "MG"},
			OccurredFrom: when, RegisteredAt: when, Status: "INQUIRY_OPENED", Description: "d", Types: []string{"FRAUD"}})
		if err != nil {
			t.Fatal(err)
		}
		return res.OccurrenceID
	}
	placed := occurrence("2026.0000001", "EXACT", 0, 0)
	near := occurrence("2026.0000003", "EXACT", 300, 100) // 300 m from it
	far := occurrence("2026.0000004", "EXACT", 80000, 0)  // 80 km away, alone
	atUnit := occurrence("2026.0000002", occurrences.PrecisionJurisdiction, 0, 0)

	traceOf := map[string]int64{}
	addCase := func(caseID, modality string, occ int64) {
		t.Helper()
		traceType := "FACE_CAPTURE"
		if modality == "FINGERPRINT" {
			traceType = graph.FeatureTypeFingerprintLift
		}
		c, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: "CRIMINAL", Modality: modality, Description: "x",
			Evidences: []cases.EvidenceInput{{Sequence: 1, Traces: []cases.TraceInput{{Sequence: 1, TraceType: traceType}}}}})
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
	addCase("A", "FACIAL", placed) // a facial and a fingerprint case on one occurrence: fused
	addCase("B", "FINGERPRINT", placed)
	addCase("C", "FACIAL", atUnit) // only at its unit's municipality: counted, not drawn
	addCase("D", "FACIAL", 0)      // no occurrence
	addCase("E", "FACIAL", placed) // not in the cluster: same occurrence, not its case
	addCase("F", "FACIAL", near)
	addCase("G", "FINGERPRINT", far)

	newCluster := func(basis string, members ...string) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRow(`INSERT INTO clusters (modality, basis) VALUES ('FACIAL', $1) RETURNING id`, basis).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for _, m := range members {
			if _, err := sqlDB.Exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, $2, $3)`, id, basis, graph.QuestionedFeatureID(traceOf[m])); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	automatic := newCluster("AUTOMATIC", "A", "B", "C", "D", "F", "G")
	reviewed := newCluster("REVIEWED", "A", "F")

	router := NewRouter(Dependencies{DB: sqlDB, JWTSecret: testJWTSecret})
	get := func(path string, into any) int {
		t.Helper()
		token, err := IssueToken(analyst, testJWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if into != nil && recorder.Code == http.StatusOK {
			if err := json.Unmarshal(recorder.Body.Bytes(), into); err != nil {
				t.Fatal(err)
			}
		}
		return recorder.Code
	}

	var m PresenceMapResponse
	if code := get(fmt.Sprintf("/api/clusters/%d/occurrences", automatic), &m); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if m.Cases != 6 || m.Facial != 4 || m.Fingerprint != 2 || m.WithOccurrence != 5 || m.Occurrences != 4 || m.Unplaced != 1 || m.Fused != 1 || m.Basis != "AUTOMATIC" {
		t.Errorf("counts: %+v", m)
	}
	if len(m.Features) != 3 {
		t.Fatalf("features %d; want the three placed occurrences", len(m.Features))
	}
	var fused OccurrenceProperties
	for _, f := range m.Features {
		if f.Properties.ID == placed {
			fused = f.Properties
		}
		if f.Properties.Certainty != "unreviewed" {
			t.Errorf("an automatic cluster's link is the matcher's: %q", f.Properties.Certainty)
		}
	}
	if len(fused.Cases) != 3 || len(fused.InSet) != 2 || len(fused.Modalities) != 2 {
		t.Errorf("fused occurrence: cases %d, in set %v, modalities %v; want A, B, E on it, A and B ours, both kinds", len(fused.Cases), fused.InSet, fused.Modalities)
	}

	// Only reviewed links: an automatic cluster has none, a reviewed one has all of its own.
	var none, confirmed PresenceMapResponse
	get(fmt.Sprintf("/api/clusters/%d/occurrences?links=reviewed", automatic), &none)
	get(fmt.Sprintf("/api/clusters/%d/occurrences?links=reviewed", reviewed), &confirmed)
	if none.Cases != 0 || len(none.Features) != 0 || confirmed.Cases != 2 || len(confirmed.Features) != 2 || confirmed.Features[0].Properties.Certainty != "confirmed" {
		t.Errorf("reviewed only: automatic %d cases, reviewed %d cases %d features", none.Cases, confirmed.Cases, len(confirmed.Features))
	}

	// The areas: the two close occurrences make one, the one 80 km away is noise.
	var areas PresenceAreasResponse
	if code := get(fmt.Sprintf("/api/clusters/%d/presence-areas?eps=500", automatic), &areas); code != http.StatusOK {
		t.Fatalf("areas status %d", code)
	}
	if areas.Mapped != 3 || len(areas.Areas) != 1 || len(areas.Areas[0].PointIDs) != 2 || len(areas.Noise) != 1 || areas.Noise[0] != far || areas.TooFew {
		t.Fatalf("areas: %+v", areas)
	}
	if a := areas.Areas[0]; a.Main || a.Fused != 1 || a.Quotient != nil || a.BaseInside != 2 {
		t.Errorf("area: main %v (two points: no main), fused %d, quotient %v, base %d", a.Main, a.Fused, a.Quotient, a.BaseInside)
	}
	if len(areas.Municipalities) != 1 || areas.Municipalities[0].Code != "3106200" || areas.Municipalities[0].Count != 3 {
		t.Errorf("municipalities: %+v", areas.Municipalities)
	}
	if code := get(fmt.Sprintf("/api/clusters/%d/presence-areas?eps=5", automatic), nil); code != http.StatusBadRequest {
		t.Errorf("a distance of 5 m: %d", code)
	}
	if code := get(fmt.Sprintf("/api/clusters/%d/occurrences?links=some", automatic), nil); code != http.StatusBadRequest {
		t.Errorf("a bad links: %d", code)
	}
	for _, path := range []string{"/api/clusters/999/occurrences", "/api/clusters/999/presence-areas", "/api/persons/NOPE/occurrences", "/api/persons/NOPE/presence-areas"} {
		if code := get(path, nil); code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, code)
		}
	}
	if code := get("/api/clusters/x/occurrences", nil); code != http.StatusBadRequest {
		t.Errorf("a bad id: %d", code)
	}
}
