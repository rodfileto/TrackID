package api

import (
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/requests"
	"github.com/rodfileto/trackid/storage"
)

// ReceiveRequestDocumentHandler keeps the request's own document (multipart field "file") -- its
// ePol task or ofício -- with one of the cases it opened or joined. Not an evidence.
func ReceiveRequestDocumentHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil || store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			return
		}
		requestID, err := strconv.ParseInt(context.Param("requestId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
			return
		}
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
		added, err := requests.AddDocument(context.Request.Context(), sqlDB, store, context.Param("caseId"), requestID, file.Filename, data)
		var validation requests.ValidationError
		switch {
		case errors.As(err, &validation):
			context.JSON(http.StatusBadRequest, gin.H{"error": validation.Error()})
		case errors.Is(err, requests.ErrCaseNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not keep the document"})
		default:
			context.JSON(http.StatusCreated, added)
		}
	}
}

// RequestDocumentHandler serves a request document of the case.
func RequestDocumentHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil || store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			return
		}
		id, err := strconv.ParseInt(context.Param("fileId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
			return
		}
		doc, err := requests.Document(context.Request.Context(), sqlDB, store, context.Param("caseId"), id)
		switch {
		case errors.Is(err, requests.ErrCaseNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		case err != nil:
			context.JSON(http.StatusBadGateway, gin.H{"error": "could not read the document"})
		default:
			context.Header("Cache-Control", "private, no-store")
			context.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": doc.Filename}))
			context.Data(http.StatusOK, doc.ContentType, doc.Data)
		}
	}
}
