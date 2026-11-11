package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/analyses"
)

// The first examiner of a fingerprint examination lists its pairs while examining
// (docs/scene-examination-plan.md, decision 6): a lift marked on the case's evidence against a
// reference marked or brought in.

func writeExaminerPairsError(context *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, analyses.ErrAnalysisNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, analyses.ErrCannotDecide), errors.Is(err, analyses.ErrNotExaminerPairs):
		context.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, analyses.ErrInvalidPair), errors.Is(err, analyses.ErrNotComparison):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, analyses.ErrPairsClosed), errors.Is(err, analyses.ErrPairDecided):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not change the pairs"})
	}
	return true
}

// AddExaminerPairsHandler adds pairs ({"pairs": [{featureA, featureB}]}) and returns the worksheet.
func AddExaminerPairsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var request addPairsRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "at least one pair is required"})
			return
		}
		id, err := strconv.ParseInt(context.Param("analysisId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid analysis id"})
			return
		}
		err = analyses.AddExaminerPairs(context.Request.Context(), sqlDB, context.Param("caseId"), id, currentUser(context), request.Pairs)
		if !writeExaminerPairsError(context, err) {
			context.Status(http.StatusNoContent)
		}
	}
}

// RemoveExaminerPairHandler takes back one pair ({featureA, featureB}) nothing was decided on.
func RemoveExaminerPairHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var pair analyses.Pair
		if err := context.ShouldBindJSON(&pair); err != nil || pair.FeatureA == "" || pair.FeatureB == "" {
			context.JSON(http.StatusBadRequest, gin.H{"error": "the pair is required"})
			return
		}
		id, err := strconv.ParseInt(context.Param("analysisId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid analysis id"})
			return
		}
		err = analyses.RemoveExaminerPair(context.Request.Context(), sqlDB, context.Param("caseId"), id, currentUser(context), pair)
		if !writeExaminerPairsError(context, err) {
			context.Status(http.StatusNoContent)
		}
	}
}
