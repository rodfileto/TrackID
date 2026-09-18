package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/person"
)

// SearchPersonsHandler finds enrolled persons by name (see
// person.SearchByName), for a caller to disambiguate before opening a full
// profile. The "name" query parameter is required; an empty/missing one
// returns 400 rather than the whole table.
func SearchPersonsHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		name := context.Query("name")
		if strings.TrimSpace(name) == "" {
			context.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}

		results, err := person.SearchByName(context.Request.Context(), db, name)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not search persons"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// GetPersonProfileHandler returns one person's full intelligence profile --
// their enrollment identity, the biometric clusters they've resolved into,
// and the criminal cases linked to them through those clusters (see
// person.GetProfile). This is the single call a person-profile panel needs;
// GetPersonIdentityHandler/ListPersonClustersHandler/ListPersonCasesHandler
// below expose the same three sections individually for callers that only
// need one.
func GetPersonProfileHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		personID := context.Param("personId")

		profile, err := person.GetProfile(context.Request.Context(), db, personID)
		if err != nil {
			if errors.Is(err, person.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load person profile"})
			return
		}
		context.JSON(http.StatusOK, profile)
	}
}

// GetPersonIdentityHandler returns one person's enrollment chain -- their
// identity documents, registers, and enrolled biometric files/features (see
// person.GetIdentity).
func GetPersonIdentityHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		personID := context.Param("personId")

		identity, err := person.GetIdentity(context.Request.Context(), db, personID)
		if err != nil {
			if errors.Is(err, person.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load person identity"})
			return
		}
		context.JSON(http.StatusOK, identity)
	}
}

// ListPersonClustersHandler returns every biometric cluster resolved to one
// person (see person.ListClusters).
func ListPersonClustersHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		personID := context.Param("personId")

		clusters, err := person.ListClusters(context.Request.Context(), db, personID)
		if err != nil {
			if errors.Is(err, person.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list person clusters"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"clusters": clusters})
	}
}

// ListPersonCasesHandler returns every criminal case linked to one person
// through their resolved biometric clusters (see person.ListCases).
func ListPersonCasesHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		personID := context.Param("personId")

		relatedCases, err := person.ListCases(context.Request.Context(), db, personID)
		if err != nil {
			if errors.Is(err, person.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list person cases"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"cases": relatedCases})
	}
}
