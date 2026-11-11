// Package organizations manages organizations, their units and where each user works in them
// (docs/organizations-plan.md, phase 1). A user is posted to one unit at a time (lotação) and may be on
// mission in another for a while (missão); both are dated, so a laudo names the unit its examiner worked in on
// its date (UnitOn). Nothing here limits access yet.
package organizations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	"github.com/rodfileto/trackid/db"
)

// Organization kinds.
const (
	KindFederal = "FEDERAL"
	KindState   = "STATE"
)

// Assignment kinds.
const (
	Posting = "POSTING" // lotação
	Mission = "MISSION" // missão
)

// dateLayout is how dates travel: the day, with no time.
const dateLayout = "2006-01-02"

var (
	ErrNotFound = errors.New("not found")
	// ErrInUse is a delete refused because something still names the record: deactivate it instead.
	ErrInUse = errors.New("in use: cases, requests, units below it or assignments name it; deactivate it instead")
	// ErrDuplicate is an acronym already taken.
	ErrDuplicate = errors.New("acronym already in use")
	// ErrOverlap is an assignment overlapping another of the same kind for the same user.
	ErrOverlap = errors.New("overlaps another assignment of the same kind")
)

// ValidationError is input the server refuses as it stands.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

var (
	ufPattern           = regexp.MustCompile(`^[A-Z]{2}$`)
	municipalityPattern = regexp.MustCompile(`^[0-9]{7}$`)
)

// Organization is a legal body that owns cases and issues laudos.
type Organization struct {
	ID        int64  `json:"id"`
	Acronym   string `json:"acronym"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	UF        string `json:"uf,omitempty"`
	Active    bool   `json:"active"`
	UnitCount int    `json:"unit_count"`
}

// Unit is a department, office or sector of an organization; units form a tree with one root each.
type Unit struct {
	ID               int64    `json:"id"`
	OrganizationID   int64    `json:"organization_id"`
	ParentID         *int64   `json:"parent_id"`
	Acronym          string   `json:"acronym"`
	Name             string   `json:"name"`
	UF               string   `json:"uf,omitempty"`
	MunicipalityCode string   `json:"municipality_code,omitempty"`
	ReceivesNewCases bool     `json:"receives_new_cases"`
	Active           bool     `json:"active"`
	LaudoHeader      []string `json:"laudo_header"`
	LaudoName        string   `json:"laudo_name"`
	// CurrentMembers counts today's postings and missions in the unit.
	CurrentMembers int `json:"current_members"`
}

// Assignment is a user's posting or mission in a unit, from StartsOn to EndsOn inclusive ("" is open-ended).
type Assignment struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	UnitID   int64  `json:"unit_id"`
	Kind     string `json:"kind"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on,omitempty"`
	Note     string `json:"note"`
}

// Service reads and writes organizations, units and assignments.
type Service struct{ DB *sql.DB }

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// constraint is the SQLSTATE and constraint of a database error, from either driver.
func constraint(err error) (code, name string) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code, pg.ConstraintName
	}
	var lp *pq.Error
	if errors.As(err, &lp) {
		return string(lp.Code), lp.Constraint
	}
	return "", ""
}

// translate turns constraint violations into the errors callers report.
func translate(err error) error {
	if err == nil {
		return nil
	}
	switch code, name := constraint(err); {
	case code == "23505" && strings.Contains(name, "one_root"):
		return ValidationError("an organization has one root unit")
	case code == "23505":
		return ErrDuplicate
	case code == "23503" && strings.Contains(name, "units_parent_fkey"):
		return ValidationError("a unit's parent is a unit of the same organization")
	case code == "23503":
		return ErrInUse
	case code == "23P01":
		return ErrOverlap
	case code == "23514":
		return ValidationError("invalid values: " + name)
	}
	return err
}

// inUse is a delete's foreign-key violation: something still names the record.
func inUse(err error) error {
	if code, _ := constraint(err); code == "23503" {
		return ErrInUse
	}
	return err
}

func inTx(ctx context.Context, sqlDB *sql.DB, fn func(q *db.Queries) error) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(db.New(tx)); err != nil {
		return translate(err)
	}
	return translate(tx.Commit())
}

