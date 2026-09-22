package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cluster"
)

// ListClustersResponse is ListClustersHandler's paginated response.
type ListClustersResponse struct {
	Items      []cluster.Summary `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// ListClustersHandler returns every persisted biometric cluster, ordered by
// how many distinct biometric cases it touches (descending). ?identified=true
// keeps only clusters resolved to an enrolled identity (see
// cluster.ListOverview).
func ListClustersHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		page := parsePositiveInt(context.Query("page"), 1)
		pageSize := parsePositiveInt(context.Query("page_size"), 25)

		result, err := cluster.ListOverview(context.Request.Context(), db, cluster.ListOverviewParams{
			Page:           page,
			PageSize:       pageSize,
			IdentifiedOnly: context.Query("identified") == "true",
		})
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list clusters"})
			return
		}
		context.JSON(http.StatusOK, ListClustersResponse{
			Items:      result.Items,
			Total:      result.Total,
			Page:       result.Page,
			PageSize:   result.PageSize,
			TotalPages: result.TotalPages,
		})
	}
}

// ClustersGraphHandler returns the top biometric clusters as a
// person -- cluster -- trace graph (see cluster.BuildGraph). ?limit= caps how
// many clusters are drawn (default 40, max 300) and ?min_members= drops clusters with fewer samples.
func ClustersGraphHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		result, err := cluster.BuildGraph(context.Request.Context(), db, min(parsePositiveInt(context.Query("limit"), 40), 300), parsePositiveInt(context.Query("min_members"), 1))
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not build cluster graph"})
			return
		}
		context.JSON(http.StatusOK, result)
	}
}
