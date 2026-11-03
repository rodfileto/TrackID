package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/db"
)

// maxMapOccurrences caps one map response; Truncated says when it was reached.
const maxMapOccurrences = 20000

// reportZone is the time zone years are cut in: a report's year is its local year.
var reportZone = func() *time.Location {
	if z, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return z
	}
	return time.FixedZone("BRT", -3*3600)
}()

// OccurrenceCase is one biometric case linked to an occurrence.
type OccurrenceCase struct {
	CaseID   string `json:"caseId"`
	Modality string `json:"modality"`
	CaseType string `json:"caseType"`
}

// OccurrenceProperties are a map feature's properties. Codes (types, group, status, precision,
// placeType) are untranslated: the frontend translates them.
type OccurrenceProperties struct {
	ID int64 `json:"id"`
	// TrackIDNumber is "<year>.<sequence>" for an occurrence registered in TrackID; Agency and
	// Number are the police agency's own report number, when known. At least one is set.
	TrackIDNumber string           `json:"trackidNumber,omitempty"`
	Agency        string           `json:"agency,omitempty"`
	Number        string           `json:"number,omitempty"`
	OccurredFrom  time.Time        `json:"occurredFrom"`
	OccurredTo    *time.Time       `json:"occurredTo,omitempty"`
	RegisteredAt  time.Time        `json:"registeredAt"`
	Attempted     *bool            `json:"attempted,omitempty"`
	Status        string           `json:"status"`
	Description   string           `json:"description"`
	Types         []string         `json:"types"`
	Group         string           `json:"group"`    // the first type's group
	CaseType      string           `json:"caseType"` // CRIMINAL or CIVIL, the first type's
	LocationID    string           `json:"locationId"`
	Precision     string           `json:"precision"`
	PlaceType     string           `json:"placeType,omitempty"`
	PlaceName     string           `json:"placeName,omitempty"`
	Neighbourhood string           `json:"neighbourhood,omitempty"`
	Cases         []OccurrenceCase `json:"cases"`
}

// OccurrenceFeature is a GeoJSON point feature.
type OccurrenceFeature struct {
	Type       string               `json:"type"` // "Feature"
	Geometry   pointGeometry        `json:"geometry"`
	Properties OccurrenceProperties `json:"properties"`
}

type pointGeometry struct {
	Type        string     `json:"type"` // "Point"
	Coordinates [2]float64 `json:"coordinates"`
}

// OccurrenceMapResponse is a GeoJSON FeatureCollection, which map libraries read as is.
type OccurrenceMapResponse struct {
	Type      string              `json:"type"` // "FeatureCollection"
	Features  []OccurrenceFeature `json:"features"`
	Truncated bool                `json:"truncated"`
}

// OccurrenceMapHandler returns occurrences as GeoJSON points. ?year= keeps one (local) year;
// ?group= keeps the comma-separated occurrence-type groups (ROBBERY, THEFT, ...).
func OccurrenceMapHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		ctx := c.Request.Context()
		q := db.New(sqlDB)
		params := db.ListOccurrencesForMapParams{MaxRows: maxMapOccurrences + 1}
		if y := c.Query("year"); y != "" {
			year, err := strconv.Atoi(y)
			if err != nil || year < 1900 || year > 3000 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year"})
				return
			}
			params.OccurredAfter = sql.NullTime{Time: time.Date(year, 1, 1, 0, 0, 0, 0, reportZone), Valid: true}
			params.OccurredBefore = sql.NullTime{Time: time.Date(year+1, 1, 1, 0, 0, 0, 0, reportZone), Valid: true}
		}
		groups := map[string]bool{}
		for _, g := range strings.Split(c.Query("group"), ",") {
			if g = strings.TrimSpace(g); g != "" {
				groups[g] = true
			}
		}

		types, err := q.ListOccurrenceTypes(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list occurrence types"})
			return
		}
		rows, err := q.ListOccurrencesForMap(ctx, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list occurrences"})
			return
		}
		resp := OccurrenceMapResponse{Type: "FeatureCollection", Features: []OccurrenceFeature{}}
		if len(rows) > maxMapOccurrences {
			rows, resp.Truncated = rows[:maxMapOccurrences], true
		}
		features, err := occurrenceFeatures(rows, types)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read occurrences"})
			return
		}
		for _, f := range features {
			if len(groups) == 0 || groups[f.Properties.Group] {
				resp.Features = append(resp.Features, f)
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}

// occurrenceFeatures turns map rows into GeoJSON features, taking each one's group and case
// type from its first occurrence type.
func occurrenceFeatures(rows []db.ListOccurrencesForMapRow, types []db.OccurrenceType) ([]OccurrenceFeature, error) {
	byCode := make(map[string]db.OccurrenceType, len(types))
	for _, t := range types {
		byCode[t.Code] = t
	}
	out := make([]OccurrenceFeature, 0, len(rows))
	for _, r := range rows {
		p := OccurrenceProperties{
			ID: r.ID, Agency: r.Agency.String, Number: r.OccurrenceNumber.String,
			TrackIDNumber: trackidNumber(r.OccurrenceYear, r.OccurrenceSeq),
			OccurredFrom:  r.OccurredFrom, RegisteredAt: r.RegisteredAt,
			Status: r.Status, Description: r.Description,
			LocationID: r.LocationID, Precision: r.Precision,
			PlaceType: r.PlaceType.String, PlaceName: r.PlaceName.String, Neighbourhood: r.Neighbourhood.String,
		}
		if r.OccurredTo.Valid {
			p.OccurredTo = &r.OccurredTo.Time
		}
		if r.Attempted.Valid {
			p.Attempted = &r.Attempted.Bool
		}
		if err := json.Unmarshal(r.Types, &p.Types); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(r.Cases, &p.Cases); err != nil {
			return nil, err
		}
		if len(p.Types) > 0 {
			t := byCode[p.Types[0]]
			p.Group, p.CaseType = t.Grp, t.CaseType
		}
		out = append(out, OccurrenceFeature{
			Type:       "Feature",
			Geometry:   pointGeometry{Type: "Point", Coordinates: [2]float64{r.Lon, r.Lat}},
			Properties: p,
		})
	}
	return out, nil
}

// OccurrenceTypesHandler lists the occurrence types (code, caseType, group) for filters.
func OccurrenceTypesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		types, err := db.New(sqlDB).ListOccurrenceTypes(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list occurrence types"})
			return
		}
		type item struct {
			Code     string `json:"code"`
			CaseType string `json:"caseType"`
			Group    string `json:"group"`
		}
		out := make([]item, 0, len(types))
		for _, t := range types {
			out = append(out, item{t.Code, t.CaseType, t.Grp})
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

// OccurrenceYearsHandler lists the (local) years that have occurrences.
func OccurrenceYearsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		years, err := db.New(sqlDB).ListOccurrenceYears(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list occurrence years"})
			return
		}
		if years == nil {
			years = []int32{}
		}
		c.JSON(http.StatusOK, gin.H{"years": years})
	}
}

// trackidNumber formats a registered occurrence's TrackID number, "2026.0001"; "" for one
// without (ingested with the police number only).
func trackidNumber(year, seq sql.NullInt32) string {
	if !year.Valid || !seq.Valid {
		return ""
	}
	return fmt.Sprintf("%d.%04d", year.Int32, seq.Int32)
}
