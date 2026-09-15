package api

import (
	"database/sql"
	"errors"
	"io"
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
