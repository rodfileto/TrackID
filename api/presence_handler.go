package api

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/articles"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/presence"
)

// Where a person or a cluster appears (docs/presence-areas-plan.md): the occurrences of the cases it has traces in,
// facial and fingerprint alike, so an occurrence both kinds of case share shows as one fused point.

// Certainty tiers of a link, from articles: examiners confirmed it, the matcher made it, or it is disputed.
const (
	certaintyConfirmed  = "confirmed"
	certaintyUnreviewed = "unreviewed"
	certaintyUncertain  = "uncertain"
)

// Which links a presence map follows: only those examiners confirmed, or every one but those they excluded.
const (
	linksReviewed = "reviewed"
	linksAll      = "all"
)

// setCase is a case of the set a person or cluster appears in, and how sure its link is.
type setCase struct {
	CaseID    string
	Modality  string
	Certainty string
}

// PresenceMapResponse is where a person or a cluster appears: the occurrences of the cases it has traces in, as the
// occurrence map's GeoJSON, with what could not be drawn. A case with no occurrence, and an occurrence with no place
// (or only its unit's municipality, see occurrences.PrecisionJurisdiction), are not points: the counts say how many,
// so the map is never taken for the whole of the presence.
type PresenceMapResponse struct {
	OccurrenceMapResponse
	Links          string `json:"links"`           // reviewed or all
	Basis          string `json:"basis,omitempty"` // of a cluster: AUTOMATIC or REVIEWED
	Cases          int64  `json:"cases"`           // cases the person or cluster has traces in
	Facial         int64  `json:"facial"`          // of them, facial
	Fingerprint    int64  `json:"fingerprint"`     // of them, fingerprint
	WithOccurrence int64  `json:"withOccurrence"`  // of them, with an occurrence
	Occurrences    int64  `json:"occurrences"`     // distinct occurrences those cases have
	Unplaced       int64  `json:"unplaced"`        // of those, with no place to draw
	Fused          int64  `json:"fused"`           // occurrences holding both a facial and a fingerprint case of the set
}

func linksParam(c *gin.Context) (string, bool) {
	switch v := c.DefaultQuery("links", linksAll); v {
	case linksReviewed, linksAll:
		return v, true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "links is reviewed or all"})
	return "", false
}

// personCases are the cases linked to a person at the certainty asked: confirmed ones always, the matcher's and the
// disputed ones too for all links, never those examiners excluded.
func personCases(ctx context.Context, sqlDB *sql.DB, personID, links string) ([]setCase, error) {
	all, err := articles.PersonCaseLinks(ctx, sqlDB, personID)
	if err != nil {
		return nil, err
	}
	var out []setCase
	for _, l := range all {
		if l.Certainty == certaintyConfirmed || (links == linksAll && (l.Certainty == certaintyUnreviewed || l.Certainty == certaintyUncertain)) {
			out = append(out, setCase{l.CaseID, l.Modality, l.Certainty})
		}
	}
	return out, nil
}

