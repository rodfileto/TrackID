package occurrences

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rodfileto/trackid/db"
)

// ValidationError is a create or link error caused by bad input. HTTP handlers map it to a 400.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

var (
	statuses   = []string{"REGISTERED", "UNDER_INVESTIGATION", "INQUIRY_OPENED", "CLOSED"}
	placeTypes = []string{"PUBLIC_ROAD", "RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "TRANSPORT", "RURAL", "OTHER"}
	precisions = []string{"EXACT", "GEOCODED", "STREET", "NEIGHBOURHOOD", "MUNICIPALITY"}
)

// PlaceInput is where a registered occurrence happened: an existing location (ExistingID, from
// the nearby-locations search, so a repeat place stays one row) or a new point. A new point's
// municipality comes from the loaded IBGE boundaries (cmd/load-municipalities).
type PlaceInput struct {
	ExistingID int64

	Lon, Lat      float64
	Precision     string // default EXACT (a point placed on the map)
	Name          string
	Address       string
	Neighbourhood string
	PlaceType     string
}

// CreateInput is an occurrence registered by a user. TrackID numbers it; Agency and AgencyNumber
// are the police agency's own report number, optional, both or neither.
type CreateInput struct {
	Types         []string
	Attempted     *bool
	OccurredFrom  time.Time
	OccurredTo    *time.Time
	ReportedAt    *time.Time
	Status        string // default REGISTERED
	Description   string
	Agency        string
	AgencyNumber  string
	OffenderCount *int16
	VictimCount   *int16
	Place         PlaceInput
	// CaseID, when set, is a case to link the new occurrence to in the same transaction (the
	// case-creation flow): it must exist, have no occurrence yet and be of the occurrence's
	// nature (CRIMINAL or CIVIL).
	CaseID string
}

// Created is a registered occurrence's identity.
type Created struct {
	ID            int64  `json:"id"`
	TrackIDNumber string `json:"trackidNumber"`
	Year          int    `json:"year"`
	LocationID    int64  `json:"locationId"`
	CaseID        string `json:"caseId,omitempty"` // the case linked to it, when CreateInput.CaseID was set
}

const maxNumberingAttempts = 3

// Create validates in, places it (an existing location or a new one in the municipality that
// covers its point) and inserts it with its types, numbering it "<year>.<sequence>" from the
// current year, as cases.Create does: the highest sequence for the year is read inside the
// transaction and incremented, and the unique (occurrence_year, occurrence_seq) index guards
// against a race, retried a few times.
func Create(ctx context.Context, sqlDB *sql.DB, in CreateInput, now time.Time) (Created, error) {
	if sqlDB == nil {
		return Created{}, fmt.Errorf("occurrences: nil db")
	}
	for attempt := 1; ; attempt++ {
		c, err := createNumbered(ctx, sqlDB, in, now)
		if err == nil {
			return c, nil
		}
		if !IsDuplicateNumber(err) || attempt >= maxNumberingAttempts {
			return Created{}, err
		}
	}
}

// IsDuplicateNumber reports whether err is a concurrent create that took the same
// "<year>.<sequence>"; the whole transaction can be retried.
func IsDuplicateNumber(err error) bool {
	return err != nil && strings.Contains(err.Error(), "occurrences_year_seq_key")
}

// normalize trims and checks in, filling defaults.
func normalize(ctx context.Context, q *db.Queries, in *CreateInput, now time.Time) error {
	known, err := q.ListOccurrenceTypes(ctx)
	if err != nil {
		return fmt.Errorf("occurrences: list types: %w", err)
	}
	caseType := map[string]string{}
	for _, t := range known {
		caseType[t.Code] = t.CaseType
	}
	var types []string
	for _, t := range in.Types {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t == "" || slices.Contains(types, t) {
			continue
		}
		if _, ok := caseType[t]; !ok {
			return ValidationError("unknown occurrence type " + t)
		}
		types = append(types, t)
	}
	if len(types) == 0 {
		return ValidationError("at least one occurrence type is required")
	}
	nature := caseType[types[0]]
	for _, t := range types[1:] {
		if caseType[t] != nature {
			return ValidationError("an occurrence's types must all be criminal or all civil")
		}
	}
	in.Types = types
	if in.CaseID = strings.TrimSpace(in.CaseID); in.CaseID != "" {
		c, err := q.GetCaseOccurrenceID(ctx, in.CaseID)
		if errors.Is(err, sql.ErrNoRows) {
			return ValidationError("case not found")
		}
		if err != nil {
			return fmt.Errorf("occurrences: case: %w", err)
		}
		if c.OccurrenceID.Valid {
			return ValidationError("the case is already linked to an occurrence")
		}
		if c.CaseType != nature {
			return ValidationError(fmt.Sprintf("a %s case needs a %s occurrence", strings.ToLower(c.CaseType), strings.ToLower(c.CaseType)))
		}
	}
	if nature == "CIVIL" && (in.Attempted != nil || in.OffenderCount != nil) {
		return ValidationError("attempted and offender count apply to criminal occurrences only")
	}

	if in.OccurredFrom.IsZero() {
		return ValidationError("when the occurrence happened is required")
	}
	if in.OccurredFrom.After(now.Add(5 * time.Minute)) {
		return ValidationError("the occurrence cannot be in the future")
	}
	if in.OccurredTo != nil && (in.OccurredTo.Before(in.OccurredFrom) || in.OccurredTo.After(now.Add(5*time.Minute))) {
		return ValidationError("the end of the occurrence must be after its start and not in the future")
	}
	if in.Status = strings.ToUpper(strings.TrimSpace(in.Status)); in.Status == "" {
		in.Status = "REGISTERED"
	}
	if !slices.Contains(statuses, in.Status) {
		return ValidationError("unknown status " + in.Status)
	}
	if in.Description = strings.TrimSpace(in.Description); in.Description == "" {
		return ValidationError("a description is required")
	}
	in.Agency, in.AgencyNumber = strings.TrimSpace(in.Agency), strings.TrimSpace(in.AgencyNumber)
	if (in.Agency == "") != (in.AgencyNumber == "") {
		return ValidationError("the agency and its report number go together: give both or neither")
	}
	for _, n := range []*int16{in.OffenderCount, in.VictimCount} {
		if n != nil && *n < 0 {
			return ValidationError("counts cannot be negative")
		}
	}

	p := &in.Place
	if p.ExistingID != 0 {
		return nil
	}
	if p.Lon < -180 || p.Lon > 180 || p.Lat < -90 || p.Lat > 90 || (p.Lon == 0 && p.Lat == 0) {
		return ValidationError("where the occurrence happened is required: a point on the map or an existing location")
	}
	if p.Precision = strings.ToUpper(strings.TrimSpace(p.Precision)); p.Precision == "" {
		p.Precision = "EXACT"
	}
	if !slices.Contains(precisions, p.Precision) {
		return ValidationError("unknown location precision " + p.Precision)
	}
	if p.PlaceType = strings.ToUpper(strings.TrimSpace(p.PlaceType)); p.PlaceType != "" && !slices.Contains(placeTypes, p.PlaceType) {
		return ValidationError("unknown place type " + p.PlaceType)
	}
	p.Name, p.Address, p.Neighbourhood = strings.TrimSpace(p.Name), strings.TrimSpace(p.Address), strings.TrimSpace(p.Neighbourhood)
	return nil
}

func createNumbered(ctx context.Context, sqlDB *sql.DB, in CreateInput, now time.Time) (Created, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Created{}, err
	}
	defer tx.Rollback()
	c, err := CreateWith(ctx, db.New(tx), in, now)
	if err != nil {
		return Created{}, err
	}
	if err := tx.Commit(); err != nil {
		return Created{}, err
	}
	return c, nil
}

