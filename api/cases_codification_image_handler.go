package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/storage"
)

// SaveCodificationImageHandler stores the rendered result of manually
// adjusting a trace's codification image (crop + brightness/contrast/
// saturation/interpolation -- see CodificationEditorModal) as a case_files
// row linked to the codification. When queue is non-nil, saving also
// enqueues the codification's embedding computation (see
// cases.SaveCodificationImage).
func SaveCodificationImageHandler(db *sql.DB, store *storage.Client, queue *asynq.Client) gin.HandlerFunc {
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
		traceID, ok := traceIDParam(context)
		if !ok {
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

		// A nil *asynq.Client boxed into the embedding.Enqueuer interface would
		// be a non-nil interface wrapping a nil pointer, so this explicit check
		// is what actually gives cases.SaveCodificationImage a nil Enqueuer when
		// queueing isn't configured.
		var enqueuer embedding.Enqueuer
		if queue != nil {
			enqueuer = queue
		}

		saved, err := cases.SaveCodificationImage(context.Request.Context(), db, store, enqueuer, caseID, evidenceID, traceID, data)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			if errors.Is(err, cases.ErrNoCodification) || errors.Is(err, cases.ErrNotImage) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not save codification image"})
			return
		}
		context.JSON(http.StatusCreated, saved)
	}
}
