package geocode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ReverseMeters is how far a clicked point may be from a census address and still be given its
// address. Beyond it the click gets none rather than a wrong one. It stands until measured
// (docs/geocoding-plan.md, phase 4).
const ReverseMeters = 100

// reverseCandidates is how many of each table's nearest rows are compared: the index orders them
// on the plane, the distance on the ellipsoid decides.
const reverseCandidates = 5

// Reverse finds the census address nearest to a point within maxMeters: a number on a street, or a
// place by its quadra and lote. found is false when none is that close.
func (c *CNEFE) Reverse(ctx context.Context, lon, lat, maxMeters float64) (Result, bool, error) {
	// The point is written out in each part, not shared through a WITH: a shared one is
	// materialised, and the nearest-neighbour order then can't use the index.
	const point = `ST_SetSRID(ST_MakePoint($1::float8, $2::float8), 4326)`
	rows, err := c.db.QueryContext(ctx, `WITH near AS (
			(SELECT a.municipality_code, a.street AS name, a.number, a.locality, a.cep, a.lat, a.lon, a.geom
			 FROM cnefe_addresses a ORDER BY a.geom <-> `+point+` LIMIT $3)
			UNION ALL
			(SELECT pl.municipality_code, pl.place_key, NULL, pl.locality, pl.cep, pl.lat, pl.lon, pl.geom
			 FROM cnefe_places pl ORDER BY pl.geom <-> `+point+` LIMIT $3))
		SELECT near.municipality_code, near.name, near.number, near.locality, near.cep, near.lat, near.lon,
			COALESCE(m.name, ''), COALESCE(m.uf, ''), ST_Distance(near.geom::geography, (`+point+`)::geography) AS meters
		FROM near LEFT JOIN municipalities m ON m.code = near.municipality_code
		ORDER BY meters, length(near.name) DESC LIMIT 1`, lon, lat, reverseCandidates)
	if err != nil {
		return Result{}, false, fmt.Errorf("geocode: cnefe reverse: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return Result{}, false, rows.Err()
	}
	var code, name, locality, cep, city, uf string
	var number *int
	var pointLat, pointLon, meters float64
	if err := rows.Scan(&code, &name, &number, &locality, &cep, &pointLat, &pointLon, &city, &uf, &meters); err != nil {
		return Result{}, false, err
	}
	if meters > maxMeters {
		return Result{}, false, nil
	}
	locality = strings.TrimSpace(locality)
	result := Result{Lon: pointLon, Lat: pointLat, Precision: PrecisionGeocoded, Street: name, Neighbourhood: locality,
		City: city, Postcode: cep, CountryCode: "br", Source: cnefeSource + code}
	if uf != "" {
		result.StateCode = "BR-" + uf
	}
	line := name
	if number != nil {
		result.HouseNumber = strconv.Itoa(*number)
		line += ", " + result.HouseNumber
	} else if kinds := parseLocators(name).kinds; len(kinds) > 0 && coarseLocators[kinds[len(kinds)-1]] {
		result.Precision = PrecisionStreet // a quadra or conjunto, not a building
	}
	result.DisplayName = strings.Join(nonEmpty(line, strings.Join(nonEmpty(locality, strings.Trim(city+"/"+uf, "/")), ", ")), " - ")
	return result, true, nil
}

// at finds the municipality whose boundary covers a point.
func (m *Municipalities) at(ctx context.Context, lon, lat float64) (municipality, bool, error) {
	var found municipality
	err := m.db.QueryRowContext(ctx, `SELECT code, name, uf FROM municipalities
		WHERE ST_Covers(geom, ST_SetSRID(ST_MakePoint($1::float8, $2::float8), 4326)::geography) LIMIT 1`, lon, lat).
		Scan(&found.code, &found.name, &found.uf)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return municipality{}, false, nil
		}
		return municipality{}, false, fmt.Errorf("geocode: municipality at point: %w", err)
	}
	return found, true, nil
}
