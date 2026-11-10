package geocode

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/text/encoding/charmap"
)

// cnefeWorstLevel is the coarsest NV_GEO_COORD kept: coarser levels place a block face, a locality
// or a census sector, not the address.
const cnefeWorstLevel = 3

type cnefeKey struct {
	municipality, street, locality string
	number                         int32
}

type cnefePlaceKey struct {
	municipality, locality, place string
}

type cnefeEntry struct {
	lat, lon      float64
	count         int32
	level         int16
	street        string // as IBGE writes it (numbers only)
	locality, cep string
}

// add takes in one address: coordinates are averaged among those at the best level.
func (e *cnefeEntry) add(lat, lon float64, level int16) {
	switch {
	case level < e.level:
		e.lat, e.lon, e.count, e.level = lat, lon, 1, level
	case level == e.level:
		e.count++
		e.lat += (lat - e.lat) / float64(e.count)
		e.lon += (lon - e.lon) / float64(e.count)
	}
}

// CNEFELoadStats counts a load, which is public data. Located rows have no number but locators.
type CNEFELoadStats struct {
	Rows, Kept, Located, NoNumber, Coarse, BadRow int
	Entries, Places                               int
}

// CNEFEData is a state's file reduced: numbers on streets, and places found by their locators.
type CNEFEData struct {
	numbers map[cnefeKey]*cnefeEntry
	places  map[cnefePlaceKey]*cnefeEntry
}

// ReadCNEFE reduces a CNEFE CSV, as IBGE publishes it per state, to one entry per number on a
// street, and one per place an address with no number gives by its locators. Coordinates at the
// same number are averaged, among those at its best level: apartments in one building differ only
// slightly. It takes a few gigabytes of memory for a large state.
func ReadCNEFE(r io.Reader) (CNEFEData, CNEFELoadStats, error) {
	reader := csv.NewReader(r)
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.ReuseRecord = true
	header, err := reader.Read()
	if err != nil {
		return CNEFEData{}, CNEFELoadStats{}, fmt.Errorf("read header: %w", err)
	}
	column := map[string]int{}
	for i, name := range header {
		column[strings.TrimPrefix(name, "\ufeff")] = i
	}
	for _, name := range []string{"COD_MUNICIPIO", "NOM_TIPO_SEGLOGR", "NOM_TITULO_SEGLOGR", "NOM_SEGLOGR", "NUM_ENDERECO", "LATITUDE", "LONGITUDE", "NV_GEO_COORD", "DSC_LOCALIDADE", "CEP"} {
		if _, ok := column[name]; !ok {
			return CNEFEData{}, CNEFELoadStats{}, fmt.Errorf("the file has no column %s", name)
		}
	}
	// The complements, NOM_COMP_ELEM1 to 5 with their values: those that are locators place an
	// address with no number.
	var complements [][2]int
	for i := 1; i <= 5; i++ {
		name, nameOK := column[fmt.Sprintf("NOM_COMP_ELEM%d", i)]
		value, valueOK := column[fmt.Sprintf("VAL_COMP_ELEM%d", i)]
		if nameOK && valueOK {
			complements = append(complements, [2]int{name, value})
		}
	}
	latin1 := charmap.ISO8859_1.NewDecoder()
	text := func(value string) string {
		if !utf8.ValidString(value) {
			if decoded, err := latin1.String(value); err == nil {
				return decoded
			}
		}
		return value
	}
	type streetNames struct{ key, display string }
	var stats CNEFELoadStats
	entries := map[cnefeKey]*cnefeEntry{}
	places := map[cnefePlaceKey]*cnefeEntry{}
	streets := map[[3]string]streetNames{} // the same few names, millions of times
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			stats.BadRow++
			continue
		}
		stats.Rows++
		number, err := strconv.Atoi(row[column["NUM_ENDERECO"]])
		var located locatorKey
		if err != nil || number <= 0 {
			located = cnefeLocators(row, column, complements, text)
			if len(located.ends) == 0 {
				stats.NoNumber++
				continue
			}
		}
		level, err := strconv.Atoi(row[column["NV_GEO_COORD"]])
		if err != nil || level > cnefeWorstLevel {
			stats.Coarse++
			continue
		}
		lat, latErr := strconv.ParseFloat(strings.Replace(row[column["LATITUDE"]], ",", ".", 1), 64)
		lon, lonErr := strconv.ParseFloat(strings.Replace(row[column["LONGITUDE"]], ",", ".", 1), 64)
		if latErr != nil || lonErr != nil {
			stats.BadRow++
			continue
		}
		if len(located.ends) > 0 {
			stats.Located++
			locality := strings.TrimSpace(text(row[column["DSC_LOCALIDADE"]]))
			for _, place := range located.filedKeys() {
				key := cnefePlaceKey{strings.Clone(row[column["COD_MUNICIPIO"]]), simplify(locality), place}
				if entry := places[key]; entry != nil {
					entry.add(lat, lon, int16(level))
				} else {
					places[key] = &cnefeEntry{lat: lat, lon: lon, count: 1, level: int16(level), locality: strings.Clone(locality), cep: strings.Clone(row[column["CEP"]])}
				}
			}
			continue
		}
		names := [3]string{row[column["NOM_TIPO_SEGLOGR"]], row[column["NOM_TITULO_SEGLOGR"]], row[column["NOM_SEGLOGR"]]}
		street, seen := streets[names]
		if !seen {
			// The Distrito Federal's type EDF says only that the street field is a sector.
			kind := text(names[0])
			if kind == "EDF" {
				kind = ""
			}
			parts := []string{kind, text(names[1]), text(names[2])}
			street = streetNames{key: StreetKey(parts...), display: strings.Join(strings.Fields(strings.Join(parts, " ")), " ")}
			streets[[3]string{strings.Clone(names[0]), strings.Clone(names[1]), strings.Clone(names[2])}] = street
		}
		if street.key == "" {
			stats.BadRow++
			continue
		}
		stats.Kept++
		locality := strings.TrimSpace(text(row[column["DSC_LOCALIDADE"]]))
		key := cnefeKey{strings.Clone(row[column["COD_MUNICIPIO"]]), street.key, simplify(locality), int32(number)}
		if entry := entries[key]; entry != nil {
			entry.add(lat, lon, int16(level))
		} else {
			entries[key] = &cnefeEntry{lat: lat, lon: lon, count: 1, level: int16(level), street: street.display,
				locality: strings.Clone(locality), cep: strings.Clone(row[column["CEP"]])}
		}
	}
	stats.Entries, stats.Places = len(entries), len(places)
	return CNEFEData{numbers: entries, places: places}, stats, nil
}

