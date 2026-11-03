package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/requests"
	"github.com/rodfileto/trackid/storage"
	"github.com/rodfileto/trackid/video"
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

type registerRequest struct {
	requests.Input
	Occurrence *createOccurrenceRequest `json:"occurrence"` // to create, when the cited one is not in TrackID
	CaseType   string                   `json:"caseType"`   // new cases' nature when there is no occurrence
	Cases      []requests.CaseInput     `json:"cases"`
}

// RegisterRequestHandler is a coordinator registering a request as it arrives, with the
// occurrence it cites and the cases it opens or joins (requests.Register).
func RegisterRequestHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var body registerRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		in := requests.RegisterInput{Input: body.Input, CaseType: body.CaseType, Cases: body.Cases}
		if body.Occurrence != nil {
			o := body.Occurrence.input()
			in.Occurrence = &o
		}
		r, err := requests.Register(context.Request.Context(), sqlDB, in, currentUser(context).ID, time.Now())
		if !writeRequestError(context, err) {
			context.JSON(http.StatusCreated, r)
		}
	}
}

// ListRequestsHandler returns every request, soonest due first (requests.ListAll).
func ListRequestsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := requests.ListAll(context.Request.Context(), sqlDB, 500)
		if !writeRequestError(context, err) {
			context.JSON(http.StatusOK, list)
		}
	}
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

// ReceiveMaterialHandler is a coordinator receiving one file a request sent in: an image or PDF
// becomes an evidence of the case, a video a case video queued for processing, and either is
// attached to the request with its SHA-256. Receiving material is the coordinator's act, so this
// does not need an analysis of the case, unlike adding evidence while examining it.
func ReceiveMaterialHandler(sqlDB *sql.DB, store *storage.Client, queue *asynq.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID, err := strconv.ParseInt(context.Param("requestId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
			return
		}
		if store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "object storage is not configured"})
			return
		}
		ctx, caseID := context.Request.Context(), context.Param("caseId")
		if writeRequestError(context, requests.Check(ctx, sqlDB, caseID, requestID)) {
			return
		}
		file, err := context.FormFile("file")
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}
		if video.IsVideo(file.Filename) && file.Size > video.MaxBytes {
			context.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the video is too large"})
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
		var fileID int64
		if video.IsVideo(file.Filename) {
			var q embedding.Enqueuer
			if queue != nil {
				q = queue
			}
			v, err := video.Add(ctx, sqlDB, store, q, caseID, file.Filename, data)
			if writeVideoError(context, err) {
				return
			}
			fileID = v.FileID
		} else {
			f, err := cases.AddEvidence(ctx, sqlDB, store, caseID, cases.EvidenceFileInput{Filename: file.Filename, Data: data})
			var validation cases.ValidationError
			switch {
			case errors.As(err, &validation):
				context.JSON(http.StatusBadRequest, gin.H{"error": validation.Error()})
				return
			case err != nil:
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not add evidence"})
				return
			}
			fileID = f.ID
		}
		if writeRequestError(context, requests.AttachFiles(ctx, sqlDB, caseID, requestID, []int64{fileID})) {
			return
		}
		context.JSON(http.StatusCreated, gin.H{"fileId": fileID})
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
