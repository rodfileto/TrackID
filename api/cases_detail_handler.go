package api

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/storage"
)

func GetCaseHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		caseID := context.Param("caseId")

		detail, err := cases.Get(context.Request.Context(), db, caseID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load case"})
			return
		}
		context.JSON(http.StatusOK, detail)
	}
}

// ListCaseCodificationsHandler returns every codification recorded across
// every trace of a case -- e.g. every face codified in a FACIAL case -- so a
// caller can render them all in one view without a request per trace.
func ListCaseCodificationsHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		caseID := context.Param("caseId")

		codifications, err := cases.ListCaseCodifications(context.Request.Context(), db, caseID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list codifications"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"codifications": codifications})
	}
}

// ListCaseClustersHandler returns the distinct biometric clusters a case's
// codified traces have resolved into, and the other cases/persons each one
// is CONFIRMED-linked to (see cases.ListCaseClusters).
func ListCaseClustersHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		caseID := context.Param("caseId")

		clusters, err := cases.ListCaseClusters(context.Request.Context(), db, caseID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load case clusters"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"clusters": clusters})
	}
}

type compareFacesRequest struct {
	CodificationIDs []int64 `json:"codificationIds" binding:"required,len=2"`
}

// CompareFacesHandler returns the machine similarity score between two face
// codifications of a case (see cases.CompareFaces).
func CompareFacesHandler(db *sql.DB, store *storage.Client, vis cases.FaceVision) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		var request compareFacesRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "codificationIds must hold exactly two codification ids"})
			return
		}
		caseID := context.Param("caseId")

		comparison, err := cases.CompareFaces(
			context.Request.Context(), db, store, vis, caseID,
			[2]int64{request.CodificationIDs[0], request.CodificationIDs[1]},
		)
		if err != nil {
			var noFace cases.NoFaceError
			switch {
			case errors.As(err, &noFace):
				context.JSON(http.StatusUnprocessableEntity, gin.H{
					"error":          "no face detected in one of the codifications",
					"codificationId": noFace.CodificationID,
				})
			case errors.Is(err, cases.ErrNotFound):
				context.JSON(http.StatusNotFound, gin.H{"error": "codification not found in this case"})
			case errors.Is(err, cases.ErrNotFaceCodification):
				context.JSON(http.StatusBadRequest, gin.H{"error": "only face codifications can be compared"})
			case errors.Is(err, cases.ErrUnsupportedImage):
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			case errors.Is(err, cases.ErrVisionUnavailable):
				context.JSON(http.StatusServiceUnavailable, gin.H{"error": "face recognition is not configured, and one of the faces has no stored embedding"})
			default:
				log.Printf("compare faces (case %s, codifications %v): %v", caseID, request.CodificationIDs, err)
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not compare faces"})
			}
			return
		}
		context.JSON(http.StatusOK, comparison)
	}
}

func DownloadEvidenceHandler(db *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		if store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "object storage is not configured"})
			return
		}
		caseID := context.Param("caseId")
		evidenceID, err := strconv.ParseInt(context.Param("evidenceId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid evidence id"})
			return
		}

		content, err := cases.DownloadEvidence(context.Request.Context(), db, store, caseID, evidenceID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "evidence not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load evidence"})
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

func AddEvidenceHandler(db *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		if store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "object storage is not configured"})
			return
		}
		caseID := context.Param("caseId")

		file, err := context.FormFile("file")
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}

		opened, err := file.Open()
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read file"})
			return
		}
		defer opened.Close()

		data, err := io.ReadAll(opened)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read file"})
			return
		}

		evidence, err := cases.AddEvidence(context.Request.Context(), db, store, caseID, cases.EvidenceFileInput{
			Filename: file.Filename,
			Data:     data,
		})
		if err != nil {
			var validationErr cases.ValidationError
			if errors.As(err, &validationErr) {
				context.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
				return
			}
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not add evidence"})
			return
		}
		context.JSON(http.StatusCreated, evidence)
	}
}

func DeleteEvidenceHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		caseID := context.Param("caseId")
		evidenceID, err := strconv.ParseInt(context.Param("evidenceId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid evidence id"})
			return
		}

		err = cases.DeleteEvidence(context.Request.Context(), db, caseID, evidenceID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "evidence not found"})
				return
			}
			if errors.Is(err, cases.ErrEvidenceHasTraces) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete evidence"})
			return
		}
		context.JSON(http.StatusOK, gin.H{})
	}
}
