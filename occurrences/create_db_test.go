package occurrences_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/occurrences"
)

// TestCreateAndLink: a registered occurrence is numbered per year, takes its municipality from
// its point, can reuse an existing location, keeps the police number unique, rejects bad input,
// and links only to a case of its own nature.
func TestCreateAndLink(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	q := db.New(sqlDB)
	// A square standing in for Rio de Janeiro's boundary.
	if err := q.UpsertMunicipality(ctx, db.UpsertMunicipalityParams{
		Code: "3304557", Name: "Rio de Janeiro", Uf: "RJ", Source: "test",
		Geojson: `{"type":"Polygon","coordinates":[[[-43.8,-23.1],[-43.1,-23.1],[-43.1,-22.7],[-43.8,-22.7],[-43.8,-23.1]]]}`,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	attempted := true
	in := occurrences.CreateInput{
		Types: []string{"street_robbery", "STREET_ROBBERY"}, Attempted: &attempted,
		OccurredFrom: now.Add(-2 * time.Hour), Description: " robbery at the bus stop ",
		Agency: "DP-005", AgencyNumber: "005-01234/2026",
		Place: occurrences.PlaceInput{Lon: -43.1773, Lat: -22.9104, Address: "Rua Senador Dantas", PlaceType: "government_office"},
	}
	first, err := occurrences.Create(ctx, sqlDB, in, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.TrackIDNumber != "2026.0001" {
		t.Errorf("first number %q", first.TrackIDNumber)
	}
	var muni, prec string
	sqlDB.QueryRow(`SELECT municipality_code, precision FROM locations WHERE id = $1`, first.LocationID).Scan(&muni, &prec)
	if muni != "3304557" || prec != "EXACT" {
		t.Errorf("location: municipality %q, precision %q", muni, prec)
	}

	// The same police number again: refused. Without one, at the same place: 2026.0002.
	if _, err := occurrences.Create(ctx, sqlDB, in, now); !isValidation(err) {
		t.Errorf("duplicate police number: %v", err)
	}
	second := in
	second.Agency, second.AgencyNumber = "", ""
	second.Place = occurrences.PlaceInput{ExistingID: first.LocationID}
	got, err := occurrences.Create(ctx, sqlDB, second, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.TrackIDNumber != "2026.0002" || got.LocationID != first.LocationID {
		t.Errorf("second: %+v", got)
	}

	for name, bad := range map[string]func(*occurrences.CreateInput){
		"no type":              func(c *occurrences.CreateInput) { c.Types = nil },
		"mixed nature":         func(c *occurrences.CreateInput) { c.Types = []string{"STREET_ROBBERY", "MISSING_PERSON"} },
		"civil with attempted": func(c *occurrences.CreateInput) { c.Types = []string{"MISSING_PERSON"} },
		"future":               func(c *occurrences.CreateInput) { c.OccurredFrom = now.Add(time.Hour) },
		"agency alone":         func(c *occurrences.CreateInput) { c.AgencyNumber = "" },
		"outside":              func(c *occurrences.CreateInput) { c.Place = occurrences.PlaceInput{Lon: -46.63, Lat: -23.55} },
	} {
		c := in
		c.AgencyNumber = "x-" + name
		bad(&c)
		if _, err := occurrences.Create(ctx, sqlDB, c, now); !isValidation(err) {
			t.Errorf("%s: err %v, want a validation error", name, err)
		}
	}

	for _, c := range []cases.CaseInput{
		{CaseID: "C-CRIM", CaseType: "CRIMINAL", Modality: "FACIAL", Description: "c"},
		{CaseID: "C-CIV", CaseType: "CIVIL", Modality: "FINGERPRINT", Description: "c"},
	} {
		if _, err := cases.Ingest(ctx, sqlDB, c); err != nil {
			t.Fatal(err)
		}
	}
	if err := occurrences.LinkCase(ctx, sqlDB, "C-CRIM", &first.ID); err != nil {
		t.Fatal(err)
	}
	if err := occurrences.LinkCase(ctx, sqlDB, "C-CIV", &first.ID); !isValidation(err) {
		t.Errorf("civil case to a criminal occurrence: %v", err)
	}
	if err := occurrences.LinkCase(ctx, sqlDB, "C-CRIM", nil); err != nil {
		t.Fatal(err)
	}
	c, _ := q.GetCaseOccurrenceID(ctx, "C-CRIM")
	if c.OccurrenceID.Valid {
		t.Error("unlink left the case linked")
	}

	// From the case-creation flow: created and linked in one go; refused for a case of the
	// other nature (and then nothing is created), or one already linked.
	withCase := second
	withCase.CaseID = "C-CRIM"
	made, err := occurrences.Create(ctx, sqlDB, withCase, now)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := q.GetCaseOccurrenceID(ctx, "C-CRIM"); c.OccurrenceID.Int64 != made.ID || made.CaseID != "C-CRIM" {
		t.Errorf("case-creation flow: case linked to %v, created %+v", c.OccurrenceID, made)
	}
	var before, after int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM occurrences`).Scan(&before)
	for name, id := range map[string]string{"civil case": "C-CIV", "already linked": "C-CRIM", "missing case": "C-NONE"} {
		bad := second
		bad.CaseID = id
		if _, err := occurrences.Create(ctx, sqlDB, bad, now); !isValidation(err) {
			t.Errorf("%s: err %v, want a validation error", name, err)
		}
	}
	sqlDB.QueryRow(`SELECT COUNT(*) FROM occurrences`).Scan(&after)
	if after != before {
		t.Errorf("a refused create left %d occurrences behind", after-before)
	}
}

// TestCreateWithoutPlaceOrDate: a document may give neither where nor when, which leaves them empty
// rather than invented, and the occurrence still shows on its own (its summary and the link
// search) while staying off the map, which needs a point. A place's details need a point too.
func TestCreateWithoutPlaceOrDate(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	q := db.New(sqlDB)
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	created, err := occurrences.Create(ctx, sqlDB, occurrences.CreateInput{
		Types: []string{"UNDETERMINED"}, Description: "a scene report's occurrence", InquiryNumber: "2026.0001234",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.LocationID != 0 {
		t.Errorf("location %d", created.LocationID)
	}
	summary, err := q.GetOccurrenceSummary(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.HasPlace || summary.OccurredFrom.Valid || summary.Lon != 0 || summary.Precision.Valid {
		t.Errorf("summary %+v", summary)
	}
	found, err := q.SearchOccurrences(ctx, "2026.0001234")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != created.ID {
		t.Errorf("search found %+v", found)
	}
	onMap, err := q.ListOccurrencesForMap(ctx, db.ListOccurrencesForMapParams{MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(onMap) != 0 {
		t.Errorf("on the map: %+v", onMap)
	}
	years, err := q.ListOccurrenceYears(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(years) != 0 {
		t.Errorf("years %v", years)
	}

	_, err = occurrences.Create(ctx, sqlDB, occurrences.CreateInput{
		Types: []string{"UNDETERMINED"}, Description: "x", Place: occurrences.PlaceInput{Address: "Rua A, 1"},
	}, now)
	if !isValidation(err) {
		t.Errorf("an address with no point: %v", err)
	}
	to := now
	_, err = occurrences.Create(ctx, sqlDB, occurrences.CreateInput{Types: []string{"UNDETERMINED"}, Description: "x", OccurredTo: &to}, now)
	if !isValidation(err) {
		t.Errorf("an end with no start: %v", err)
	}
}

// TestUpdate: a correction replaces the types, date, inquiry and description, and must keep the
// nature of the cases linked.
func TestUpdate(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	q := db.New(sqlDB)
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	created, err := occurrences.Create(ctx, sqlDB, occurrences.CreateInput{Types: []string{"UNDETERMINED"}, Description: "a", InquiryNumber: "1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := occurrences.Update(ctx, sqlDB, created.ID, occurrences.UpdateInput{
		Types: []string{"FRAUD"}, OccurredFrom: now.Add(-time.Hour), InquiryNumber: "2", Description: "b",
	}, now); err != nil {
		t.Fatal(err)
	}
	summary, err := q.GetOccurrenceSummary(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.OccurredFrom.Valid || summary.InquiryNumber.String != "2" || !strings.Contains(string(summary.Types), "FRAUD") {
		t.Errorf("summary %+v", summary)
	}
	if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "C", CaseType: "CRIMINAL", Modality: "FINGERPRINT", Description: "c"}); err != nil {
		t.Fatal(err)
	}
	if err := occurrences.LinkCase(ctx, sqlDB, "C", &created.ID); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]occurrences.UpdateInput{
		"another nature": {Types: []string{"BODY_FOUND"}, Description: "b"},
		"no types":       {Description: "b"},
		"no description": {Types: []string{"FRAUD"}},
		"future":         {Types: []string{"FRAUD"}, Description: "b", OccurredFrom: now.Add(time.Hour)},
	} {
		if err := occurrences.Update(ctx, sqlDB, created.ID, in, now); !isValidation(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func isValidation(err error) bool {
	var v occurrences.ValidationError
	return errors.As(err, &v)
}