// OrganizationInput is what an admin sets on an organization.
type OrganizationInput struct {
	Acronym string `json:"acronym"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	UF      string `json:"uf"`
	Active  *bool  `json:"active"`
}

func (in *OrganizationInput) clean() error {
	in.Acronym, in.Name, in.UF = strings.TrimSpace(in.Acronym), strings.TrimSpace(in.Name), strings.ToUpper(strings.TrimSpace(in.UF))
	switch {
	case in.Acronym == "" || in.Name == "":
		return ValidationError("acronym and name are required")
	case in.Kind != KindFederal && in.Kind != KindState:
		return ValidationError("kind is FEDERAL or STATE")
	case in.Kind == KindState && !ufPattern.MatchString(in.UF):
		return ValidationError("a state's organization names its state (UF)")
	case in.Kind == KindFederal:
		in.UF = ""
	}
	return nil
}

func (s Service) ListOrganizations(ctx context.Context) ([]Organization, error) {
	rows, err := db.New(s.DB).ListOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Organization, len(rows))
	for i, r := range rows {
		out[i] = Organization{ID: r.ID, Acronym: r.Acronym, Name: r.Name, Kind: r.Kind, UF: r.Uf.String, Active: r.Active, UnitCount: int(r.UnitCount)}
	}
	return out, nil
}

// CreateOrganization creates an organization with its root unit, named as it is.
func (s Service) CreateOrganization(ctx context.Context, in OrganizationInput) (int64, error) {
	if err := in.clean(); err != nil {
		return 0, err
	}
	var id int64
	err := inTx(ctx, s.DB, func(q *db.Queries) error {
		var err error
		if id, err = q.CreateOrganization(ctx, db.CreateOrganizationParams{Acronym: in.Acronym, Name: in.Name, Kind: in.Kind, Uf: nullString(in.UF)}); err != nil {
			return err
		}
		return q.CreateRootUnit(ctx, db.CreateRootUnitParams{OrganizationID: id, Acronym: in.Acronym, Name: in.Name, Uf: nullString(in.UF)})
	})
	return id, err
}

func (s Service) UpdateOrganization(ctx context.Context, id int64, in OrganizationInput) error {
	if err := in.clean(); err != nil {
		return err
	}
	active := in.Active == nil || *in.Active
	n, err := db.New(s.DB).UpdateOrganization(ctx, db.UpdateOrganizationParams{ID: id, Acronym: in.Acronym, Name: in.Name, Kind: in.Kind, Uf: nullString(in.UF), Active: active})
	if err != nil {
		return translate(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOrganization deletes an organization that has only its root unit, and nothing names either.
func (s Service) DeleteOrganization(ctx context.Context, id int64) error {
	return inTx(ctx, s.DB, func(q *db.Queries) error {
		units, err := q.CountOrganizationUnits(ctx, id)
		if err != nil {
			return err
		}
		if units > 1 {
			return ErrInUse
		}
		if err := q.DeleteOrganizationRoot(ctx, id); err != nil {
			return inUse(err)
		}
		n, err := q.DeleteOrganization(ctx, id)
		if err == nil && n == 0 {
			err = ErrNotFound
		}
		return inUse(err)
	})
}

// UnitInput is what an admin sets on a unit. LaudoHeader is the laudo header's lines, empty to take them
// from the unit above.
type UnitInput struct {
	ParentID         *int64   `json:"parent_id"`
	Acronym          string   `json:"acronym"`
	Name             string   `json:"name"`
	UF               string   `json:"uf"`
	MunicipalityCode string   `json:"municipality_code"`
	LaudoHeader      []string `json:"laudo_header"`
	LaudoName        string   `json:"laudo_name"`
	Active           *bool    `json:"active"`
	// ReceivesNewCases, when true, makes this the unit that new cases naming none go to. It moves from
	// whichever unit had it; there is always one, so it cannot be set false.
	ReceivesNewCases bool `json:"receives_new_cases"`
}

func (in *UnitInput) clean() error {
	in.Acronym, in.Name, in.LaudoName = strings.TrimSpace(in.Acronym), strings.TrimSpace(in.Name), strings.TrimSpace(in.LaudoName)
	in.UF, in.MunicipalityCode = strings.ToUpper(strings.TrimSpace(in.UF)), strings.TrimSpace(in.MunicipalityCode)
	header := []string{}
	for _, line := range in.LaudoHeader {
		if line = strings.TrimSpace(line); line != "" {
			header = append(header, line)
		}
	}
	in.LaudoHeader = header
	switch {
	case in.Acronym == "" || in.Name == "":
		return ValidationError("acronym and name are required")
	case in.UF != "" && !ufPattern.MatchString(in.UF):
		return ValidationError("UF is two letters")
	case in.MunicipalityCode != "" && !municipalityPattern.MatchString(in.MunicipalityCode):
		return ValidationError("the municipality is IBGE's seven-digit code")
	case in.MunicipalityCode != "" && in.UF == "":
		return ValidationError("a unit with a municipality names its state (UF)")
	}
	return nil
}

func unitFrom(r db.ListUnitsRow) Unit {
	u := Unit{ID: r.ID, OrganizationID: r.OrganizationID, Acronym: r.Acronym, Name: r.Name, UF: r.Uf.String, MunicipalityCode: r.MunicipalityCode.String,
		ReceivesNewCases: r.ReceivesNewCases, Active: r.Active, LaudoHeader: r.LaudoHeader, LaudoName: r.LaudoName, CurrentMembers: int(r.CurrentMembers)}
	if r.ParentID.Valid {
		p := r.ParentID.Int64
		u.ParentID = &p
	}
	if u.LaudoHeader == nil {
		u.LaudoHeader = []string{}
	}
	return u
}

// ListUnits lists every unit of every organization.
func (s Service) ListUnits(ctx context.Context) ([]Unit, error) {
	rows, err := db.New(s.DB).ListUnits(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Unit, len(rows))
	for i, r := range rows {
		out[i] = unitFrom(r)
	}
	return out, nil
}

// CreateUnit creates a unit below another; its organization is its parent's.
func (s Service) CreateUnit(ctx context.Context, in UnitInput) (int64, error) {
	if err := in.clean(); err != nil {
		return 0, err
	}
	if in.ParentID == nil {
		return 0, ValidationError("a new unit is placed below another")
	}
	var id int64
	err := inTx(ctx, s.DB, func(q *db.Queries) error {
		parent, err := q.GetUnit(ctx, *in.ParentID)
		if errors.Is(err, sql.ErrNoRows) {
			return ValidationError("the parent unit does not exist")
		}
		if err != nil {
			return err
		}
		if id, err = q.CreateUnit(ctx, db.CreateUnitParams{OrganizationID: parent.OrganizationID, ParentID: sql.NullInt64{Int64: parent.ID, Valid: true},
			Acronym: in.Acronym, Name: in.Name, Uf: nullString(in.UF), MunicipalityCode: nullString(in.MunicipalityCode),
			LaudoHeader: in.LaudoHeader, LaudoName: in.LaudoName}); err != nil {
			return err
		}
		if in.ReceivesNewCases {
			return receive(ctx, q, id)
		}
		return nil
	})
	return id, err
}

func receive(ctx context.Context, q *db.Queries, id int64) error {
	if err := q.ClearCaseReceiver(ctx, id); err != nil {
		return err
	}
	return q.SetCaseReceiver(ctx, id)
}

// UpdateUnit changes a unit. A root unit stays the root; any other moves only below a unit of its own
// organization that is not below itself.
func (s Service) UpdateUnit(ctx context.Context, id int64, in UnitInput) error {
	if err := in.clean(); err != nil {
		return err
	}
	return inTx(ctx, s.DB, func(q *db.Queries) error {
		current, err := q.GetUnit(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		parent := current.ParentID
		switch {
		case !current.ParentID.Valid && in.ParentID != nil:
			return ValidationError("the root unit stays the root")
		case current.ParentID.Valid && in.ParentID == nil:
			return ValidationError("only the root unit has no parent")
		case in.ParentID != nil && *in.ParentID != current.ParentID.Int64:
			line, err := q.UnitAncestors(ctx, *in.ParentID)
			if err != nil {
				return err
			}
			if len(line) == 0 {
				return ValidationError("the parent unit does not exist")
			}
			for _, a := range line {
				if a.ID == id {
					return ValidationError("a unit cannot be placed below itself")
				}
			}
			parent = sql.NullInt64{Int64: *in.ParentID, Valid: true}
		}
		active := in.Active == nil || *in.Active
		if !active && current.ReceivesNewCases {
			return ValidationError("the unit receiving new cases stays active; give that to another unit first")
		}
		if _, err := q.UpdateUnit(ctx, db.UpdateUnitParams{ID: id, ParentID: parent, Acronym: in.Acronym, Name: in.Name, Uf: nullString(in.UF),
			MunicipalityCode: nullString(in.MunicipalityCode), LaudoHeader: in.LaudoHeader, LaudoName: in.LaudoName, Active: active}); err != nil {
			return err
		}
		if in.ReceivesNewCases && !current.ReceivesNewCases {
			return receive(ctx, q, id)
		}
		return nil
	})
}

// DeleteUnit deletes a unit nothing names: no case, request, unit below it or assignment, past ones
// included. A root unit goes with its organization.
func (s Service) DeleteUnit(ctx context.Context, id int64) error {
	return inTx(ctx, s.DB, func(q *db.Queries) error {
		current, err := q.GetUnit(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !current.ParentID.Valid {
			return ValidationError("the root unit goes with its organization")
		}
		if current.ReceivesNewCases {
			return ValidationError("the unit receiving new cases stays; give that to another unit first")
		}
		_, err = q.DeleteUnit(ctx, id)
		return inUse(err)
	})
}

// AssignmentInput is a posting or mission as an admin sets it. Dates are YYYY-MM-DD; EndsOn "" is open-ended.
type AssignmentInput struct {
	UserID   int64  `json:"user_id"`
	UnitID   int64  `json:"unit_id"`
	Kind     string `json:"kind"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
	Note     string `json:"note"`
}

