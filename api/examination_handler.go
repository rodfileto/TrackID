package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/examination"
	"github.com/rodfileto/trackid/storage"
)

// AnalysisWorksheetHandler returns an analysis's pairs as the signed-in user sees them
// (examination.GetWorksheet): their step's pairs and drafts when it is their turn, blind to the
// first examiner's decisions while they verify.
func AnalysisWorksheetHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		ws, err := examination.GetWorksheet(context.Request.Context(), sqlDB, currentUser(context), target.ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the analysis's pairs"})
			return
		}
		context.JSON(http.StatusOK, ws)
	}
}

type saveDraftRequest struct {
	FeatureA string `json:"featureA" binding:"required"`
	FeatureB string `json:"featureB" binding:"required"`
	Decision string `json:"decision" binding:"required"`
	Notes    string `json:"notes"`
}

// SaveDraftHandler records the signed-in examiner's draft decision on one pair
// (examination.SaveDraft); it can be changed until they submit.
func SaveDraftHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		var request saveDraftRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid decision"})
			return
		}
		err := examination.SaveDraft(context.Request.Context(), sqlDB, currentUser(context), target.ID,
			examination.Pair{FeatureA: request.FeatureA, FeatureB: request.FeatureB}, request.Decision, request.Notes)
		if !writeExaminationError(context, err) {
			context.JSON(http.StatusOK, gin.H{"saved": true})
		}
	}
}

// SubmitStepHandler submits the signed-in examiner's step (examination.Submit) and has clusters
// reconciled with the new decisions.
func SubmitStepHandler(sqlDB *sql.DB, queue *asynq.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		result, err := examination.Submit(context.Request.Context(), sqlDB, currentUser(context), target.ID)
		if writeExaminationError(context, err) {
			return
		}
		if queue != nil && result.Decisions > 0 {
			embedding.EnqueueSyncFace(queue)
		}
		context.JSON(http.StatusOK, result)
	}
}

// writeExaminationError writes the response for an examination error and reports whether there
// was one.
func writeExaminationError(context *gin.Context, err error) bool {
	var incomplete examination.IncompleteError
	switch {
	case err == nil:
		return false
	case errors.Is(err, analyses.ErrCannotDecide):
		context.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.As(err, &incomplete), errors.Is(err, examination.ErrNothingToSubmit):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, examination.ErrInvalidDecision), errors.Is(err, examination.ErrNotInScope):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not record the decision"})
	}
	return true
}

// PairImageHandler returns the image of one face of the analysis's pairs (examination.PairImage),
// named by its feature id: how the workspace shows a pair, blind or not.
func PairImageHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "object storage is not configured"})
			return
		}
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		img, err := examination.PairImage(context.Request.Context(), sqlDB, store, currentUser(context), target.ID, context.Query("feature"))
		switch {
		case errors.Is(err, examination.ErrNotInScope), errors.Is(err, examination.ErrNoImage):
			context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the image"})
			return
		}
		contentType := img.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		context.Header("Cache-Control", "private, no-store")
		context.Data(http.StatusOK, contentType, img.Data)
	}
}
