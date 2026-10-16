package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/occurrences"
)

type createOccurrenceRequest struct {
	Types         []string   `json:"types"`
	Attempted     *bool      `json:"attempted"`
	OccurredFrom  time.Time  `json:"occurredFrom"`
	OccurredTo    *time.Time `json:"occurredTo"`
	ReportedAt    *time.Time `json:"reportedAt"`
	Status        string     `json:"status"`
	Description   string     `json:"description"`
	Agency        string     `json:"agency"`
	AgencyNumber  string     `json:"agencyNumber"`
	OffenderCount *int16     `json:"offenderCount"`
	VictimCount   *int16     `json:"victimCount"`
	CaseID        string     `json:"caseId"` // optional: link this case in the same transaction
	Place         struct {
		ExistingID    int64   `json:"existingId"`
		Lon           float64 `json:"lon"`
		Lat           float64 `json:"lat"`
		Precision     string  `json:"precision"`
		Name          string  `json:"name"`
		Address       string  `json:"address"`
		Neighbourhood string  `json:"neighbourhood"`
		PlaceType     string  `json:"placeType"`
	} `json:"place"`
}

// CreateOccurrenceHandler registers an occurrence (occurrences.Create): TrackID numbers it.
func CreateOccurrenceHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		var r createOccurrenceRequest
		if err := c.ShouldBindJSON(&r); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		// Linking a case is working on it: the same rule as the cases/:caseId routes (CaseAccess).
		if caseID := strings.TrimSpace(r.CaseID); caseID != "" {
			switch err := distribution.CanOpen(c.Request.Context(), sqlDB, currentUser(c), caseID); {
			case errors.Is(err, distribution.ErrNotAssigned):
				c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
				return
			case err != nil && !errors.Is(err, analyses.ErrCaseNotFound): // an unknown case is occurrences.Create's to report
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check case access"})
				return
			}
		}
		created, err := occurrences.Create(c.Request.Context(), sqlDB, occurrences.CreateInput{
			Types: r.Types, Attempted: r.Attempted, OccurredFrom: r.OccurredFrom, OccurredTo: r.OccurredTo,
			ReportedAt: r.ReportedAt, Status: r.Status, Description: r.Description,
			Agency: r.Agency, AgencyNumber: r.AgencyNumber, OffenderCount: r.OffenderCount, VictimCount: r.VictimCount,
			CaseID: r.CaseID,
			Place: occurrences.PlaceInput{
				ExistingID: r.Place.ExistingID, Lon: r.Place.Lon, Lat: r.Place.Lat, Precision: r.Place.Precision,
				Name: r.Place.Name, Address: r.Place.Address, Neighbourhood: r.Place.Neighbourhood, PlaceType: r.Place.PlaceType,
			},
		}, time.Now())
		if err != nil {
			writeOccurrenceError(c, err, "could not register occurrence")
			return
		}
		c.JSON(http.StatusCreated, created)
	}
}

func writeOccurrenceError(c *gin.Context, err error, fallback string) {
	var v occurrences.ValidationError
	if errors.As(err, &v) {
		c.JSON(http.StatusBadRequest, gin.H{"error": v.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
}

// OccurrenceSummary is an occurrence as a list or a case page shows it.
type OccurrenceSummary struct {
	ID            int64     `json:"id"`
	TrackIDNumber string    `json:"trackidNumber,omitempty"`
	Agency        string    `json:"agency,omitempty"`
	Number        string    `json:"number,omitempty"`
	OccurredFrom  time.Time `json:"occurredFrom"`
	Status        string    `json:"status"`
	Types         []string  `json:"types"`
	PlaceName     string    `json:"placeName,omitempty"`
	Neighbourhood string    `json:"neighbourhood,omitempty"`
}

func summarize(id int64, year, seq sql.NullInt32, agency, number sql.NullString, from time.Time, status string,
	place, neighbourhood sql.NullString, types json.RawMessage) (OccurrenceSummary, error) {
	s := OccurrenceSummary{
		ID: id, TrackIDNumber: trackidNumber(year, seq), Agency: agency.String, Number: number.String,
		OccurredFrom: from, Status: status, PlaceName: place.String, Neighbourhood: neighbourhood.String,
	}
	return s, json.Unmarshal(types, &s.Types)
}

// SearchOccurrencesHandler finds occurrences by TrackID or police number (?q=, at least 2
// characters): the case page's link search.
func SearchOccurrencesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		q := strings.TrimSpace(c.Query("q"))
		if len([]rune(q)) < 2 {
			c.JSON(http.StatusOK, gin.H{"items": []OccurrenceSummary{}})
			return
		}
		rows, err := db.New(sqlDB).SearchOccurrences(c.Request.Context(), q)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search occurrences"})
			return
		}
		items := make([]OccurrenceSummary, 0, len(rows))
		for _, r := range rows {
			s, err := summarize(r.ID, r.OccurrenceYear, r.OccurrenceSeq, r.Agency, r.OccurrenceNumber, r.OccurredFrom, r.Status, r.PlaceName, r.Neighbourhood, r.Types)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read occurrences"})
				return
			}
			items = append(items, s)
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

// CaseOccurrenceHandler returns the occurrence a case is linked to ({"occurrence": null} when
// none).
func CaseOccurrenceHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		ctx := c.Request.Context()
		q := db.New(sqlDB)
		bc, err := q.GetCaseOccurrenceID(ctx, c.Param("caseId"))
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read case"})
			return
		}
		if !bc.OccurrenceID.Valid {
			c.JSON(http.StatusOK, gin.H{"occurrence": nil, "caseType": bc.CaseType})
			return
		}
		r, err := q.GetOccurrenceSummary(ctx, bc.OccurrenceID.Int64)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read occurrence"})
			return
		}
		s, err := summarize(r.ID, r.OccurrenceYear, r.OccurrenceSeq, r.Agency, r.OccurrenceNumber, r.OccurredFrom, r.Status, r.PlaceName, r.Neighbourhood, r.Types)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read occurrence"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"occurrence": s, "caseType": bc.CaseType})
	}
}

