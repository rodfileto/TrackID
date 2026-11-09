package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PublicNominatimURL is OpenStreetMap's own Nominatim instance. Its usage
// policy (https://operations.osmfoundation.org/policies/nominatim/) allows at
// most one request per second, requires an identifying User-Agent, and forbids
// autocomplete (search-as-you-type) and systematic bulk geocoding: a picker
// that suggests as the user types needs a self-hosted instance, set through
// GEOCODER_URL.
const PublicNominatimURL = "https://nominatim.openstreetmap.org"

// Nominatim is a Geocoder backed by a Nominatim server.
type Nominatim struct {
	baseURL     string
	userAgent   string
	email       string
	language    string
	countries   string
	minInterval time.Duration
	http        *http.Client

	mu   sync.Mutex
	last time.Time
}

// NominatimConfig configures NewNominatim. Only UserAgent is required.
type NominatimConfig struct {
	BaseURL     string        // defaults to PublicNominatimURL
	UserAgent   string        // identifies the application, as the usage policy requires
	Email       string        // optional contact sent with each request
	Language    string        // Accept-Language for names; defaults to "pt-BR"
	Countries   string        // comma-separated ISO 3166-1 codes searches are limited to; defaults to "br"
	MinInterval time.Duration // minimum gap between requests; defaults to 1s on the public instance, 0 elsewhere
}

// NewNominatim returns a Nominatim client. Requests from one client are spaced
// by MinInterval, so a single client shared by the process keeps it within the
// server's rate limit.
func NewNominatim(cfg NominatimConfig) (*Nominatim, error) {
	if strings.TrimSpace(cfg.UserAgent) == "" {
		return nil, fmt.Errorf("geocode: a User-Agent identifying the application is required")
	}
	n := &Nominatim{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		userAgent:   cfg.UserAgent,
		email:       cfg.Email,
		language:    cfg.Language,
		countries:   cfg.Countries,
		minInterval: cfg.MinInterval,
		http:        &http.Client{Timeout: 30 * time.Second},
	}
	if n.baseURL == "" {
		n.baseURL = PublicNominatimURL
	}
	if n.language == "" {
		n.language = "pt-BR"
	}
	if n.countries == "" {
		n.countries = "br"
	}
	if n.minInterval == 0 && n.baseURL == PublicNominatimURL {
		n.minInterval = time.Second
	}
	return n, nil
}

// Search geocodes q, best match first. It returns ErrNotFound when nothing
// matches at municipality precision or better.
func (n *Nominatim) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{}
	if q.Text != "" {
		v.Set("q", q.Text)
	} else {
		for k, s := range map[string]string{
			"street": q.Street, "city": q.City, "state": q.State,
			"postalcode": q.Postcode, "country": q.Country,
		} {
			if s != "" {
				v.Set(k, s)
			}
		}
		if q.Country == "" {
			v.Set("country", "Brasil")
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 1
	}
	v.Set("limit", strconv.Itoa(limit))
	v.Set("countrycodes", n.countries)

	var places []nominatimPlace
	if err := n.get(ctx, "/search", v, &places); err != nil {
		return nil, err
	}
	var out []Result
	for _, p := range places {
		if r, ok := p.result(); ok {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

// Reverse returns the address at a point.
func (n *Nominatim) Reverse(ctx context.Context, lon, lat float64) (Result, error) {
	v := url.Values{}
	v.Set("lon", strconv.FormatFloat(lon, 'f', -1, 64))
	v.Set("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	v.Set("zoom", "18")

	var p nominatimPlace
	if err := n.get(ctx, "/reverse", v, &p); err != nil {
		return Result{}, err
	}
	r, ok := p.result()
	if !ok {
		return Result{}, ErrNotFound
	}
	// The point asked about, not the matched object's centre.
	r.Lon, r.Lat = lon, lat
	return r, nil
}

func (n *Nominatim) get(ctx context.Context, path string, v url.Values, out any) error {
	v.Set("format", "jsonv2")
	v.Set("addressdetails", "1")
	if n.email != "" {
		v.Set("email", n.email)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.baseURL+path+"?"+v.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", n.userAgent)
	req.Header.Set("Accept-Language", n.language)

	if err := n.wait(ctx); err != nil {
		return err
	}
	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("geocode: %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("geocode: %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geocode: %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	// /reverse answers 200 with {"error": "Unable to geocode"} when nothing is there.
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return ErrNotFound
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("geocode: %s: %w", path, err)
	}
	return nil
}

// wait blocks until minInterval has passed since the previous request.
func (n *Nominatim) wait(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if d := time.Until(n.last.Add(n.minInterval)); d > 0 {
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	n.last = time.Now()
	return nil
}

type nominatimPlace struct {
	OSMType     string `json:"osm_type"`
	OSMID       int64  `json:"osm_id"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	PlaceRank   int    `json:"place_rank"`
	DisplayName string `json:"display_name"`
	Address     struct {
		HouseNumber   string `json:"house_number"`
		Road          string `json:"road"`
		Suburb        string `json:"suburb"`
		Neighbourhood string `json:"neighbourhood"`
		CityDistrict  string `json:"city_district"`
		City          string `json:"city"`
		Town          string `json:"town"`
		Municipality  string `json:"municipality"`
		State         string `json:"state"`
		StateCode     string `json:"ISO3166-2-lvl4"`
		Postcode      string `json:"postcode"`
		CountryCode   string `json:"country_code"`
	} `json:"address"`
}

// precisionOf maps Nominatim's place_rank
// (https://nominatim.org/release-docs/latest/customize/Ranking/) to a
// locations.precision: 30 is a building or address, 26-29 a street, 17-25 a
// neighbourhood or district, 12-16 a municipality. Coarser ranks have none.
func precisionOf(rank int, houseNumber string) string {
	switch {
	case rank >= 30 || houseNumber != "":
		return PrecisionGeocoded
	case rank >= 26:
		return PrecisionStreet
	case rank >= 17:
		return PrecisionNeighbourhood
	case rank >= 12:
		return PrecisionMunicipality
	}
	return ""
}

func (p nominatimPlace) result() (Result, bool) {
	precision := precisionOf(p.PlaceRank, p.Address.HouseNumber)
	if precision == "" {
		return Result{}, false
	}
	lat, err1 := strconv.ParseFloat(p.Lat, 64)
	lon, err2 := strconv.ParseFloat(p.Lon, 64)
	if err1 != nil || err2 != nil {
		return Result{}, false
	}
	a := p.Address
	return Result{
		Lon: lon, Lat: lat, Precision: precision,
		DisplayName:   p.DisplayName,
		HouseNumber:   a.HouseNumber,
		Street:        a.Road,
		Neighbourhood: firstNonEmpty(a.Suburb, a.Neighbourhood, a.CityDistrict),
		City:          firstNonEmpty(a.City, a.Town, a.Municipality),
		State:         a.State,
		StateCode:     a.StateCode,
		Postcode:      a.Postcode,
		CountryCode:   a.CountryCode,
		Source:        fmt.Sprintf("osm:%s/%d", p.OSMType, p.OSMID),
	}, true
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
