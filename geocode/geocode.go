// Package geocode turns addresses into points and points into addresses, for
// locations (see MODEL.md section 2.3): the future address and coordinate
// picker, and importers that only have an address.
//
// Geocoder is the boundary. Layered is what the API uses: IBGE's census
// address list (CNEFE) in TrackID's database, then an optional Nominatim, then
// the municipality (docs/geocoding-plan.md). A result's Source says where it
// came from. CNEFE results credit "Fonte: IBGE, Censo Demográfico 2022".
// Nominatim results are OpenStreetMap data, under the ODbL: whatever displays
// them must credit "© OpenStreetMap contributors", and a database built from
// them is a derived database under the same licence.
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

	// Near is where the user is looking (the map's centre): an address that names no municipality
	// is looked up first in the one there. Nil for none; Nominatim ignores it.
	Near *Point
}

// Point is a longitude and latitude.
type Point struct{ Lon, Lat float64 }

// Result is one geocoded place.
type Result struct {
	// JSON names differ from the field names only in case, so results encoded before the tags
	// existed (e.g. a client's cache) still decode.
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
	Precision string  `json:"precision"` // one of the Precision constants

	DisplayName   string `json:"displayName"`
	HouseNumber   string `json:"houseNumber,omitempty"`
	Street        string `json:"street,omitempty"`
	Neighbourhood string `json:"neighbourhood,omitempty"`
	City          string `json:"city,omitempty"`
	State         string `json:"state,omitempty"`
	StateCode     string `json:"stateCode,omitempty"` // ISO 3166-2, e.g. "BR-RJ"
	Postcode      string `json:"postcode,omitempty"`
	CountryCode   string `json:"countryCode,omitempty"` // ISO 3166-1 alpha-2, lower case

	// Source identifies the provider's object, e.g. "osm:way/123456", for
	// tracing a stored point back to where it came from.
	Source string `json:"source"`

	// Flags are reasons to check the point before trusting it (the Flag constants); empty when it
	// is what the address asked for.
	Flags []string `json:"flags,omitempty"`
}

// Flags on a Result: it may be right, but something about it is uncertain.
const (
	FlagNearestNumber    = "NEAREST_NUMBER"    // the number wasn't found; this is the nearest on the street
	FlagStreetAdjusted   = "STREET_ADJUSTED"   // the street's spelling differs from the address's
	FlagLocatorPartial   = "LOCATOR_PARTIAL"   // only part of the quadra, lote, casa was found: the point is that part
	FlagStreetOnly       = "STREET_ONLY"       // the address has a number, the point only the street
	FlagMunicipalityOnly = "MUNICIPALITY_ONLY" // only the municipality was found: the point is near its middle
	FlagNamedPlace       = "NAMED_PLACE"       // found by name, with no municipality to confirm it
	FlagSourcesDisagree  = "SOURCES_DISAGREE"  // the census list and the map are far apart
)

// Flags lists every Flag constant, for checking flags stored with a location.
var Flags = []string{FlagNearestNumber, FlagStreetAdjusted, FlagLocatorPartial, FlagStreetOnly, FlagMunicipalityOnly,
	FlagNamedPlace, FlagSourcesDisagree}

// Geocoder resolves addresses to points (Search) and points to addresses
// (Reverse).
type Geocoder interface {
	Search(ctx context.Context, q Query) ([]Result, error)
	Reverse(ctx context.Context, lon, lat float64) (Result, error)
}
