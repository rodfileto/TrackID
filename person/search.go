package person

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rodfileto/trackid/db"
)

// searchResultLimit caps how many identity_register matches SearchByName
// returns -- a name search is a disambiguation aid for an analyst who
// already has a name in mind, not a bulk export.
const searchResultLimit = 25

// SearchResult is one identity_register whose name matched a SearchByName
// query -- a candidate for a caller to disambiguate before opening that
// person's full Profile. The same person_id can appear in more than one
// SearchResult when the search term matches several of their registers
// (e.g. an alias, or the same name spelled differently across enrollments).
type SearchResult struct {
	PersonID       string `json:"personId"`
	Name           string `json:"name"`
	RegisterNumber string `json:"registerNumber"`
	DocumentType   string `json:"documentType"`
	DocumentNumber string `json:"documentNumber"`
}

// SearchByName finds every enrolled person with at least one identity
// register whose name contains query (case-insensitive substring match,
// backed by the trigram index in
// db/migrations/021_add_person_name_search.sql). Results are capped at
// searchResultLimit; an empty (after trimming) query returns no results
// rather than the whole table.
func SearchByName(ctx context.Context, sqlDB *sql.DB, query string) ([]SearchResult, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("person: nil db")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	rows, err := db.New(sqlDB).SearchPersonsByName(ctx, db.SearchPersonsByNameParams{
		Name:  "%" + query + "%",
		Limit: searchResultLimit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, SearchResult{
			PersonID:       r.PersonID,
			Name:           r.Name,
			RegisterNumber: r.RegisterNumber,
			DocumentType:   r.DocumentType,
			DocumentNumber: r.DocumentNumber,
		})
	}
	return out, nil
}