type dates struct {
	start time.Time
	end   sql.NullTime
}

func (in *AssignmentInput) clean() (dates, error) {
	in.Note = strings.TrimSpace(in.Note)
	var d dates
	start, err := time.Parse(dateLayout, strings.TrimSpace(in.StartsOn))
	if err != nil {
		return d, ValidationError("the start date is YYYY-MM-DD")
	}
	d.start = start
	if e := strings.TrimSpace(in.EndsOn); e != "" {
		end, err := time.Parse(dateLayout, e)
		if err != nil {
			return d, ValidationError("the end date is YYYY-MM-DD")
		}
		if end.Before(start) {
			return d, ValidationError("the end date is not before the start")
		}
		d.end = sql.NullTime{Time: end, Valid: true}
	}
	return d, nil
}

func formatDate(t time.Time) string { return t.Format(dateLayout) }

// ListAssignments lists every posting and mission, past ones included, each user's latest first.
func (s Service) ListAssignments(ctx context.Context) ([]Assignment, error) {
	rows, err := db.New(s.DB).ListUnitAssignments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Assignment, len(rows))
	for i, r := range rows {
		a := Assignment{ID: r.ID, UserID: r.UserID, UnitID: r.UnitID, Kind: r.Kind, StartsOn: formatDate(r.StartsOn), Note: r.Note}
		if r.EndsOn.Valid {
			a.EndsOn = formatDate(r.EndsOn.Time)
		}
		out[i] = a
	}
	return out, nil
}

