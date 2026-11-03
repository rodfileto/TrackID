package geocode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPrecisionOf(t *testing.T) {
	for _, c := range []struct {
		rank  int
		house string
		want  string
	}{
		{30, "", PrecisionGeocoded},
		{26, "459", PrecisionGeocoded}, // a number on a street-ranked match
		{26, "", PrecisionStreet},
		{20, "", PrecisionNeighbourhood},
		{16, "", PrecisionMunicipality},
		{8, "", ""}, // a state: too coarse
	} {
		if got := precisionOf(c.rank, c.house); got != c.want {
			t.Errorf("precisionOf(%d, %q) = %q, want %q", c.rank, c.house, got, c.want)
		}
	}
}

func TestNominatim(t *testing.T) {
	var mu sync.Mutex
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r)
		mu.Unlock()
		switch r.URL.Path {
		case "/search":
			if r.URL.Query().Get("street") == "nowhere" {
				w.Write([]byte(`[{"osm_type":"relation","osm_id":1,"lat":"-22.9","lon":"-43.2","place_rank":8,"address":{}}]`))
				return
			}
			w.Write([]byte(`[{"osm_type":"node","osm_id":42,"lat":"-22.9845","lon":"-43.2011","place_rank":30,
				"display_name":"459, Rua Visconde de Pirajá, Ipanema, Rio de Janeiro",
				"address":{"house_number":"459","road":"Rua Visconde de Pirajá","suburb":"Ipanema",
				"city":"Rio de Janeiro","state":"Rio de Janeiro","ISO3166-2-lvl4":"BR-RJ",
				"postcode":"22410-003","country_code":"br"}}]`))
		case "/reverse":
			if r.URL.Query().Get("lat") == "0" {
				w.Write([]byte(`{"error":"Unable to geocode"}`))
				return
			}
			w.Write([]byte(`{"osm_type":"way","osm_id":7,"lat":"-22.98","lon":"-43.20","place_rank":26,
				"address":{"road":"Rua Visconde de Pirajá","city":"Rio de Janeiro"}}`))
		}
	}))
	defer srv.Close()

	if _, err := NewNominatim(NominatimConfig{BaseURL: srv.URL}); err == nil {
		t.Error("NewNominatim without a User-Agent: accepted")
	}
	n, err := NewNominatim(NominatimConfig{BaseURL: srv.URL, UserAgent: "trackid-test", Email: "ops@example.org", MinInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	start := time.Now()
	res, err := n.Search(ctx, Query{Street: "Rua Visconde de Pirajá 459", City: "Rio de Janeiro", State: "RJ", Postcode: "22410-003"})
	if err != nil {
		t.Fatal(err)
	}
	r := res[0]
	if r.Precision != PrecisionGeocoded || r.Lat != -22.9845 || r.Lon != -43.2011 || r.Neighbourhood != "Ipanema" ||
		r.StateCode != "BR-RJ" || r.Source != "osm:node/42" {
		t.Errorf("search result: %+v", r)
	}

	if _, err := n.Search(ctx, Query{Street: "nowhere"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("state-level match: err = %v, want ErrNotFound", err)
	}

	rev, err := n.Reverse(ctx, -43.2011, -22.9845)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Precision != PrecisionStreet || rev.Lon != -43.2011 || rev.Lat != -22.9845 {
		t.Errorf("reverse result: %+v", rev)
	}
	if _, err := n.Reverse(ctx, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("reverse with nothing there: err = %v, want ErrNotFound", err)
	}

	// Four requests, spaced by at least MinInterval.
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("4 requests took %v, want >= 150ms at a 50ms interval", d)
	}
	q := seen[0].URL.Query()
	for k, want := range map[string]string{
		"street": "Rua Visconde de Pirajá 459", "city": "Rio de Janeiro", "state": "RJ",
		"postalcode": "22410-003", "country": "Brasil", "countrycodes": "br",
		"format": "jsonv2", "addressdetails": "1", "limit": "1", "email": "ops@example.org",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("search param %s = %q, want %q", k, got, want)
		}
	}
	if ua := seen[0].Header.Get("User-Agent"); ua != "trackid-test" {
		t.Errorf("User-Agent = %q", ua)
	}
	if lang := seen[0].Header.Get("Accept-Language"); lang != "pt-BR" {
		t.Errorf("Accept-Language = %q", lang)
	}
}

func TestNominatimPublicDefaults(t *testing.T) {
	n, err := NewNominatim(NominatimConfig{UserAgent: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if n.baseURL != PublicNominatimURL || n.minInterval != time.Second {
		t.Errorf("public defaults: url %q, interval %v", n.baseURL, n.minInterval)
	}
}