// cnefeLocators reduces a row's street field and its locator complements, leaving its units out.
// The Distrito Federal's street type, EDF, says only that the field is a sector: SQN 103 BLOCO B.
func cnefeLocators(row []string, column map[string]int, complements [][2]int, text func(string) string) locatorKey {
	var parts []string
	if kind := row[column["NOM_TIPO_SEGLOGR"]]; kind != "EDF" {
		parts = append(parts, text(kind))
	}
	parts = append(parts, text(row[column["NOM_TITULO_SEGLOGR"]]), text(row[column["NOM_SEGLOGR"]]))
	for _, complement := range complements {
		if name := row[complement[0]]; locatorWords[name] != "" {
			parts = append(parts, name, text(row[complement[1]]))
		}
	}
	return parseLocators(strings.Join(parts, " "))
}

// LoadCNEFE replaces the rows of the municipalities in data, leaving other states' as they are, in
// one transaction. sqlDB must use the pgx driver (internal/database), for its bulk copy.
func LoadCNEFE(ctx context.Context, sqlDB *sql.DB, data CNEFEData) error {
	municipalities := map[string]bool{}
	rows := make([][]any, 0, len(data.numbers))
	for key, entry := range data.numbers {
		municipalities[key.municipality] = true
		rows = append(rows, []any{key.municipality, key.street, key.locality, key.number, entry.street, entry.lat, entry.lon, entry.level, entry.count, entry.locality, entry.cep})
	}
	placeRows := make([][]any, 0, len(data.places))
	for key, entry := range data.places {
		municipalities[key.municipality] = true
		placeRows = append(placeRows, []any{key.municipality, key.place, key.locality, entry.lat, entry.lon, entry.level, entry.count, entry.locality, entry.cep})
	}
	codes := make([]string, 0, len(municipalities))
	for code := range municipalities {
		codes = append(codes, code)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) error {
		stdlibConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return errors.New("geocode: LoadCNEFE needs the pgx driver")
		}
		tx, err := stdlibConn.Conn().Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		for _, table := range []string{"cnefe_addresses", "cnefe_places"} {
			if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE municipality_code = ANY($1)`, codes); err != nil {
				return err
			}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"cnefe_addresses"},
			[]string{"municipality_code", "street_key", "locality_key", "number", "street", "lat", "lon", "level", "addresses", "locality", "cep"},
			pgx.CopyFromRows(rows)); err != nil {
			return err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"cnefe_places"},
			[]string{"municipality_code", "place_key", "locality_key", "lat", "lon", "level", "addresses", "locality", "cep"},
			pgx.CopyFromRows(placeRows)); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}
