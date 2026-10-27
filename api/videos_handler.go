package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/storage"
	"github.com/rodfileto/trackid/video"
)

// writeVideoError writes the response for a video error and reports whether there was one.
func writeVideoError(context *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, video.ErrCaseNotFound), errors.Is(err, video.ErrNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, video.ErrNotVideo), errors.Is(err, video.ErrOutOfRange):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, video.ErrNotProcessed):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, video.ErrNoStore):
		context.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not process the video"})
	}
	return true
}

// ListVideosHandler returns a case's videos with their tracks.
func ListVideosHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := video.List(context.Request.Context(), sqlDB, context.Param("caseId"))
		if !writeVideoError(context, err) {
			context.JSON(http.StatusOK, list)
		}
	}
}

// AddVideoHandler stores a video sent in (multipart field "file") and queues its processing.
func AddVideoHandler(sqlDB *sql.DB, store *storage.Client, queue *asynq.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		file, err := context.FormFile("file")
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}
		if file.Size > video.MaxBytes {
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
		var q embedding.Enqueuer
		if queue != nil {
			q = queue
		}
		v, err := video.Add(context.Request.Context(), sqlDB, store, q, context.Param("caseId"), file.Filename, data)
		if !writeVideoError(context, err) {
			context.JSON(http.StatusCreated, v)
		}
	}
}

type extractFrameRequest struct {
	TimestampMs int64       `json:"timestampMs"`
	Box         *[4]float32 `json:"box"`
}

// ExtractFrameHandler takes a frame of a processed video as a new evidence of the case.
func ExtractFrameHandler(sqlDB *sql.DB, store *storage.Client, queue *asynq.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		fileID, err := strconv.ParseInt(context.Param("fileId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
			return
		}
		var body extractFrameRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid frame"})
			return
		}
		var q embedding.Enqueuer
		if queue != nil {
			q = queue
		}
		out, err := video.Extract(context.Request.Context(), sqlDB, store, q, context.Param("caseId"), fileID, body.TimestampMs, body.Box)
		if !writeVideoError(context, err) {
			context.JSON(http.StatusCreated, out)
		}
	}
}
