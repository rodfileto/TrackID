// Package occurrences is the extension contract for the occurrence layer (MODEL.md section
// 2.3): an organization normalizes its police reports into Input and calls Ingest, which upserts
// the location, the occurrence and its types in one transaction. Cases link to an occurrence
// through cases.CaseInput.Occurrence.
package occurrences

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rodfileto/trackid/db"
)

// LocationInput is a place, keyed by the caller's LocationID: two reports at the same place
// share it. Lon/Lat are WGS 84.
type LocationInput struct {
	LocationID       string
	Name             string // optional
	Lon, Lat         float64
	Precision        string // EXACT, GEOCODED, STREET, NEIGHBOURHOOD or MUNICIPALITY
	MunicipalityCode string // IBGE, 7 digits
	UF               string
	Neighbourhood    string // optional
	Address          string // optional
	PlaceType        string // optional: PUBLIC_ROAD, RESIDENCE, COMMERCE, BANK, TRANSPORT, RURAL, OTHER
}

// Input is one registered occurrence, keyed by (Agency, Number), its police report; or, when it
// has no report (Agency and Number empty), by Inquiry, the inquiry (IPL) that registers it.
// Inquiry is optional on a reported occurrence. Pointer fields are optional; the crime-only ones
// (Attempted, OffenderCount) stay empty on a civil occurrence.
type Input struct {
	Agency       string
	Number       string
	Inquiry      string
	Location     LocationInput
	OccurredFrom time.Time
	OccurredTo   *time.Time
	ReportedAt   *time.Time
	RegisteredAt time.Time
	Attempted    *bool

	OffenderCount *int16
	VictimCount   *int16
	Status        string // REGISTERED, UNDER_INVESTIGATION, INQUIRY_OPENED or CLOSED
	Description   string
	// Types are occurrence_types codes; they replace the occurrence's types.
	Types []string
}

// Result is what Ingest wrote.
type Result struct {
	OccurrenceID int64
	LocationID   int64
	Inserted     bool // false when its key, (Agency, Number) or Inquiry, already existed and was updated
}

// Ingest upserts in.Location, the occurrence and its types in one transaction.
func Ingest(ctx context.Context, sqlDB *sql.DB, in Input) (Result, error) {
	reported := in.Agency != "" || in.Number != ""
	if reported && (in.Agency == "" || in.Number == "") {
		return Result{}, fmt.Errorf("occurrences: Agency and Number go together")
	}
	if !reported && in.Inquiry == "" {
		return Result{}, fmt.Errorf("occurrences: Agency and Number, or Inquiry, are required")
	}
	key := in.Agency + "/" + in.Number
	if !reported {
		key = in.Inquiry
	}
	if in.Location.LocationID == "" {
		return Result{}, fmt.Errorf("occurrences: Location.LocationID is required")
	}
	if len(in.Types) == 0 {
		return Result{}, fmt.Errorf("occurrences: at least one type is required")
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)

	l := in.Location
	loc, err := q.UpsertLocation(ctx, db.UpsertLocationParams{
		LocationID: l.LocationID, Name: nullString(l.Name), Lon: l.Lon, Lat: l.Lat,
		Precision: l.Precision, MunicipalityCode: l.MunicipalityCode, Uf: l.UF,
		Neighbourhood: nullString(l.Neighbourhood), Address: nullString(l.Address), PlaceType: nullString(l.PlaceType),
	})
	if err != nil {
		return Result{}, fmt.Errorf("occurrences: location %s: %w", l.LocationID, err)
	}

	var id int64
	var inserted bool
	if reported {
		occ, err := q.UpsertOccurrence(ctx, db.UpsertOccurrenceParams{
			Agency: nullString(in.Agency), OccurrenceNumber: nullString(in.Number), InquiryNumber: nullString(in.Inquiry),
			LocationID: loc.ID, OccurredFrom: in.OccurredFrom, OccurredTo: nullTime(in.OccurredTo), ReportedAt: nullTime(in.ReportedAt),
			RegisteredAt: in.RegisteredAt, Attempted: nullBool(in.Attempted),
			OffenderCount: nullInt16(in.OffenderCount), VictimCount: nullInt16(in.VictimCount),
			Status: in.Status, Description: in.Description,
		})
		id, inserted = occ.ID, occ.Inserted
		err = wrap(err, key)
	} else {
		occ, err := q.UpsertOccurrenceByInquiry(ctx, db.UpsertOccurrenceByInquiryParams{
			InquiryNumber: nullString(in.Inquiry),
			LocationID:    loc.ID, OccurredFrom: in.OccurredFrom, OccurredTo: nullTime(in.OccurredTo), ReportedAt: nullTime(in.ReportedAt),
			RegisteredAt: in.RegisteredAt, Attempted: nullBool(in.Attempted),
			OffenderCount: nullInt16(in.OffenderCount), VictimCount: nullInt16(in.VictimCount),
			Status: in.Status, Description: in.Description,
		})
		id, inserted = occ.ID, occ.Inserted
		err = wrap(err, key)
	}
	if err != nil {
		return Result{}, err
	}
	if err := q.DeleteOccurrenceClassifications(ctx, id); err != nil {
		return Result{}, fmt.Errorf("occurrences: %s: types: %w", key, err)
	}
	for _, t := range in.Types {
		if err := q.AddOccurrenceClassification(ctx, db.AddOccurrenceClassificationParams{OccurrenceID: id, TypeCode: t}); err != nil {
			return Result{}, fmt.Errorf("occurrences: %s: type %s: %w", key, t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return Result{OccurrenceID: id, LocationID: loc.ID, Inserted: inserted}, nil
}

func wrap(err error, key string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("occurrences: %s: %w", key, err)
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func nullBool(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *b, Valid: true}
}

func nullInt16(n *int16) sql.NullInt16 {
	if n == nil {
		return sql.NullInt16{}
	}
	return sql.NullInt16{Int16: *n, Valid: true}
}
