package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rodfileto/trackid/db"
)

func TestOccurrenceFeatures(t *testing.T) {
	at := time.Date(2024, 5, 3, 21, 30, 0, 0, time.UTC)
	rows := []db.ListOccurrencesForMapRow{{
		ID: 7, Agency: sql.NullString{String: "DP-005", Valid: true}, OccurrenceNumber: sql.NullString{String: "SIM-005-00001/2024", Valid: true},
		OccurrenceYear: sql.NullInt32{Int32: 2024, Valid: true}, OccurrenceSeq: sql.NullInt32{Int32: 7, Valid: true},
		OccurredFrom: at, RegisteredAt: at,
		Attempted: sql.NullBool{Bool: true, Valid: true}, Status: "REGISTERED", Lon: -43.17, Lat: -22.91,
		Precision: "EXACT", PlaceType: sql.NullString{String: "BANK", Valid: true},
		Types: json.RawMessage(`["FRAUD"]`),
		Cases: json.RawMessage(`[{"caseId":"SIM-2024-00001","modality":"FACIAL","caseType":"CRIMINAL"}]`),
	}}
	types := []db.OccurrenceType{{Code: "FRAUD", CaseType: "CRIMINAL", Grp: "FRAUD"}}
	fs, err := occurrenceFeatures(rows, types)
	if err != nil {
		t.Fatal(err)
	}
	p := fs[0].Properties
	if fs[0].Geometry.Coordinates != [2]float64{-43.17, -22.91} || p.Group != "FRAUD" || p.CaseType != "CRIMINAL" ||
		p.Attempted == nil || !*p.Attempted || p.OccurredTo != nil || len(p.Cases) != 1 || p.Cases[0].Modality != "FACIAL" ||
		p.PlaceType != "BANK" || p.TrackIDNumber != "2024.0007" || p.Agency != "DP-005" {
		t.Errorf("feature: %+v", fs[0])
	}
}

func TestOccurrenceRoutesWithoutDatabase(t *testing.T) {
	router := NewRouter(Dependencies{JWTSecret: testJWTSecret})
	for _, path := range []string{"/api/occurrences/map", "/api/occurrences/types", "/api/occurrences/years"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, authorizedRequest(t, http.MethodGet, path))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status %d, want 503", path, recorder.Code)
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: status %d, want 401", path, recorder.Code)
		}
	}
}
