package cases_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rodfileto/trackid/db"
)

// TestOccurrenceLayer covers migration 011 through the db queries: a location upserts by its
// business key with its point round-tripping as lon/lat, an occurrence upserts by (agency,
// number) and carries several types, two cases link to one occurrence, and the radius query
// finds it by distance while leaving municipality centroids out unless asked.
func TestOccurrenceLayer(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	q := db.New(sqlDB)

	// Praça da Sé, São Paulo, and a centroid-only location about 1 km away.
	loc, err := q.UpsertLocation(ctx, db.UpsertLocationParams{
		LocationID: "LOC-1", Lon: -46.6340, Lat: -23.5505, Precision: "EXACT",
		MunicipalityCode: "3550308", Uf: "SP",
		PlaceType: sql.NullString{String: "BANK", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !loc.Inserted {
		t.Error("first UpsertLocation: inserted = false")
	}
	again, err := q.UpsertLocation(ctx, db.UpsertLocationParams{
		LocationID: "LOC-1", Lon: -46.6340, Lat: -23.5505, Precision: "EXACT",
		MunicipalityCode: "3550308", Uf: "SP",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Inserted || again.ID != loc.ID {
		t.Errorf("re-upsert: got %+v, want the same row %d", again, loc.ID)
	}
	got, err := q.GetLocationByLocationID(ctx, "LOC-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Lon != -46.6340 || got.Lat != -23.5505 {
		t.Errorf("point round-trip: got (%v, %v)", got.Lon, got.Lat)
	}
	centroid, err := q.UpsertLocation(ctx, db.UpsertLocationParams{
		LocationID: "LOC-2", Lon: -46.6340, Lat: -23.5415, Precision: "MUNICIPALITY",
		MunicipalityCode: "3550308", Uf: "SP",
	})
	if err != nil {
		t.Fatal(err)
	}

	when := time.Date(2026, 3, 14, 22, 0, 0, 0, time.UTC)
	occ := func(number string, location int64) int64 {
		t.Helper()
		row, err := q.UpsertOccurrence(ctx, db.UpsertOccurrenceParams{
			Agency: sql.NullString{String: "DP-01", Valid: true}, OccurrenceNumber: sql.NullString{String: number, Valid: true}, LocationID: location,
			OccurredFrom: when, RegisteredAt: when.Add(2 * time.Hour),
			Attempted: sql.NullBool{Bool: false, Valid: true},
			Status:    "REGISTERED", Description: "bank robbery " + number,
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	o1 := occ("BO-1", loc.ID)
	occ("BO-2", centroid.ID)

	for _, code := range []string{"BANK_ROBBERY", "ROBBERY_HOMICIDE"} {
		if err := q.AddOccurrenceClassification(ctx, db.AddOccurrenceClassificationParams{OccurrenceID: o1, TypeCode: code}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.AddOccurrenceClassification(ctx, db.AddOccurrenceClassificationParams{OccurrenceID: o1, TypeCode: "NOT_A_TYPE"}); err == nil {
		t.Error("unknown type code accepted")
	}

	// One occurrence, two cases: CCTV and latents.
	for _, c := range []struct{ id, modality string }{{"C-FACE", "FACIAL"}, {"C-PRINT", "FINGERPRINT"}} {
		if _, err := q.UpsertBiometricCase(ctx, db.UpsertBiometricCaseParams{CaseID: c.id, CaseType: "CRIMINAL", Modality: c.modality, Description: c.id}); err != nil {
			t.Fatal(err)
		}
		n, err := q.SetBiometricCaseOccurrence(ctx, db.SetBiometricCaseOccurrenceParams{OccurrenceID: sql.NullInt64{Int64: o1, Valid: true}, CaseID: c.id})
		if err != nil || n != 1 {
			t.Fatalf("link %s: %d rows, %v", c.id, n, err)
		}
	}
	var linked int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biometric_cases WHERE occurrence_id = $1`, o1).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked != 2 {
		t.Errorf("cases linked to the occurrence: %d, want 2", linked)
	}

	// 2 km around a point ~500 m east of LOC-1: BO-1 only, unless centroids are included.
	near := func(includeCentroids bool) []db.ListOccurrencesWithinRadiusRow {
		t.Helper()
		rows, err := q.ListOccurrencesWithinRadius(ctx, db.ListOccurrencesWithinRadiusParams{
			Lon: -46.6291, Lat: -23.5505, RadiusM: 2000, IncludeMunicipalityCentroids: includeCentroids,
		})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if rows := near(false); len(rows) != 1 || rows[0].OccurrenceNumber.String != "BO-1" || rows[0].DistanceM < 400 || rows[0].DistanceM > 600 {
		t.Errorf("without centroids: %+v", rows)
	}
	if rows := near(true); len(rows) != 2 {
		t.Errorf("with centroids: %d rows, want 2", len(rows))
	}
}