// CreateWith is Create on q, inside the caller's transaction (registering a request creates the
// occurrence it cites this way). The caller retries its whole transaction when IsDuplicateNumber.
func CreateWith(ctx context.Context, q *db.Queries, in CreateInput, now time.Time) (Created, error) {
	if err := normalize(ctx, q, &in, now); err != nil {
		return Created{}, err
	}
	year := now.Year()
	locationID, err := placeLocation(ctx, q, in.Place)
	if err != nil {
		return Created{}, err
	}
	maxSeq, err := q.MaxOccurrenceSeqForYear(ctx, int32(year))
	if err != nil {
		return Created{}, fmt.Errorf("occurrences: read max sequence: %w", err)
	}
	seq := maxSeq + 1
	row, err := q.CreateOccurrence(ctx, db.CreateOccurrenceParams{
		OccurrenceYear: sql.NullInt32{Int32: int32(year), Valid: true},
		OccurrenceSeq:  sql.NullInt32{Int32: seq, Valid: true},
		Agency:         nullString(in.Agency), OccurrenceNumber: nullString(in.AgencyNumber),
		LocationID: locationID, OccurredFrom: in.OccurredFrom, OccurredTo: nullTime(in.OccurredTo),
		ReportedAt: nullTime(in.ReportedAt), Attempted: nullBool(in.Attempted),
		OffenderCount: nullInt16(in.OffenderCount), VictimCount: nullInt16(in.VictimCount),
		Status: in.Status, Description: in.Description,
	})
	if err != nil {
		if strings.Contains(err.Error(), "occurrences_agency_number_key") {
			return Created{}, ValidationError("an occurrence with this agency and report number is already registered")
		}
		return Created{}, fmt.Errorf("occurrences: create: %w", err)
	}
	for _, t := range in.Types {
		if err := q.AddOccurrenceClassification(ctx, db.AddOccurrenceClassificationParams{OccurrenceID: row.ID, TypeCode: t}); err != nil {
			return Created{}, fmt.Errorf("occurrences: type %s: %w", t, err)
		}
	}
	if in.CaseID != "" {
		if _, err := q.SetBiometricCaseOccurrence(ctx, db.SetBiometricCaseOccurrenceParams{
			OccurrenceID: sql.NullInt64{Int64: row.ID, Valid: true}, CaseID: in.CaseID,
		}); err != nil {
			return Created{}, fmt.Errorf("occurrences: link case %s: %w", in.CaseID, err)
		}
	}
	return Created{ID: row.ID, TrackIDNumber: fmt.Sprintf("%04d.%04d", year, seq), Year: year, LocationID: locationID, CaseID: in.CaseID}, nil
}