// CreateAssignment records a posting or mission in an active unit. A posting that starts after the user's
// open-ended one is a transfer: the open one ends the day before.
func (s Service) CreateAssignment(ctx context.Context, in AssignmentInput, by int64) (int64, error) {
	if in.Kind != Posting && in.Kind != Mission {
		return 0, ValidationError("kind is POSTING or MISSION")
	}
	d, err := in.clean()
	if err != nil {
		return 0, err
	}
	var id int64
	err = inTx(ctx, s.DB, func(q *db.Queries) error {
		unit, err := q.GetUnit(ctx, in.UnitID)
		if errors.Is(err, sql.ErrNoRows) {
			return ValidationError("the unit does not exist")
		}
		if err != nil {
			return err
		}
		if !unit.Active {
			return ValidationError("the unit is inactive")
		}
		if in.Kind == Posting {
			if err := q.EndOpenPosting(ctx, db.EndOpenPostingParams{UserID: in.UserID, StartsOn: d.start}); err != nil {
				return err
			}
		}
		id, err = q.CreateUnitAssignment(ctx, db.CreateUnitAssignmentParams{UserID: in.UserID, UnitID: in.UnitID, Kind: in.Kind, StartsOn: d.start,
			EndsOn: d.end, Note: in.Note, CreatedBy: sql.NullInt64{Int64: by, Valid: by != 0}})
		if code, name := constraint(err); code == "23503" && strings.Contains(name, "user_id") {
			return ValidationError("the user does not exist")
		}
		return err
	})
	return id, err
}

