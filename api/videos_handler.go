package api

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/collections"
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
	case errors.Is(err, video.ErrNotProcessed), errors.Is(err, video.ErrBusy):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, video.ErrNoStore), errors.Is(err, video.ErrNoQueue):
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
		caseID := context.Param("caseId")
		origin, ok := materialOrigin(context, sqlDB, caseID, collections.Origin{})
		if !ok {
			return
		}
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
		v, err := video.Add(context.Request.Context(), sqlDB, store, q, caseID, file.Filename, data)
		if writeVideoError(context, err) {
			return
		}
		if writeCollectionError(context, collections.Attach(context.Request.Context(), sqlDB, caseID, v.FileID, origin)) {
			return
		}
		context.JSON(http.StatusCreated, v)
	}
}

type extractFrameRequest struct {
	TimestampMs int64            `json:"timestampMs"`
	Faces       []video.TraceBox `json:"faces"`
	// Box is the single face of the older form of the request.
	Box *[4]float32 `json:"box"`
}

func videoFileID(context *gin.Context) (int64, bool) {
	fileID, err := strconv.ParseInt(context.Param("fileId"), 10, 64)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return 0, false
	}
	return fileID, true
}

// ExtractFrameHandler takes a frame of a processed video as a new evidence of the case, with the
// faces chosen marked on it as traces.
func ExtractFrameHandler(sqlDB *sql.DB, store *storage.Client, queue *asynq.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		fileID, ok := videoFileID(context)
		if !ok {
			return
		}
		var body extractFrameRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid frame"})
			return
		}
		faces := body.Faces
		if len(faces) == 0 && body.Box != nil {
			faces = []video.TraceBox{{Box: *body.Box}}
		}
		if len(faces) > video.MaxTracesPerFrame {
			context.JSON(http.StatusBadRequest, gin.H{"error": "too many faces for one frame"})
			return
		}
		for _, f := range faces {
			if !(f.Box[2] > f.Box[0] && f.Box[3] > f.Box[1]) {
				context.JSON(http.StatusBadRequest, gin.H{"error": "invalid face box"})
				return
			}
		}
		var q embedding.Enqueuer
		if queue != nil {
			q = queue
		}
		out, err := video.Extract(context.Request.Context(), sqlDB, store, q, context.Param("caseId"), fileID, body.TimestampMs, faces)
		if !writeVideoError(context, err) {
			context.JSON(http.StatusCreated, out)
		}
	}
}

// VideoContentHandler serves a case's video for the page to play, with range requests.
func VideoContentHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		fileID, ok := videoFileID(context)
		if !ok {
			return
		}
		opened, err := video.Open(context.Request.Context(), sqlDB, store, context.Param("caseId"), fileID)
		if writeVideoError(context, err) {
			return
		}
		defer opened.Close()
		f, err := os.Open(opened.Path)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read the video"})
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read the video"})
			return
		}
		context.Header("Content-Type", opened.ContentType)
		context.Header("Cache-Control", "private, no-store")
		http.ServeContent(context.Writer, context.Request, "", info.ModTime(), f)
	}
}

type frameFacesRequest struct {
	TimestampMs int64 `json:"timestampMs"`
}

// FrameFacesHandler returns the frame at a time of a processed video with the faces found in it,
// each matched to a person of the video when it is one.
func FrameFacesHandler(sqlDB *sql.DB, store *storage.Client, vis cases.FaceVision) gin.HandlerFunc {
	var detect func([]byte) ([]vision.Detection, error)
	if vis != nil {
		detect = func(img []byte) ([]vision.Detection, error) { return vis.DetectFaces(bytes.NewReader(img)) }
	}
	return func(context *gin.Context) {
		fileID, ok := videoFileID(context)
		if !ok {
			return
		}
		var body frameFacesRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid frame"})
			return
		}
		frame, err := video.FrameFaces(context.Request.Context(), sqlDB, store, detect, context.Param("caseId"), fileID, body.TimestampMs)
		if !writeVideoError(context, err) {
			context.JSON(http.StatusOK, frame)
		}
	}
}

// ReprocessVideoHandler queues a case's video to be processed again.
func ReprocessVideoHandler(sqlDB *sql.DB, queue *asynq.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		fileID, ok := videoFileID(context)
		if !ok {
			return
		}
		var q embedding.Enqueuer
		if queue != nil {
			q = queue
		}
		err := video.Reprocess(context.Request.Context(), sqlDB, q, context.Param("caseId"), fileID)
		if !writeVideoError(context, err) {
			context.Status(http.StatusAccepted)
		}
	}
}
