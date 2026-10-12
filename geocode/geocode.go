// Package geocode turns addresses into points and points into addresses, for
// locations (see MODEL.md section 2.3): the future address and coordinate
// picker, and importers that only have an address.
//
// Geocoder is the boundary; Nominatim (OpenStreetMap) is its implementation.
// Results are OpenStreetMap data, under the ODbL: whatever displays them must
// credit "© OpenStreetMap contributors", and a database built from them is a
// derived database under the same licence.
package geocode

import (
	"context"
	"errors"
)

// Precision values, the same as locations.precision. A result coarser than a
// municipality (a state, a country) gets none and is not returned.
const (
	PrecisionGeocoded      = "GEOCODED"      // a building or a house number
	PrecisionStreet        = "STREET"        // a street, not a number on it
	PrecisionNeighbourhood = "NEIGHBOURHOOD" // a neighbourhood or district
	PrecisionMunicipality  = "MUNICIPALITY"  // only the municipality
)

// ErrNotFound is returned when a query has no result at municipality
// precision or better.
var ErrNotFound = errors.New("geocode: no result")

// Query is an address to geocode: either free text (Text) or structured
// fields, not both. Structured fields left empty are not sent.
type Query struct {
	Text string

	Street   string // house number and street, e.g. "Rua Visconde de Pirajá 459"
	City     string
	State    string // name or UF
	Postcode string
	Country  string // defaults to "Brasil" for structured queries

	Limit int // results wanted; defaults to 1
}

// Result is one geocoded place.
type Result struct {
	Lon, Lat  float64
	Precision string // one of the Precision constants

	DisplayName   string
	HouseNumber   string
	Street        string
	Neighbourhood string
	City          string
	State         string
	StateCode     string // ISO 3166-2, e.g. "BR-RJ"
	Postcode      string
	CountryCode   string // ISO 3166-1 alpha-2, lower case

	// Source identifies the provider's object, e.g. "osm:way/123456", for
	// tracing a stored point back to where it came from.
	Source string
}

// Geocoder resolves addresses to points (Search) and points to addresses
// (Reverse).
type Geocoder interface {
	Search(ctx context.Context, q Query) ([]Result, error)
	Reverse(ctx context.Context, lon, lat float64) (Result, error)
}