// clusterCases are the cases a cluster holds traces of. A REVIEWED cluster's links are confirmed; an AUTOMATIC one's are
// the matcher's, so asking for reviewed links of it gives none. found is false for no such cluster.
func clusterCases(ctx context.Context, sqlDB *sql.DB, clusterID int64, links string) (cases []setCase, basis string, found bool, err error) {
	q := db.New(sqlDB)
	basis, err = q.GetClusterBasis(ctx, clusterID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	certainty := certaintyUnreviewed
	if basis == string(cluster.BasisReviewed) {
		certainty = certaintyConfirmed
	}
	if links == linksReviewed && certainty != certaintyConfirmed {
		return nil, basis, true, nil
	}
	rows, err := q.AskClusterCases(ctx, clusterID)
	if err != nil {
		return nil, basis, true, err
	}
	for _, r := range rows {
		cases = append(cases, setCase{r.CaseID, r.Modality, certainty})
	}
	return cases, basis, true, nil
}

// certaintyRank orders certainty for "the best link an occurrence has".
var certaintyRank = map[string]int{certaintyConfirmed: 0, certaintyUnreviewed: 1, certaintyUncertain: 2}

// presenceMap builds the response for a set of cases.
func presenceMap(ctx context.Context, sqlDB *sql.DB, set []setCase, links string) (PresenceMapResponse, error) {
	resp := PresenceMapResponse{OccurrenceMapResponse: OccurrenceMapResponse{Type: "FeatureCollection", Features: []OccurrenceFeature{}}, Links: links}
	if len(set) == 0 {
		return resp, nil
	}
	caseIDs := make([]string, len(set))
	byCase := make(map[string]setCase, len(set))
	for i, c := range set {
		caseIDs[i] = c.CaseID
		byCase[c.CaseID] = c
		switch c.Modality {
		case "FACIAL":
			resp.Facial++
		case "FINGERPRINT":
			resp.Fingerprint++
		}
	}
	q := db.New(sqlDB)
	counts, err := q.PresenceCounts(ctx, caseIDs)
	if err != nil {
		return resp, err
	}
	resp.Cases, resp.WithOccurrence, resp.Occurrences, resp.Unplaced = counts.Cases, counts.WithOccurrence, counts.Occurrences, counts.Unplaced

	// An occurrence is fused when the set holds both a facial and a fingerprint case on it, drawn or not.
	onOccurrence, err := q.ListCaseOccurrenceIDs(ctx, caseIDs)
	if err != nil {
		return resp, err
	}
	kinds := map[int64]map[string]bool{}
	for _, r := range onOccurrence {
		if kinds[r.OccurrenceID] == nil {
			kinds[r.OccurrenceID] = map[string]bool{}
		}
		kinds[r.OccurrenceID][byCase[r.CaseID].Modality] = true
	}
	for _, k := range kinds {
		if k["FACIAL"] && k["FINGERPRINT"] {
			resp.Fused++
		}
	}

	types, err := q.ListOccurrenceTypes(ctx)
	if err != nil {
		return resp, err
	}
	rows, err := q.ListOccurrencesForMapByCases(ctx, caseIDs)
	if err != nil {
		return resp, err
	}
	if len(rows) > maxMapOccurrences {
		rows, resp.Truncated = rows[:maxMapOccurrences], true
	}
	mapRows := make([]db.ListOccurrencesForMapRow, len(rows))
	for i, r := range rows {
		mapRows[i] = db.ListOccurrencesForMapRow(r)
	}
	features, err := occurrenceFeatures(mapRows, types)
	if err != nil {
		return resp, err
	}
	for i := range features {
		p := &features[i].Properties
		modalities := map[string]bool{}
		for _, c := range p.Cases {
			s, ok := byCase[c.CaseID]
			if !ok {
				continue
			}
			p.InSet = append(p.InSet, c.CaseID)
			modalities[s.Modality] = true
			if p.Certainty == "" || certaintyRank[s.Certainty] < certaintyRank[p.Certainty] {
				p.Certainty = s.Certainty
			}
		}
		for m := range modalities {
			p.Modalities = append(p.Modalities, m)
		}
		sort.Strings(p.Modalities)
	}
	resp.Features = features
	return resp, nil
}

// PersonOccurrencesHandler returns where a person appears. ?links=reviewed keeps the links examiners confirmed;
// the default is every link but the excluded. An intelligence read of the person, logged as one (router).
func PersonOccurrencesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		links, ok := linksParam(c)
		if !ok {
			return
		}
		set, err := personCases(c.Request.Context(), sqlDB, c.Param("personId"), links)
		if errors.Is(err, articles.ErrSubjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the person's cases"})
			return
		}
		resp, err := presenceMap(c.Request.Context(), sqlDB, set, links)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the person's occurrences"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func clusterIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("clusterId"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return 0, false
	}
	return id, true
}

// ClusterOccurrencesHandler returns where a cluster appears: the occurrences of the cases it holds traces of.
func ClusterOccurrencesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		id, ok := clusterIDParam(c)
		if !ok {
			return
		}
		links, ok := linksParam(c)
		if !ok {
			return
		}
		set, basis, found, err := clusterCases(c.Request.Context(), sqlDB, id, links)
		if err == nil && !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the cluster's cases"})
			return
		}
		resp, err := presenceMap(c.Request.Context(), sqlDB, set, links)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the cluster's occurrences"})
			return
		}
		resp.Basis = basis
		c.JSON(http.StatusOK, resp)
	}
}

// minBaseForQuotient is how many occurrences, at least, an area's surroundings must hold for a quotient to mean anything.
const minBaseForQuotient = 10

// footprintFloorM is the least radius an area is measured against the whole base over.
const footprintFloorM = 300

// AreaResponse is an area of presence with how it compares to every occurrence around it.
type AreaResponse struct {
	presence.Area
	// BaseInside is how many placed occurrences of all lie within the area's footprint, the person's own included.
	BaseInside int64 `json:"baseInside"`
	// Quotient is the area's share of the person's mapped occurrences over the footprint's share of all occurrences: above
	// 1 the person is there more than the base is, 1 is as much as anyone. Nil when the base is too small to say.
	Quotient *float64 `json:"quotient,omitempty"`
	// Fused counts the area's occurrences that hold both a facial and a fingerprint case of the set.
	Fused int `json:"fused"`
}

// MunicipalityCount is how many of the person's drawn occurrences are in a municipality.
type MunicipalityCount struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	UF    string `json:"uf"`
	Count int    `json:"count"`
}

// PresenceAreasResponse is the main areas of presence (presence.Areas) with the by-municipality count.
type PresenceAreasResponse struct {
	Links          string              `json:"links"`
	Basis          string              `json:"basis,omitempty"`
	EpsM           float64             `json:"epsM"`
	MinPts         int                 `json:"minPts"`
	Mapped         int                 `json:"mapped"` // occurrences clustered: those placed more exactly than a municipality
	Areas          []AreaResponse      `json:"areas"`
	Noise          []int64             `json:"noise"`
	Municipalities []MunicipalityCount `json:"municipalities"`
	// TooFew says there are fewer mapped occurrences than it takes to single out a main area.
	TooFew bool `json:"tooFew"`
}

