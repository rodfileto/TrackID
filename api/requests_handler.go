package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/requests"
)

// writeRequestError writes the response for a requests error and reports whether there was one.
func writeRequestError(context *gin.Context, err error) bool {
	var validation requests.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, requests.ErrCaseNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.As(err, &validation):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not record the request"})
	}
	return true
}

// ListCaseRequestsHandler returns a case's requests, each with its questions and files.
func ListCaseRequestsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := requests.List(context.Request.Context(), sqlDB, context.Param("caseId"))
		if !writeRequestError(context, err) {
			context.JSON(http.StatusOK, list)
		}
	}
}

// CreateCaseRequestHandler is a coordinator registering a request on a case (requests.Create).
func CreateCaseRequestHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var in requests.Input
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		r, err := requests.Create(context.Request.Context(), sqlDB, context.Param("caseId"), in, currentUser(context).ID)
		if !writeRequestError(context, err) {
			context.JSON(http.StatusCreated, r)
		}
	}
}

type attachFilesRequest struct {
	FileIDs []int64 `json:"fileIds" binding:"required"`
}

// AttachRequestFilesHandler is a coordinator marking more evidence files as sent in by a request.
func AttachRequestFilesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID, err := strconv.ParseInt(context.Param("requestId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
			return
		}
		var body attachFilesRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid files"})
			return
		}
		caseID := context.Param("caseId")
		if writeRequestError(context, requests.AttachFiles(context.Request.Context(), sqlDB, caseID, requestID, body.FileIDs)) {
			return
		}
		list, err := requests.List(context.Request.Context(), sqlDB, caseID)
		if !writeRequestError(context, err) {
			context.JSON(http.StatusOK, list)
		}
	}
}

// ListCaseMaterialHandler returns every trace of a case with its role and suitability.
func ListCaseMaterialHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := cases.ListMaterial(context.Request.Context(), sqlDB, context.Param("caseId"))
		switch {
		case errors.Is(err, cases.ErrNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the material"})
		default:
			context.JSON(http.StatusOK, list)
		}
	}
}

type traceRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// SetTraceRoleHandler marks a trace as questioned or reference material (cases.SetTraceRole).
func SetTraceRoleHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		traceID, err := strconv.ParseInt(context.Param("traceId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid trace id"})
			return
		}
		var body traceRoleRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
			return
		}
		var validation cases.ValidationError
		err = cases.SetTraceRole(context.Request.Context(), sqlDB, context.Param("caseId"), traceID, body.Role)
		switch {
		case errors.Is(err, cases.ErrNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
		case errors.As(err, &validation):
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, cases.ErrRoleLocked):
			context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not set the role"})
		default:
			context.JSON(http.StatusOK, gin.H{"saved": true})
		}
	}
}
