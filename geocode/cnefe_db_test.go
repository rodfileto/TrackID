package geocode

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/db/migrations"
)

// testDB is a throwaway, migrated database on the server named by TRACKID_TEST_DATABASE_URL, opened
// with pgx as LoadCNEFE needs; skipped when unset.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	adminURL := os.Getenv("TRACKID_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TRACKID_TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("pgx", adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("trackid_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`) })
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	sqlDB, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

// A made-up CNEFE file in IBGE's layout: real municipality codes, invented addresses.
const cnefeSample = "COD_MUNICIPIO;DSC_LOCALIDADE;NOM_TIPO_SEGLOGR;NOM_TITULO_SEGLOGR;NOM_SEGLOGR;NUM_ENDERECO;NOM_COMP_ELEM1;VAL_COMP_ELEM1;NOM_COMP_ELEM2;VAL_COMP_ELEM2;NOM_COMP_ELEM3;VAL_COMP_ELEM3;LATITUDE;LONGITUDE;NV_GEO_COORD;CEP\n" +
	"5300108;SAMAMBAIA;EDF;;QR 308 CONJUNTO 13;0;LOTE;8;;;;;-15.8772;-48.0745;1;72306613\n" +
	"5300108;SAMAMBAIA;EDF;;QR 308 CONJUNTO 13;0;LOTE;8;APARTAMENTO;101;;;-15.8774;-48.0747;1;72306613\n" +
	"5300108;SAMAMBAIA;EDF;;QR 308 CONJUNTO 13;0;LOTE;10;;;;;-15.8780;-48.0750;1;72306613\n" +
	"5300108;ASA NORTE;EDF;;SQN 103 BLOCO B;0;APARTAMENTO;401;;;;;-15.7792;-47.8838;1;70732020\n" +
	"5300108;PARANOA;EDF;;QUADRA 3 CONJUNTO B;0;LOTE;1;;;;;-15.77;-47.77;1;71000000\n" +
	"5300108;VARJAO;EDF;;QUADRA 3 CONJUNTO B;0;LOTE;1;;;;;-15.71;-47.87;1;71000000\n" +
	"5300108;SOBRADINHO;EDF;;QUADRA 13 CONJUNTO F;0;LOTE;60;;;;;-15.6429;-47.7838;1;73040136\n" +
	"5300108;SOBRADINHO I;EDF;;QUADRA 13 CONJUNTO F;0;LOTE;60;;;;;-15.6431;-47.7840;1;73040136\n" +
	"5300108;ASA SUL;EDF;;SQS 207 BLOCO I;0;APARTAMENTO;301;;;;;-15.8184;-47.8992;5;70253090\n" +
	"5208707;JARDIM NOVO MUNDO;AVENIDA;;URUGUAIANA;0;QUADRA;22;LOTE;4;CASA;1;-16.68;-49.22;1;74000000\n" +
	"5208707;JARDIM NOVO MUNDO;AVENIDA;;URUGUAIANA;0;QUADRA;22;LOTE;6;;;-16.6810;-49.2210;1;74000000\n" +
	"5208707;CENTRO;RUA;;SEM LOCALIZADOR;0;FUNDOS;;;;;;-16.6;-49.2;1;74000000\n" +
	"5208707;CENTRO;RUA;;NUMERADA;15;;;;;;;-16.6;-49.2;1;74000000\n" +
	"5208707;SETOR OESTE;RUA;DOUTOR;JOÃO DA SILVA;100;;;;;;;-16.6800;-49.2600;1;74110000\n" +
	"5208707;SETOR OESTE;RUA;DOUTOR;JOÃO DA SILVA;100;APARTAMENTO;12;;;;;-16.6802;-49.2602;1;74110000\n" +
	"5208707;SETOR OESTE;RUA;DOUTOR;JOÃO DA SILVA;140;;;;;;;-16.6810;-49.2610;2;74110000\n" +
	// One street name in two localities far apart, and one place under two spellings of its locality.
	"5208707;VILA ALFA;RUA;;BELA VISTA;10;;;;;;;-16.6000;-49.3000;1;74000001\n" +
	"5208707;VILA BETA;RUA;;BELA VISTA;10;;;;;;;-16.7500;-49.1000;1;74000002\n" +
	"5208707;SETOR NOVO;RUA;;PERTO;5;;;;;;;-16.7000;-49.2000;1;74000003\n" +
	"5208707;SETOR NOVO I;RUA;;PERTO;5;;;;;;;-16.7001;-49.2001;1;74000003\n"

