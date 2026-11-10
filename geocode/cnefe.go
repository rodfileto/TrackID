package geocode

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cnefeNearNumbers = 50   // the nearest number on the street still says where on it
	cnefeSameStreet  = 0.88 // how alike two spellings of a street must be
	samePlaceMeters  = 500  // a place CNEFE files under localities this close is the same place
	cnefeSource      = "cnefe2022:"
)

// CNEFE looks addresses up in the census address list loaded into TrackID's database
// (cmd/load-cnefe): by number on a street, or by quadra, lote or casa.
type CNEFE struct {
	db *sql.DB

	mu        sync.Mutex
	streets   map[string][]string // a municipality's streets, for spellings that differ
	loaded    bool
	checkedAt time.Time
}

// NewCNEFE returns a lookup over the cnefe tables, empty or not.
func NewCNEFE(sqlDB *sql.DB) *CNEFE {
	return &CNEFE{db: sqlDB, streets: map[string][]string{}}
}

// Loaded reports whether any state is loaded, looking again at most once a minute while none is,
// so a state loaded while the server runs is found.
func (c *CNEFE) Loaded(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded || time.Since(c.checkedAt) < time.Minute {
		return c.loaded, nil
	}
	c.checkedAt = time.Now()
	err := c.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cnefe_addresses) OR EXISTS (SELECT 1 FROM cnefe_places)`).Scan(&c.loaded)
	if err != nil {
		return false, fmt.Errorf("geocode: cnefe: %w", err)
	}
	return c.loaded, nil
}

var leadingNumber = regexp.MustCompile(`^\d+`)

// find looks an address up in a municipality: its exact number, or the nearest on the street, which
// still says where on it; with no number, by its quadra, lote or casa.
func (c *CNEFE) find(ctx context.Context, place municipality, parsed ParsedAddress, text string) (Result, string, bool, error) {
	if result, step, found, err := c.findNumber(ctx, place, parsed, text); found || err != nil {
		return result, step, found, err
	}
	if parsed.Place != "" {
		return c.findPlace(ctx, place, parsed)
	}
	return Result{}, "", false, nil
}

// findNumber finds the number, or the nearest on the street. text, the address as written, says
// which locality is meant when the street's name is in several.
func (c *CNEFE) findNumber(ctx context.Context, place municipality, parsed ParsedAddress, text string) (Result, string, bool, error) {
	digits := leadingNumber.FindString(parsed.Number)
	if parsed.Street == "" || digits == "" {
		return Result{}, "", false, nil
	}
	number, err := strconv.Atoi(digits)
	if err != nil {
		return Result{}, "", false, nil
	}
	street, err := c.street(ctx, place.code, StreetKey(parsed.Street))
	if err != nil || street == "" {
		return Result{}, "", false, err
	}
	// The nearest number first; at it, one row per locality the street name has there.
	rows, err := c.db.QueryContext(ctx, `SELECT number, lat, lon, street, locality, locality_key, cep, addresses FROM cnefe_addresses
		WHERE municipality_code = $1 AND street_key = $2 AND number BETWEEN $3::int - $4::int AND $3::int + $4::int
		ORDER BY abs(number - $3::int), number`, place.code, street, number, cnefeNearNumbers)
	if err != nil {
		return Result{}, "", false, fmt.Errorf("geocode: cnefe: %w", err)
	}
	defer rows.Close()
	found := -1
	var atNumber []placeRow
	for rows.Next() {
		var n int
		var row placeRow
		if err := rows.Scan(&n, &row.lat, &row.lon, &row.street, &row.locality, &row.localityKey, &row.cep, &row.addresses); err != nil {
			return Result{}, "", false, err
		}
		if found >= 0 && n != found {
			break
		}
		found = n
		atNumber = append(atNumber, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, "", false, err
	}
	// The same street name in two localities: the one the address names, or none.
	row, ok := pickLocality(atNumber, " "+simplify(text)+" ")
	if !ok {
		return Result{}, "", false, nil
	}
	locality := strings.TrimSpace(row.locality)
	result := Result{Lon: row.lon, Lat: row.lat, Precision: PrecisionStreet,
		DisplayName: fmt.Sprintf("%s, %d - %s, %s/%s", row.street, found, locality, place.name, place.uf),
		Street:      row.street, Neighbourhood: locality, City: place.name, StateCode: "BR-" + place.uf, Postcode: row.cep,
		CountryCode: "br", Source: cnefeSource + place.code}
	if street != StreetKey(parsed.Street) {
		result.Flags = append(result.Flags, FlagStreetAdjusted)
	}
	if found != number {
		result.Flags = append(result.Flags, FlagNearestNumber)
		return result, stepCNEFENear, true, nil
	}
	result.Precision, result.HouseNumber = PrecisionGeocoded, strconv.Itoa(found)
	return result, stepCNEFENumber, true, nil
}

// street finds the table's spelling of a street in a municipality: the same, or the only one close
// enough, of the same type (a Rua is not an Avenida of the same name).
func (c *CNEFE) street(ctx context.Context, code, wanted string) (string, error) {
	var exists bool
	if err := c.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cnefe_addresses WHERE municipality_code = $1 AND street_key = $2)`, code, wanted).Scan(&exists); err != nil {
		return "", fmt.Errorf("geocode: cnefe: %w", err)
	}
	if exists {
		return wanted, nil
	}
	streets, err := c.streetsOf(ctx, code)
	if err != nil {
		return "", err
	}
	kind, _, _ := strings.Cut(wanted, " ")
	best, second := 0.0, 0.0
	match := ""
	for _, street := range streets {
		if first, _, _ := strings.Cut(street, " "); first != kind {
			continue
		}
		score := similarity(wanted, street)
		switch {
		case score > best:
			best, second, match = score, best, street
		case score > second:
			second = score
		}
	}
	if best < cnefeSameStreet || best == second {
		return "", nil
	}
	return match, nil
}

