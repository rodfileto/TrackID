package occurrences_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/occurrences"
)

// TestJurisdictionIsNoPlace: an occurrence placed at its investigating unit's municipality keeps that municipality,
// but nothing geographic reads it as a place.
func TestJurisdictionIsNoPlace(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	q := db.New(sqlDB)
	when := time.Date(2024, 5, 3, 12, 0, 0, 0, time.UTC)
	lon, lat := -43.9378, -19.9208
	placed, err := occurrences.Ingest(ctx, sqlDB, occurrences.Input{Inquiry: "2024.0000001",
		Location:     occurrences.LocationInput{LocationID: "L-PLACE", Lon: lon, Lat: lat, Precision: "EXACT", MunicipalityCode: "3106200", UF: "MG"},
		OccurredFrom: when, RegisteredAt: when, Status: "INQUIRY_OPENED", Description: "placed", Types: []string{"UNDETERMINED"}})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := occurrences.Ingest(ctx, sqlDB, occurrences.Input{Inquiry: "2024.0000002",
		Location: occurrences.LocationInput{LocationID: "JURISDICTION:SR/PF/MG", Name: "Superintendência", Lon: lon, Lat: lat,
			Precision: occurrences.PrecisionJurisdiction, MunicipalityCode: "3106200", UF: "MG"},
		RegisteredAt: when, Status: "INQUIRY_OPENED", Description: "at the unit", Types: []string{"UNDETERMINED"}})
	if err != nil {
		t.Fatal(err)
	}

	ids := func(rows []int64) map[int64]bool {
		m := map[int64]bool{}
		for _, id := range rows {
			m[id] = true
		}
		return m
	}
	mapRows, err := q.ListOccurrencesForMap(ctx, db.ListOccurrencesForMapParams{MaxRows: 100})
	if err != nil {
		t.Fatal(err)
	}
	var onMap []int64
	for _, r := range mapRows {
		onMap = append(onMap, r.ID)
	}
	if m := ids(onMap); !m[placed.OccurrenceID] || m[unit.OccurrenceID] {
		t.Errorf("map: %v; want the placed occurrence only", onMap)
	}
	near, err := q.ListOccurrencesWithinRadius(ctx, db.ListOccurrencesWithinRadiusParams{Lon: lon, Lat: lat, RadiusM: 1000, IncludeMunicipalityCentroids: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(near) != 1 || near[0].ID != placed.OccurrenceID {
		t.Errorf("within radius: %+v; want the placed occurrence only", near)
	}
	locs, err := q.ListLocationsNear(ctx, db.ListLocationsNearParams{Lon: lon, Lat: lat, RadiusM: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].LocationID != "L-PLACE" {
		t.Errorf("locations offered for reuse: %+v", locs)
	}
	sum, err := q.AskOccurrenceSummary(ctx, db.AskOccurrenceSummaryParams{Uf: sql.NullString{String: "MG", Valid: true},
		PlaceTypes: []string{}, Types: []string{}, Groups: []string{}, Statuses: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Matched != 1 || sum.UnplacedExcluded != 1 {
		t.Errorf("AI summary in MG: %+v; want one matched and one left out for having no place", sum)
	}
	details, err := q.AskOccurrenceDetails(ctx, []int64{unit.OccurrenceID})
	if err != nil || len(details) != 1 || details[0].HasPlace {
		t.Errorf("AI details of the unit's occurrence: %+v, %v; want no place", details, err)
	}

	// The real place replaces it.
	if err := occurrences.SetPlace(ctx, sqlDB, unit.OccurrenceID, occurrences.PlaceInput{ExistingID: placed.LocationID}); err != nil {
		t.Fatal(err)
	}
	if mapRows, err = q.ListOccurrencesForMap(ctx, db.ListOccurrencesForMapParams{MaxRows: 100}); err != nil || len(mapRows) != 2 {
		t.Errorf("map once placed: %d rows, %v; want 2", len(mapRows), err)
	}
}