// LinkCaseOccurrenceHandler links a case to an occurrence ({"occurrenceId": 12}) or unlinks it
// ({"occurrenceId": null}); occurrences.LinkCase checks they are of the same nature.
func LinkCaseOccurrenceHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		var r struct {
			OccurrenceID *int64 `json:"occurrenceId"`
		}
		if err := c.ShouldBindJSON(&r); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if err := occurrences.LinkCase(c.Request.Context(), sqlDB, c.Param("caseId"), r.OccurrenceID); err != nil {
			writeOccurrenceError(c, err, "could not link the case")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// NearbyLocationsHandler lists existing locations within ?radius= metres (default 50, max 500)
// of ?lat=&lon=, nearest first, for reuse when registering an occurrence.
func NearbyLocationsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		lon, lat, ok := parsePoint(c)
		if !ok {
			return
		}
		radius := 50.0
		if v, err := strconv.ParseFloat(c.Query("radius"), 64); err == nil && v > 0 {
			radius = min(v, 500)
		}
		rows, err := db.New(sqlDB).ListLocationsNear(c.Request.Context(), db.ListLocationsNearParams{Lon: lon, Lat: lat, RadiusM: radius})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list locations"})
			return
		}
		type item struct {
			ID              int64   `json:"id"`
			Name            string  `json:"name,omitempty"`
			Address         string  `json:"address,omitempty"`
			Neighbourhood   string  `json:"neighbourhood,omitempty"`
			PlaceType       string  `json:"placeType,omitempty"`
			Precision       string  `json:"precision"`
			Lon             float64 `json:"lon"`
			Lat             float64 `json:"lat"`
			DistanceM       float64 `json:"distanceM"`
			OccurrenceCount int32   `json:"occurrenceCount"`
		}
		items := make([]item, 0, len(rows))
		for _, r := range rows {
			items = append(items, item{r.ID, r.Name.String, r.Address.String, r.Neighbourhood.String, r.PlaceType.String,
				r.Precision, r.Lon, r.Lat, r.DistanceM, r.OccurrenceCount})
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func parsePoint(c *gin.Context) (lon, lat float64, ok bool) {
	lon, err1 := strconv.ParseFloat(c.Query("lon"), 64)
	lat, err2 := strconv.ParseFloat(c.Query("lat"), 64)
	if err1 != nil || err2 != nil || lon < -180 || lon > 180 || lat < -90 || lat > 90 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "lat and lon are required"})
		return 0, 0, false
	}
	return lon, lat, true
}

// GeocodeSearchHandler geocodes ?q= (up to 5 results). The form calls it when the user presses
// Search, never while typing: the public Nominatim forbids autocomplete.
func GeocodeSearchHandler(g geocode.Geocoder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "geocoder is not configured"})
			return
		}
		q := strings.TrimSpace(c.Query("q"))
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
			return
		}
		res, err := g.Search(c.Request.Context(), geocode.Query{Text: q, Limit: 5})
		if errors.Is(err, geocode.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{"items": []geocode.Result{}})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "the geocoder did not answer"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": res})
	}
}

// GeocodeReverseHandler returns the address at ?lat=&lon= ({"item": null} when none).
func GeocodeReverseHandler(g geocode.Geocoder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "geocoder is not configured"})
			return
		}
		lon, lat, ok := parsePoint(c)
		if !ok {
			return
		}
		res, err := g.Reverse(c.Request.Context(), lon, lat)
		if errors.Is(err, geocode.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{"item": nil})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "the geocoder did not answer"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": res})
	}
}
