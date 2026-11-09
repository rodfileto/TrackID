// Package operations holds intelligence operations (docs/operations-plan.md): named lines of work that
// analysts follow.
package operations

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/rodfileto/trackid/db"
)

// Statuses of an operation.
const (
	StatusActive = "ACTIVE"
	StatusClosed = "CLOSED"
)

// ValidationError is an error caused by bad input. HTTP handlers map it to a 400.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

var (
	// ErrNotFound is an operation that doesn't exist.
	ErrNotFound = errors.New("operation not found")
	// ErrSlugTaken is a slug another operation already has.
	ErrSlugTaken = errors.New("an operation with this slug already exists")

	slugFormat  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nonSlugRuns = regexp.MustCompile(`[^a-z0-9]+`)
)

// Operation is an operation as the API gives it.
type Operation struct {
	ID          int64      `json:"id"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`
}

// CreateInput is a new operation. Slug comes from Name when empty.
type CreateInput struct {
	Slug        string
	Name        string
	Description string
}

// UpdateInput is what can change about an operation; its slug can't.
type UpdateInput struct {
	Name        string
	Description string
	Status      string
}

// Slugify makes a slug from a name: "Operação Ágata 2" gives "operacao-agata-2".
func Slugify(name string) string {
	folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), name)
	if err != nil {
		folded = name
	}
	return strings.Trim(nonSlugRuns.ReplaceAllString(strings.ToLower(folded), "-"), "-")
}

// Create registers an operation. createdBy is 0 when no user is behind it.
func Create(ctx context.Context, sqlDB *sql.DB, in CreateInput, createdBy int64) (Operation, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Operation{}, ValidationError("an operation needs a name")
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = Slugify(name)
	}
	if !slugFormat.MatchString(slug) {
		return Operation{}, ValidationError("a slug has only lower-case letters and numbers, separated by hyphens")
	}
	row, err := db.New(sqlDB).CreateOperation(ctx, db.CreateOperationParams{
		Slug: slug, Name: name, Description: strings.TrimSpace(in.Description),
		CreatedBy: sql.NullInt64{Int64: createdBy, Valid: createdBy != 0},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrSlugTaken
	}
	if err != nil {
		return Operation{}, err
	}
	return fromRow(row), nil
}

// List returns every operation, the active ones first.
func List(ctx context.Context, sqlDB *sql.DB) ([]Operation, error) {
	rows, err := db.New(sqlDB).ListOperations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Operation, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRow(row))
	}
	return out, nil
}

// Get returns an operation by its slug.
func Get(ctx context.Context, sqlDB *sql.DB, slug string) (Operation, error) {
	row, err := db.New(sqlDB).GetOperationBySlug(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	return fromRow(row), nil
}

// Update changes an operation's name, description and status. Closing one records when; reopening it
// clears that.
func Update(ctx context.Context, sqlDB *sql.DB, slug string, in UpdateInput) (Operation, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Operation{}, ValidationError("an operation needs a name")
	}
	if in.Status != StatusActive && in.Status != StatusClosed {
		return Operation{}, ValidationError("unknown status " + in.Status)
	}
	row, err := db.New(sqlDB).UpdateOperation(ctx, db.UpdateOperationParams{
		Slug: slug, Name: name, Description: strings.TrimSpace(in.Description), Status: in.Status,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	return fromRow(row), nil
}

func fromRow(row db.Operation) Operation {
	operation := Operation{
		ID: row.ID, Slug: row.Slug, Name: row.Name, Description: row.Description, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.ClosedAt.Valid {
		closed := row.ClosedAt.Time
		operation.ClosedAt = &closed
	}
	return operation
}
