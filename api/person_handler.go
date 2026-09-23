package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/person"
	"github.com/rodfileto/trackid/storage"
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

// SearchPersonsByFaceHandler finds enrolled persons whose enrolled face, and
// biometric cases whose evidence holds a matching face trace, most resemble
// the face in an uploaded photo (see person.SearchByFace), for a caller to
// disambiguate before opening a full profile or case. The image is sent as
// multipart form field "image" and is required.
func SearchPersonsByFaceHandler(db *sql.DB, vis cases.FaceVision) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		if vis == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "face search is not configured"})
			return
		}

		file, err := context.FormFile("image")
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "image is required"})
			return
		}
		opened, err := file.Open()
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read image"})
			return
		}
		defer opened.Close()
		data, err := io.ReadAll(opened)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read image"})
			return
		}

		results, err := person.SearchByFace(context.Request.Context(), db, vis, data)
		if err != nil {
			switch {
			case errors.Is(err, person.ErrUnsupportedImage):
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			case errors.Is(err, person.ErrNoFaceDetected):
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			default:
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not search persons by face"})
			}
			return
		}
		context.JSON(http.StatusOK, results)
	}
}

// GetPersonProfileHandler returns one person's full intelligence profile --
// their enrollment identity, the biometric clusters they've resolved into,
// and the biometric cases linked to them through those clusters (see
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

// DownloadIdentityFileHandler serves one identity_file's raw bytes (e.g. an
// enrollment photo, for a face thumbnail), scoped to the person named in the
// URL (see person.DownloadIdentityFile).
func DownloadIdentityFileHandler(db *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		if store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "object storage is not configured"})
			return
		}
		personID := context.Param("personId")
		identityFileID, err := strconv.ParseInt(context.Param("identityFileId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity file id"})
			return
		}

		content, err := person.DownloadIdentityFile(context.Request.Context(), db, store, personID, identityFileID)
		if err != nil {
			if errors.Is(err, person.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "identity file not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load identity file"})
			return
		}

		contentType := content.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		context.Header("Content-Type", contentType)
		context.Header("Content-Disposition", "inline; filename=\""+content.Filename+"\"")
		context.Data(http.StatusOK, contentType, content.Data)
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

// ListPersonCasesHandler returns every biometric case linked to one person
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