func (c *CNEFE) streetsOf(ctx context.Context, code string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if streets, ok := c.streets[code]; ok {
		return streets, nil
	}
	rows, err := c.db.QueryContext(ctx, `SELECT DISTINCT street_key FROM cnefe_addresses WHERE municipality_code = $1`, code)
	if err != nil {
		return nil, fmt.Errorf("geocode: cnefe: %w", err)
	}
	defer rows.Close()
	var streets []string
	for rows.Next() {
		var street string
		if err := rows.Scan(&street); err != nil {
			return nil, err
		}
		streets = append(streets, street)
	}
	c.streets[code] = streets
	return streets, rows.Err()
}

type placeRow struct {
	street                     string // numbers only
	locality, localityKey, cep string
	lat, lon                   float64
	addresses                  int
}

// findPlace looks up an address by its locators.
func (c *CNEFE) findPlace(ctx context.Context, place municipality, parsed ParsedAddress) (Result, string, bool, error) {
	key := parseLocators(parsed.Place)
	candidates := key.candidates()
	if len(candidates) == 0 {
		return Result{}, "", false, nil
	}
	keys := make([]string, len(candidates))
	for i, candidate := range candidates {
		keys[i] = candidate.key
	}
	rows, err := c.db.QueryContext(ctx, `SELECT place_key, locality, locality_key, cep, lat, lon, addresses FROM cnefe_places
		WHERE municipality_code = $1 AND place_key = ANY($2)`, place.code, keys)
	if err != nil {
		return Result{}, "", false, fmt.Errorf("geocode: cnefe places: %w", err)
	}
	defer rows.Close()
	found := map[string][]placeRow{}
	for rows.Next() {
		var key string
		var row placeRow
		if err := rows.Scan(&key, &row.locality, &row.localityKey, &row.cep, &row.lat, &row.lon, &row.addresses); err != nil {
			return Result{}, "", false, err
		}
		found[key] = append(found[key], row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, "", false, err
	}
	hint := " " + key.String() + " " + simplify(parsed.City) + " "
	for _, candidate := range candidates {
		row, ok := pickLocality(found[candidate.key], hint)
		if !ok {
			continue
		}
		step, precision := stepCNEFEPlace, PrecisionGeocoded
		result := Result{Lon: row.lon, Lat: row.lat,
			DisplayName: fmt.Sprintf("%s - %s, %s/%s", candidate.key, strings.TrimSpace(row.locality), place.name, place.uf),
			Street:      candidate.key, Neighbourhood: strings.TrimSpace(row.locality), City: place.name, StateCode: "BR-" + place.uf,
			Postcode: row.cep, CountryCode: "br", Source: cnefeSource + place.code}
		if candidate.locators < len(key.ends) {
			step = stepCNEFEPlacePart
			result.Flags = append(result.Flags, FlagLocatorPartial)
		}
		if coarseLocators[candidate.last] {
			precision = PrecisionStreet
		}
		result.Precision = precision
		return result, step, true, nil
	}
	return Result{}, "", false, nil
}

// pickLocality chooses among the localities a key is filed under: the only one, the one the address
// names (the longest, so SOBRADINHO II isn't SOBRADINHO), or, when the address names none, all of
// them if they are one place filed under spellings of its locality. The same quadra in two towns is
// neither.
func pickLocality(rows []placeRow, hint string) (placeRow, bool) {
	switch len(rows) {
	case 0:
		return placeRow{}, false
	case 1:
		return rows[0], true
	}
	best, tie := -1, false
	for i, row := range rows {
		if row.localityKey == "" || !strings.Contains(hint, " "+row.localityKey+" ") {
			continue
		}
		switch {
		case best < 0 || len(row.localityKey) > len(rows[best].localityKey):
			best, tie = i, false
		case len(row.localityKey) == len(rows[best].localityKey):
			tie = true
		}
	}
	if best >= 0 && !tie {
		return rows[best], true
	}
	var merged placeRow
	for _, row := range rows {
		merged.lat += row.lat * float64(row.addresses)
		merged.lon += row.lon * float64(row.addresses)
		merged.addresses += row.addresses
	}
	merged.lat /= float64(merged.addresses)
	merged.lon /= float64(merged.addresses)
	for _, row := range rows {
		if distanceMeters(row.lat, row.lon, merged.lat, merged.lon) > samePlaceMeters {
			return placeRow{}, false
		}
	}
	merged.street, merged.locality, merged.cep = rows[0].street, rows[0].locality, rows[0].cep
	return merged, true
}
