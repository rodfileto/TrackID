package geocode

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Municipalities finds a municipality by its name in core's municipalities table (IBGE), and its
// point: the last search step, which needs no geocoding server.
type Municipalities struct {
	db *sql.DB

	mu       sync.Mutex
	byNameUF map[string]municipality   // simplified "NAME|UF"
	byName   map[string][]municipality // simplified name, for an address that names no state
	brasilia municipality              // the Distrito Federal's one municipality
	loadedAt time.Time
}

type municipality struct{ code, name, uf string }

// NewMunicipalities reads the table when first asked.
func NewMunicipalities(sqlDB *sql.DB) *Municipalities {
	return &Municipalities{db: sqlDB}
}

// find resolves a name and, when given, a state. With no state, a name only one municipality has is
// enough. In the Distrito Federal, addresses name the region (Ceilândia/DF), not Brasília, so any
// name there is Brasília.
func (m *Municipalities) find(ctx context.Context, city, uf string) (municipality, bool, error) {
	if err := m.load(ctx); err != nil {
		return municipality{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	name := simplify(city)
	if uf != "" {
		if found, ok := m.byNameUF[name+"|"+uf]; ok {
			return found, true, nil
		}
		if uf == "DF" && m.brasilia.code != "" {
			return m.brasilia, true, nil
		}
		return municipality{}, false, nil
	}
	if candidates := m.byName[name]; len(candidates) == 1 {
		return candidates[0], true, nil
	}
	return municipality{}, false, nil
}

// load reads the table once it holds rows; while it's empty it looks again at most once a minute,
// since municipalities may be loaded while the server runs.
func (m *Municipalities) load(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.byNameUF) > 0 || time.Since(m.loadedAt) < time.Minute {
		return nil
	}
	m.loadedAt = time.Now()
	rows, err := m.db.QueryContext(ctx, `SELECT code, name, uf FROM municipalities`)
	if err != nil {
		return fmt.Errorf("geocode: municipalities: %w", err)
	}
	defer rows.Close()
	byNameUF, byName := map[string]municipality{}, map[string][]municipality{}
	for rows.Next() {
		var found municipality
		if err := rows.Scan(&found.code, &found.name, &found.uf); err != nil {
			return err
		}
		name := simplify(found.name)
		byNameUF[name+"|"+found.uf] = found
		byName[name] = append(byName[name], found)
		if found.uf == "DF" {
			m.brasilia = found
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	m.byNameUF, m.byName = byNameUF, byName
	return nil
}

// point is a place inside the municipality, near its middle: its centroid when that falls inside,
// as for most, and otherwise a point on its surface.
func (m *Municipalities) point(ctx context.Context, found municipality) (Result, error) {
	var lat, lon float64
	err := m.db.QueryRowContext(ctx, `SELECT ST_Y(p), ST_X(p) FROM (
		SELECT CASE WHEN ST_Contains(g, ST_Centroid(g)) THEN ST_Centroid(g) ELSE ST_PointOnSurface(g) END AS p
		FROM (SELECT geom::geometry AS g FROM municipalities WHERE code = $1) AS m) AS c`, found.code).Scan(&lat, &lon)
	if err != nil {
		return Result{}, fmt.Errorf("geocode: municipality point: %w", err)
	}
	return Result{Lon: lon, Lat: lat, Precision: PrecisionMunicipality, DisplayName: found.name + "/" + found.uf,
		City: found.name, StateCode: "BR-" + found.uf, CountryCode: "br", Source: "ibge:" + found.code}, nil
}
