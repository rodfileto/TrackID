package geocode

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// The steps a search goes through, from the most exact.
const (
	stepCNEFENumber    = "cnefe"            // the address's number on the street, from the census list
	stepCNEFENear      = "cnefe_near"       // the nearest number on the street
	stepCNEFEPlace     = "cnefe_place"      // the quadra, lote or casa
	stepCNEFEPlacePart = "cnefe_place_part" // part of it
	stepNumber         = "number"           // the map's server, number and street
	stepStreet         = "street"           // the map's server, the street
	stepText           = "text"             // the map's server, the text as written
	stepMunicipality   = "municipality"     // the municipality, from core's own table
)

// disagreeMeters is how far apart the census list and the map can put an address before the
// result says so.
const disagreeMeters = 2000

// Layered geocodes through every source configured, most exact first (docs/geocoding-plan.md): the
// census address list (CNEFE) in TrackID's database, then a Nominatim server, then the
// municipality. The first result that agrees with the municipality and state the address names wins,
// and it carries Flags saying how sure it is. Any source may be missing: with none but the
// municipalities, an address still finds its municipality.
type Layered struct {
	CNEFE          *CNEFE          // nil: no census list
	Nominatim      Geocoder        // nil: no map server
	Municipalities *Municipalities // nil: no municipality step
}

// NewLayered builds a Layered over the database's tables and nominatim, either of which may be nil.
func NewLayered(sqlDB *sql.DB, nominatim Geocoder) *Layered {
	layered := &Layered{Nominatim: nominatim}
	if sqlDB != nil {
		layered.CNEFE, layered.Municipalities = NewCNEFE(sqlDB), NewMunicipalities(sqlDB)
	}
	return layered
}

