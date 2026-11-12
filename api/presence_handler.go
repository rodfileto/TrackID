package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/person"
)

// PresenceMapResponse is where a person or a cluster appears: the occurrences of the cases it has traces in, as the
// occurrence map's GeoJSON, with what could not be drawn. A case with no occurrence, and an occurrence with no place
// (or only its unit's municipality, see occurrences.PrecisionJurisdiction), are not points: the counts say how many,
// so the map is never taken for the whole of the presence.
type PresenceMapResponse struct {
	OccurrenceMapResponse
	Cases          int64 `json:"cases"`          // cases the person or cluster has traces in
	WithOccurrence int64 `json:"withOccurrence"` // of them, with an occurrence
	Occurrences    int64 `json:"occurrences"`    // distinct occurrences those cases have
	Unplaced       int64 `json:"unplaced"`       // of those, with no place to draw
}

// presenceMap builds the response for a set of case ids.
func presenceMap(ctx context.Context, sqlDB *sql.DB, caseIDs []string) (PresenceMapResponse, error) {
	resp := PresenceMapResponse{OccurrenceMapResponse: OccurrenceMapResponse{Type: "FeatureCollection", Features: []OccurrenceFeature{}}}
	if len(caseIDs) == 0 {
		return resp, nil
	}
	q := db.New(sqlDB)
	counts, err := q.PresenceCounts(ctx, caseIDs)
	if err != nil {
		return resp, err
	}
	resp.Cases, resp.WithOccurrence, resp.Occurrences, resp.Unplaced = counts.Cases, counts.WithOccurrence, counts.Occurrences, counts.Unplaced
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
	mine := make(map[string]bool, len(caseIDs))
	for _, id := range caseIDs {
		mine[id] = true
	}
	for i := range features {
		for _, c := range features[i].Properties.Cases {
			if mine[c.CaseID] {
				features[i].Properties.InSet = append(features[i].Properties.InSet, c.CaseID)
			}
		}
	}
	resp.Features = features
	return resp, nil
}

// PersonOccurrencesHandler returns where a person appears (person.ListCases: the cases their resolved clusters hold
// traces of). An intelligence read of the person, logged as one (router).
func PersonOccurrencesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		related, err := person.ListCases(c.Request.Context(), sqlDB, c.Param("personId"))
		if errors.Is(err, person.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the person's cases"})
			return
		}
		ids := make([]string, len(related))
		for i, r := range related {
			ids[i] = r.CaseID
		}
		resp, err := presenceMap(c.Request.Context(), sqlDB, ids)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the person's occurrences"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ClusterOccurrencesHandler returns where a cluster appears: the occurrences of the cases it holds traces of.
func ClusterOccurrencesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		clusterID, err := strconv.ParseInt(c.Param("clusterId"), 10, 64)
		if err != nil || clusterID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
			return
		}
		ctx := c.Request.Context()
		q := db.New(sqlDB)
		members, err := q.ListClusterMembersByClusterIDs(ctx, []int64{clusterID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the cluster"})
			return
		}
		if len(members) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		cases, err := q.AskClusterCases(ctx, clusterID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the cluster's cases"})
			return
		}
		ids := make([]string, len(cases))
		for i, r := range cases {
			ids[i] = r.CaseID
		}
		resp, err := presenceMap(ctx, sqlDB, ids)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the cluster's occurrences"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}