func areasParams(c *gin.Context) (eps float64, minPts int, ok bool) {
	eps, minPts = 3000, 2
	if v := c.Query("eps"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 50 || f > 100000 || math.IsNaN(f) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "eps is a distance in metres, from 50 to 100000"})
			return 0, 0, false
		}
		eps = f
	}
	if v := c.Query("minPts"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 || n > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "minPts is from 2 to 50"})
			return 0, 0, false
		}
		minPts = n
	}
	return eps, minPts, true
}

// presenceAreas clusters the drawn occurrences of a set: those placed more exactly than a municipality, whose centre
// says nothing of distance.
func presenceAreas(ctx context.Context, sqlDB *sql.DB, set []setCase, links string, eps float64, minPts int) (PresenceAreasResponse, error) {
	resp := PresenceAreasResponse{Links: links, EpsM: eps, MinPts: minPts, Areas: []AreaResponse{}, Noise: []int64{}, Municipalities: []MunicipalityCount{}}
	m, err := presenceMap(ctx, sqlDB, set, links)
	if err != nil {
		return resp, err
	}
	byID := map[int64]OccurrenceFeature{}
	var points []presence.Point
	codes := map[string]int{}
	for _, f := range m.Features {
		p := f.Properties
		byID[p.ID] = f
		if p.MunicipalityCode != "" {
			codes[p.MunicipalityCode]++
		}
		if p.Precision == "MUNICIPALITY" {
			continue
		}
		points = append(points, presence.Point{ID: p.ID, Lon: f.Geometry.Coordinates[0], Lat: f.Geometry.Coordinates[1],
			At: p.OccurredFrom, Reviewed: p.Certainty == certaintyConfirmed, Types: p.Types})
	}
	found := presence.Areas(points, eps, minPts)
	resp.Mapped, resp.Noise, resp.TooFew = found.Mapped, found.Noise, found.Mapped < presence.MinMainPoints

	q := db.New(sqlDB)
	baseTotal, err := q.CountPlacedOccurrences(ctx)
	if err != nil {
		return resp, err
	}
	for _, a := range found.Areas {
		out := AreaResponse{Area: a}
		for _, id := range a.PointIDs {
			if k := byID[id].Properties.Modalities; len(k) > 1 {
				out.Fused++
			}
		}
		radius := math.Max(a.RadiusM+60, footprintFloorM)
		out.BaseInside, err = q.CountPlacedOccurrencesNear(ctx, db.CountPlacedOccurrencesNearParams{Lon: a.Centre[0], Lat: a.Centre[1], RadiusM: radius})
		if err != nil {
			return resp, err
		}
		if out.BaseInside >= minBaseForQuotient && baseTotal > 0 && found.Mapped > 0 {
			quotient := (float64(len(a.PointIDs)) / float64(found.Mapped)) / (float64(out.BaseInside) / float64(baseTotal))
			out.Quotient = &quotient
		}
		resp.Areas = append(resp.Areas, out)
	}

	if len(codes) > 0 {
		list := make([]string, 0, len(codes))
		for code := range codes {
			list = append(list, code)
		}
		names, err := q.ListMunicipalityNames(ctx, list)
		if err != nil {
			return resp, err
		}
		known := map[string]db.ListMunicipalityNamesRow{}
		for _, n := range names {
			known[n.Code] = n
		}
		for code, count := range codes {
			n := known[code]
			name := n.Name
			if name == "" {
				name = code
			}
			resp.Municipalities = append(resp.Municipalities, MunicipalityCount{Code: code, Name: name, UF: n.Uf, Count: count})
		}
		sort.Slice(resp.Municipalities, func(i, j int) bool {
			a, b := resp.Municipalities[i], resp.Municipalities[j]
			if a.Count != b.Count {
				return a.Count > b.Count
			}
			return a.Name < b.Name
		})
	}
	return resp, nil
}

// PersonPresenceAreasHandler returns a person's main areas of presence. ?eps= (metres) and ?minPts= set the density;
// ?links= as PersonOccurrencesHandler.
func PersonPresenceAreasHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		links, ok := linksParam(c)
		if !ok {
			return
		}
		eps, minPts, ok := areasParams(c)
		if !ok {
			return
		}
		set, err := personCases(c.Request.Context(), sqlDB, c.Param("personId"), links)
		if errors.Is(err, articles.ErrSubjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the person's cases"})
			return
		}
		resp, err := presenceAreas(c.Request.Context(), sqlDB, set, links, eps, minPts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not find the person's areas"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ClusterPresenceAreasHandler returns a cluster's main areas of presence.
func ClusterPresenceAreasHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		id, ok := clusterIDParam(c)
		if !ok {
			return
		}
		links, ok := linksParam(c)
		if !ok {
			return
		}
		eps, minPts, ok := areasParams(c)
		if !ok {
			return
		}
		set, basis, found, err := clusterCases(c.Request.Context(), sqlDB, id, links)
		if err == nil && !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the cluster's cases"})
			return
		}
		resp, err := presenceAreas(c.Request.Context(), sqlDB, set, links, eps, minPts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not find the cluster's areas"})
			return
		}
		resp.Basis = basis
		c.JSON(http.StatusOK, resp)
	}
}