// Search geocodes free text (q.Text) or structured fields. It returns the first step's results that
// agree with the address, up to q.Limit, or ErrNotFound.
func (l *Layered) Search(ctx context.Context, q Query) ([]Result, error) {
	text := q.Text
	if text == "" {
		text = strings.Join(nonEmpty(q.Street, q.City), ", ")
		if q.State != "" {
			text += "/" + q.State
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 1
	}
	parsed := ParseAddress(text)

	var place municipality
	var placeKnown, inferred bool
	if l.Municipalities != nil && parsed.City != "" {
		var err error
		if place, placeKnown, err = l.Municipalities.find(ctx, parsed.City, parsed.UF); err != nil {
			return nil, err
		}
	}
	// No municipality named: the one the user is looking at, for the census list and the map's
	// number and street. It is a guess, so the map's free-text search still runs after, and the
	// municipality alone is never the answer.
	biased := parsed
	if l.Municipalities != nil && parsed.City == "" && q.Near != nil {
		var err error
		if place, inferred, err = l.Municipalities.at(ctx, q.Near.Lon, q.Near.Lat); err != nil {
			return nil, err
		}
		if inferred {
			placeKnown = true
			biased.City, biased.UF = place.name, place.uf
		}
	}

	if l.CNEFE != nil && placeKnown {
		loaded, err := l.CNEFE.Loaded(ctx)
		if err != nil {
			return nil, err
		}
		if loaded {
			result, _, found, err := l.CNEFE.find(ctx, place, biased, text)
			if err != nil {
				return nil, err
			}
			if found {
				l.crossCheck(ctx, text, biased, &result)
				return []Result{result}, nil
			}
		}
	}

	if l.Nominatim != nil {
		passes := []ParsedAddress{parsed}
		if inferred && parsed.Street != "" {
			passes = []ParsedAddress{biased, parsed}
		}
		for _, pass := range passes {
			results, step, err := l.searchNominatim(ctx, text, pass, limit)
			if err != nil {
				return nil, err
			}
			if len(results) > 0 {
				for i := range results {
					results[i].Flags = append(results[i].Flags, stepFlags(step, pass, results[i])...)
				}
				return results, nil
			}
		}
	}

	if placeKnown && !inferred {
		result, err := l.Municipalities.point(ctx, place)
		if err != nil {
			return nil, err
		}
		result.Flags = []string{FlagMunicipalityOnly}
		return []Result{result}, nil
	}
	return nil, ErrNotFound
}

// Reverse gives the census address nearest to the point, within ReverseMeters, and otherwise asks
// the map server, when there is one. Further than that from any census address, and with no map
// server, a point has no address: better none than a neighbour's from a block away.
func (l *Layered) Reverse(ctx context.Context, lon, lat float64) (Result, error) {
	if l.CNEFE != nil {
		loaded, err := l.CNEFE.Loaded(ctx)
		if err != nil {
			return Result{}, err
		}
		if loaded {
			result, found, err := l.CNEFE.Reverse(ctx, lon, lat, ReverseMeters)
			if err != nil {
				return Result{}, err
			}
			if found {
				return result, nil
			}
		}
	}
	if l.Nominatim == nil {
		return Result{}, ErrNotFound
	}
	return l.Nominatim.Reverse(ctx, lon, lat)
}

// searchNominatim runs the map server's steps: number and street, the street, and the text as written
// when no street could be read from it. It returns the first step's agreeing results.
func (l *Layered) searchNominatim(ctx context.Context, text string, parsed ParsedAddress, limit int) ([]Result, string, error) {
	type step struct {
		name  string
		query Query
	}
	const candidates = 5
	var steps []step
	structured := func(street string) Query {
		return Query{Street: street, City: parsed.City, State: parsed.UF, Limit: candidates}
	}
	if parsed.Street != "" && parsed.Number != "" && parsed.City != "" {
		steps = append(steps, step{stepNumber, structured(parsed.Number + " " + parsed.Street)})
	}
	if parsed.Street != "" && parsed.City != "" {
		steps = append(steps, step{stepStreet, structured(parsed.Street)})
	}
	if parsed.Street == "" || parsed.City == "" {
		steps = append(steps, step{stepText, Query{Text: text, Limit: candidates}})
	}
	for _, s := range steps {
		results, err := l.Nominatim.Search(ctx, s.query)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		var agreeing []Result
		for _, result := range results {
			// A municipality-only answer is left to the municipality step, from core's own table.
			if agrees(result, parsed) && result.Precision != PrecisionMunicipality {
				agreeing = append(agreeing, result)
				if len(agreeing) == limit {
					break
				}
			}
		}
		if len(agreeing) > 0 {
			return agreeing, s.name, nil
		}
	}
	return nil, "", nil
}

// crossCheck asks the map server for an address the census list found: two sources far apart mean
// one has the wrong place. A server that's down, or that knows only the street, just goes without.
func (l *Layered) crossCheck(ctx context.Context, text string, parsed ParsedAddress, result *Result) {
	if l.Nominatim == nil || result.Precision != PrecisionGeocoded {
		return
	}
	others, _, err := l.searchNominatim(ctx, text, parsed, 1)
	if err != nil || len(others) == 0 || others[0].Precision == PrecisionMunicipality {
		return
	}
	if distanceMeters(result.Lat, result.Lon, others[0].Lat, others[0].Lon) > disagreeMeters {
		result.Flags = append(result.Flags, FlagSourcesDisagree)
	}
}

// stepFlags says what to check about a map server's result, from how it was found.
func stepFlags(step string, parsed ParsedAddress, result Result) []string {
	switch step {
	case stepNumber, stepStreet:
		if parsed.Number != "" && result.Precision != PrecisionGeocoded {
			return []string{FlagStreetOnly}
		}
	case stepText:
		if parsed.City == "" {
			return []string{FlagNamedPlace}
		}
	}
	return nil
}

// agrees checks a result against what the address says: a geocoder happily returns a same-named
// street in another municipality, which would put a false point on the map. An address naming no
// municipality gives nothing to compare, so only its state can disagree, when known.
func agrees(result Result, parsed ParsedAddress) bool {
	if parsed.UF != "" && result.StateCode != "" && result.StateCode != "BR-"+parsed.UF {
		return false
	}
	if parsed.City != "" && simplify(result.City) != simplify(parsed.City) {
		// The Distrito Federal's addresses name the region, and the map names Brasília.
		if !(parsed.UF == "DF" && simplify(result.City) == "BRASILIA") {
			return false
		}
	}
	return true
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