// placeLocation returns the location to use: the existing one, or a new one in the municipality
// that covers the point.
func placeLocation(ctx context.Context, q *db.Queries, p PlaceInput) (int64, error) {
	if p.ExistingID != 0 {
		l, err := q.GetLocationByID(ctx, p.ExistingID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ValidationError("the chosen location does not exist")
		}
		if err != nil {
			return 0, fmt.Errorf("occurrences: location: %w", err)
		}
		return l.ID, nil
	}
	m, err := q.GetMunicipalityAt(ctx, db.GetMunicipalityAtParams{Lon: p.Lon, Lat: p.Lat})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ValidationError("the point is outside every loaded municipality (see cmd/load-municipalities)")
	}
	if err != nil {
		return 0, fmt.Errorf("occurrences: municipality: %w", err)
	}
	key := make([]byte, 8)
	if _, err := rand.Read(key); err != nil {
		return 0, err
	}
	loc, err := q.UpsertLocation(ctx, db.UpsertLocationParams{
		LocationID: "TID-L-" + hex.EncodeToString(key), Name: nullString(p.Name), Lon: p.Lon, Lat: p.Lat,
		Precision: p.Precision, MunicipalityCode: m.Code, Uf: m.Uf,
		Neighbourhood: nullString(p.Neighbourhood), Address: nullString(p.Address), PlaceType: nullString(p.PlaceType),
	})
	if err != nil {
		return 0, fmt.Errorf("occurrences: location: %w", err)
	}
	return loc.ID, nil
}

// LinkCase links the case caseID to occurrence occurrenceID, or unlinks it when occurrenceID is
// nil. A case links only to an occurrence of its own nature (CRIMINAL or CIVIL).
func LinkCase(ctx context.Context, sqlDB *sql.DB, caseID string, occurrenceID *int64) error {
	q := db.New(sqlDB)
	c, err := q.GetCaseOccurrenceID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("case not found")
	}
	if err != nil {
		return fmt.Errorf("occurrences: case: %w", err)
	}
	link := sql.NullInt64{}
	if occurrenceID != nil {
		natures, err := q.ListOccurrenceCaseTypes(ctx, *occurrenceID)
		if err != nil {
			return fmt.Errorf("occurrences: occurrence types: %w", err)
		}
		if len(natures) == 0 {
			return ValidationError("occurrence not found")
		}
		if !slices.Contains(natures, c.CaseType) {
			return ValidationError(fmt.Sprintf("a %s case cannot be linked to a %s occurrence", strings.ToLower(c.CaseType), strings.ToLower(natures[0])))
		}
		link = sql.NullInt64{Int64: *occurrenceID, Valid: true}
	}
	if _, err := q.SetBiometricCaseOccurrence(ctx, db.SetBiometricCaseOccurrenceParams{OccurrenceID: link, CaseID: caseID}); err != nil {
		return fmt.Errorf("occurrences: link: %w", err)
	}
	return nil
}