// UpdateAssignment changes an assignment's unit, dates or note; its user and kind stay.
func (s Service) UpdateAssignment(ctx context.Context, id int64, in AssignmentInput) error {
	d, err := in.clean()
	if err != nil {
		return err
	}
	return inTx(ctx, s.DB, func(q *db.Queries) error {
		current, err := q.GetUnitAssignment(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if in.UnitID != current.UnitID {
			unit, err := q.GetUnit(ctx, in.UnitID)
			if errors.Is(err, sql.ErrNoRows) {
				return ValidationError("the unit does not exist")
			}
			if err != nil {
				return err
			}
			if !unit.Active {
				return ValidationError("the unit is inactive")
			}
		}
		_, err = q.UpdateUnitAssignment(ctx, db.UpdateUnitAssignmentParams{ID: id, UnitID: in.UnitID, StartsOn: d.start, EndsOn: d.end, Note: in.Note})
		return err
	})
}

// DeleteAssignment deletes an assignment recorded by mistake. One that ended is history: end it, don't delete
// it, when it was real (laudos issued meanwhile keep the unit they named either way).
func (s Service) DeleteAssignment(ctx context.Context, id int64) error {
	n, err := db.New(s.DB).DeleteUnitAssignment(ctx, id)
	if err != nil {
		return translate(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Placement is where a user worked on a day, as a laudo names it.
type Placement struct {
	Mission      bool   `json:"mission,omitempty"` // on mission there, not posted
	UnitID       int64  `json:"unitId"`
	Acronym      string `json:"acronym"`
	Name         string `json:"name"`
	LaudoName    string `json:"laudoName"` // as the preâmbulo names the unit
	Organization string `json:"organization"`
	// Header is the laudo header's lines: the unit's, or its nearest ancestor's, or the organization's name.
	Header []string `json:"header"`
}

// UnitOn is where a user worked on a day (in São Paulo time): the mission covering it, or else the posting.
// It is nil when the user has neither.
func UnitOn(ctx context.Context, q *db.Queries, userID int64, day time.Time) (*Placement, error) {
	y, m, dd := day.Date()
	row, err := q.GetUnitOnDate(ctx, db.GetUnitOnDateParams{UserID: userID, Day: time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("unit of user %d: %w", userID, err)
	}
	p := &Placement{Mission: row.Kind == Mission, UnitID: row.UnitID, Acronym: row.Acronym, Name: row.Name, LaudoName: row.LaudoName,
		Organization: row.OrganizationName}
	if p.LaudoName == "" {
		p.LaudoName = p.Name
	}
	line, err := q.UnitAncestors(ctx, row.UnitID)
	if err != nil {
		return nil, fmt.Errorf("header of unit %d: %w", row.UnitID, err)
	}
	for _, a := range line {
		if len(a.LaudoHeader) > 0 {
			p.Header = a.LaudoHeader
			break
		}
	}
	if len(p.Header) == 0 {
		p.Header = []string{p.Organization}
	}
	return p, nil
}
