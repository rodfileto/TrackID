package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/person"
)

// ListClustersResponse is ListClustersHandler's paginated response.
type ListClustersResponse struct {
	Items      []cluster.Summary `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// ListClustersHandler returns every AUTOMATIC biometric cluster (an
// intelligence view: SYSTEM matches included), ordered by
// how many distinct biometric cases it touches (descending). ?identified=true
// keeps only clusters resolved to an enrolled identity (see
// cluster.ListOverview).
func ListClustersHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		page := parsePositiveInt(context.Query("page"), 1)
		pageSize := parsePositiveInt(context.Query("page_size"), 25)

		result, err := cluster.ListOverview(context.Request.Context(), sqlDB, cluster.ListOverviewParams{
			Page:           page,
			PageSize:       pageSize,
			IdentifiedOnly: context.Query("identified") == "true",
			ConflictOnly:   context.Query("conflict") == "true",
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
func ClustersGraphHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		result, err := cluster.BuildGraph(context.Request.Context(), sqlDB, min(parsePositiveInt(context.Query("limit"), 40), 300), parsePositiveInt(context.Query("min_members"), 1))
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not build cluster graph"})
			return
		}
		context.JSON(http.StatusOK, result)
	}
}

// OverviewGraphHandler returns the home page's illustration of how identities, clusters and case
// traces connect, to every signed-in user: the same graph as ClustersGraphHandler, anonymous
// (cluster.Anonymous), at a fixed size.
func OverviewGraphHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		result, err := cluster.BuildGraph(context.Request.Context(), sqlDB, 60, 2)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not build the overview"})
			return
		}
		context.JSON(http.StatusOK, cluster.Anonymous(result))
	}
}

// ClusterDetailResponse is GetClusterHandler's response: the cluster's structure and decisions
// (cluster.Detail) and its members resolved for display (person.ClusterMember: an enrollment's
// person and register, a trace's case and thumbnail).
type ClusterDetailResponse struct {
	cluster.Detail
	ResolvedMembers []person.ClusterMember `json:"resolvedMembers"`
	// Conflicts are the identity conflicts sitting in the cluster (cluster.ListConflicts).
	Conflicts []cluster.Conflict `json:"conflicts"`
}

// GetClusterHandler returns one cluster, of either layer, in depth (cluster.GetDetail): every
// decision touching it, what holds it together, its merges and the persons it resolves to, with
// their samples in other clusters. An intelligence view, like the clusters listing.
func GetClusterHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		clusterID, err := strconv.ParseInt(context.Param("clusterId"), 10, 64)
		if err != nil || clusterID <= 0 {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
			return
		}
		ctx := context.Request.Context()
		detail, err := cluster.GetDetail(ctx, sqlDB, clusterID)
		if errors.Is(err, cluster.ErrClusterNotFound) {
			context.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the cluster"})
			return
		}
		resolved, _, err := person.ClusterByID(ctx, sqlDB, clusterID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the cluster"})
			return
		}
		members := resolved.Members
		if members == nil {
			members = []person.ClusterMember{}
		}
		conflicts, err := cluster.ListConflicts(ctx, sqlDB)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the cluster"})
			return
		}
		ofCluster := cluster.ConflictsOfCluster(conflicts, clusterID)
		if err := cluster.AddRegisters(ctx, sqlDB, ofCluster); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the cluster"})
			return
		}
		context.JSON(http.StatusOK, ClusterDetailResponse{Detail: detail, ResolvedMembers: members, Conflicts: ofCluster})
	}
}

// PersonIdentityConflictsHandler returns the identity conflicts involving a person, for the person
// page. Reading them is reading the person, and is logged as such (router).
func PersonIdentityConflictsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		ctx := context.Request.Context()
		row, err := db.New(sqlDB).GetPersonByPersonID(ctx, context.Param("personId"))
		if errors.Is(err, sql.ErrNoRows) {
			context.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the person"})
			return
		}
		conflicts, err := cluster.ListConflicts(ctx, sqlDB)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the conflicts"})
			return
		}
		ofPerson := cluster.ConflictsOfPerson(conflicts, row.ID)
		if err := cluster.AddRegisters(ctx, sqlDB, ofPerson); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the conflicts"})
			return
		}
		// The comparison shows the other people's registers too, and each of them has been read. The
		// route's own log covers this person.
		logged := map[string]bool{context.Param("personId"): true}
		for _, c := range ofPerson {
			for _, p := range c.Persons {
				if !logged[p.PersonID] {
					logged[p.PersonID] = true
					if !recordAccess(context, sqlDB, accessPerson, p.PersonID) {
						return
					}
				}
			}
		}
		context.JSON(http.StatusOK, gin.H{"conflicts": ofPerson})
	}
}