func loadSample(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	data, _, err := ReadCNEFE(strings.NewReader(cnefeSample))
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadCNEFE(context.Background(), sqlDB, data); err != nil {
		t.Fatal(err)
	}
	if err := LoadCNEFE(context.Background(), sqlDB, data); err != nil {
		t.Fatalf("loading again replaces: %v", err)
	}
	for _, m := range []struct{ code, name, uf string }{
		{"5300108", "Brasília", "DF"}, {"5208707", "Goiânia", "GO"}, {"5200258", "Águas Lindas de Goiás", "GO"},
		{"3106200", "Belo Horizonte", "MG"}, {"3500000", "Campo Grande", "SP"}, {"5002704", "Campo Grande", "MS"},
	} {
		if _, err := sqlDB.Exec(`INSERT INTO municipalities (code, name, uf, geom, source) VALUES ($1, $2, $3,
			ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON('{"type":"Polygon","coordinates":[[[-50,-17],[-48,-17],[-48,-15],[-50,-15],[-50,-17]]]}'), 4326))::geography, 'test')`,
			m.code, m.name, m.uf); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadCNEFE(t *testing.T) {
	data, stats, err := ReadCNEFE(strings.NewReader(cnefeSample))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Rows != 20 || stats.Located != 10 || stats.Coarse != 1 || stats.NoNumber != 1 || stats.Entries != 7 {
		t.Fatalf("stats %+v", stats)
	}
	// Each place under its whole key and its shorter ones; a key in two localities twice.
	for key, n := range map[string]int{"QR 308 CONJUNTO 13 LOTE 8": 1, "QR 308 CONJUNTO 13": 1, "QUADRA 3 CONJUNTO B": 2,
		"AVENIDA URUGUAIANA QUADRA 22 LOTE 4 CASA 1": 1, "AVENIDA URUGUAIANA QUADRA 22 LOTE 4": 1, "AVENIDA URUGUAIANA QUADRA 22": 1} {
		found := 0
		for k := range data.places {
			if k.place == key {
				found++
			}
		}
		if found != n {
			t.Errorf("%q filed %d times, want %d", key, found, n)
		}
	}
	if entry := data.places[cnefePlaceKey{"5300108", "SAMAMBAIA", "QR 308 CONJUNTO 13"}]; entry == nil || entry.count != 3 {
		t.Errorf("the conjunto is its lotes: %+v", entry)
	}
	// Two addresses at one number are averaged; the street keeps IBGE's spelling for display.
	entry := data.numbers[cnefeKey{"5208707", "RUA DOUTOR JOAO SILVA", "SETOR OESTE", 100}]
	if entry == nil || entry.count != 2 || entry.street != "RUA DOUTOR JOÃO DA SILVA" || math.Abs(entry.lat+16.6801) > 1e-9 {
		t.Errorf("number 100: %+v", entry)
	}
}

func TestCNEFEFind(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	loadSample(t, sqlDB)
	layered := NewLayered(sqlDB, nil)
	for _, c := range []struct {
		address, precision string
		lat                float64
		flags              []string
	}{
		{"QR 308 CONJUNTO 13 LOTE 8, SAMAMBAIA/DF", PrecisionGeocoded, -15.8773, nil}, // the region for a municipality: DF has one
		{"QR 308, CONJ. 13, LT. 08 - Samambaia, Brasília/DF", PrecisionGeocoded, -15.8773, nil},
		{"QR 308 CONJUNTO 13 LOTE 99, SAMAMBAIA/DF", PrecisionStreet, -15.87753, []string{FlagLocatorPartial}}, // the conjunto
		{"SQN 103 BLOCO B APTO 401, ASA NORTE, BRASILIA/DF", PrecisionGeocoded, -15.7792, nil},
		{"QUADRA 3 CONJUNTO B LOTE 1, VARJÃO/DF", PrecisionGeocoded, -15.71, nil},
		{"QUADRA 13 CONJUNTO F LOTE 60, BRASILIA/DF", PrecisionGeocoded, -15.6430, nil}, // one place, two spellings
		{"AV. URUGUAIANA, QD 22, LT 4, CASA 1, JARDIM NOVO MUNDO, GOIÂNIA/GO", PrecisionGeocoded, -16.68, nil},
		{"AVENIDA URUGUAIANA QD 22 LT 9, GOIANIA/GO", PrecisionStreet, -16.6805, []string{FlagLocatorPartial}},
		{"RUA NUMERADA, 15, QD 1, GOIANIA/GO", PrecisionGeocoded, -16.6, nil}, // the number first
		{"R. Dr. João da Silva, 100, Goiânia/GO", PrecisionGeocoded, -16.6801, nil},
		{"Rua Doutor Joao Silva 120, Goiânia - Goiás", PrecisionStreet, -16.6801, []string{FlagNearestNumber}}, // 100 and 140 tie: the lower
		{"Rua Dr. Joao Silvva, 140, Goiânia/GO", PrecisionGeocoded, -16.6810, []string{FlagStreetAdjusted}},
		{"Rua Bela Vista, 10, Vila Beta, Goiânia/GO", PrecisionGeocoded, -16.75, nil}, // the locality named
		{"Rua Perto, 5, Goiânia/GO", PrecisionGeocoded, -16.70005, nil},               // two spellings of one place

	} {
		results, err := layered.Search(ctx, Query{Text: c.address})
		if err != nil {
			t.Errorf("%q: %v", c.address, err)
			continue
		}
		got := results[0]
		if got.Precision != c.precision || math.Abs(got.Lat-c.lat) > 0.0001 || !slices.Equal(got.Flags, c.flags) || !strings.HasPrefix(got.Source, "cnefe2022:") {
			t.Errorf("%q: %+v", c.address, got)
		}
	}
	// Not in the list, and no map server: the municipality, or nothing.
	for _, address := range []string{
		"QUADRA 3 CONJUNTO B LOTE 1, BRASÍLIA/DF", // in two towns, and neither named
		"SQS 207 BLOCO I, BRASILIA/DF",            // only a coarse point
		"QD 22 LT 4, JARDIM NOVO MUNDO, GOIANIA/GO",
		"RUA OUTRA, QD 22, LT 4, GOIANIA/GO",
		"AVENIDA URUGUAIANA QD 22 LT 4, ÁGUAS LINDAS DE GOIÁS/GO",
		"Rua Bela Vista, 10, Goiânia/GO", // a street name in two localities, and neither named
	} {
		results, err := layered.Search(ctx, Query{Text: address})
		if err != nil || len(results) != 1 || results[0].Precision != PrecisionMunicipality || !slices.Equal(results[0].Flags, []string{FlagMunicipalityOnly}) {
			t.Errorf("%q: expected the municipality, got %+v %v", address, results, err)
		}
	}
	// A name two states share needs its state; with it, the municipality.
	if _, err := layered.Search(ctx, Query{Text: "Rua A, 1, Campo Grande"}); err != ErrNotFound {
		t.Errorf("Campo Grande with no state: %v", err)
	}
	if results, err := layered.Search(ctx, Query{Text: "Rua A, 1, Campo Grande/MS"}); err != nil || results[0].Source != "ibge:5002704" {
		t.Errorf("Campo Grande/MS: %+v %v", results, err)
	}
}

// fakeMap answers searches from a table of structured streets and free text.
type fakeMap struct {
	results map[string][]Result
	asked   []string
}

func (f *fakeMap) Search(_ context.Context, q Query) ([]Result, error) {
	key := q.Text
	if key == "" {
		key = q.Street + "|" + q.City
	}
	f.asked = append(f.asked, key)
	if r, ok := f.results[key]; ok {
		return r, nil
	}
	return nil, ErrNotFound
}

func (f *fakeMap) Reverse(context.Context, float64, float64) (Result, error) {
	return Result{}, ErrNotFound
}

func TestLayeredWithAMapServer(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	loadSample(t, sqlDB)
	bh := func(lat float64, precision string) Result {
		return Result{Lat: lat, Lon: -43.9, Precision: precision, City: "Belo Horizonte", StateCode: "BR-MG", Source: "osm:way/1"}
	}
	fake := &fakeMap{results: map[string][]Result{
		// The same street name in another municipality, then the right one.
		"500 RUA GOIÁS|BELO HORIZONTE":   {{Lat: -20, Precision: PrecisionGeocoded, City: "Contagem", StateCode: "BR-MG"}, bh(-19.92, PrecisionGeocoded)},
		"77 RUA DA BAHIA|BELO HORIZONTE": nil,
		"RUA DA BAHIA|BELO HORIZONTE":    {bh(-19.93, PrecisionStreet)},
		// Far from where the census list puts it.
		"100 RUA DR. JOÃO DA SILVA|GOIÂNIA":  {{Lat: -16.75, Lon: -49.26, Precision: PrecisionGeocoded, City: "Goiânia", StateCode: "BR-GO"}},
		"Aeroporto Internacional de Confins": {{Lat: -19.63, Precision: PrecisionGeocoded, City: "Confins", StateCode: "BR-MG"}},
	}}
	layered := NewLayered(sqlDB, fake)

	results, err := layered.Search(ctx, Query{Text: "Rua Goiás, 500, Belo Horizonte/MG", Limit: 5})
	if err != nil || len(results) != 1 || results[0].Lat != -19.92 || len(results[0].Flags) != 0 {
		t.Errorf("another municipality's street is left out: %+v %v", results, err)
	}
	results, err = layered.Search(ctx, Query{Text: "Rua da Bahia, 77, Belo Horizonte/MG"})
	if err != nil || !slices.Equal(results[0].Flags, []string{FlagStreetOnly}) {
		t.Errorf("the number not found, the street: %+v %v", results, err)
	}
	results, err = layered.Search(ctx, Query{Text: "Aeroporto Internacional de Confins"})
	if err != nil || !slices.Equal(results[0].Flags, []string{FlagNamedPlace}) {
		t.Errorf("a named place: %+v %v", results, err)
	}
	results, err = layered.Search(ctx, Query{Text: "R. Dr. João da Silva, 100, Goiânia/GO"})
	if err != nil || !strings.HasPrefix(results[0].Source, "cnefe2022:") || !slices.Equal(results[0].Flags, []string{FlagSourcesDisagree}) {
		t.Errorf("the census list first, and the map far from it: %+v %v", results, err)
	}
	fake.asked = nil
	if results, err = layered.Search(ctx, Query{Text: "Rua Inexistente, 9, Belo Horizonte/MG"}); err != nil || results[0].Precision != PrecisionMunicipality {
		t.Errorf("nothing anywhere: the municipality: %+v %v", results, err)
	}
	if !slices.Equal(fake.asked, []string{"9 RUA INEXISTENTE|BELO HORIZONTE", "RUA INEXISTENTE|BELO HORIZONTE"}) {
		t.Errorf("steps asked of the map: %q", fake.asked)
	}
}
