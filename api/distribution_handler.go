package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/distribution"
)

// DistributionStatusHandler reports pending analyses per type and each examiner's load.
func DistributionStatusHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		status, err := distribution.GetStatus(context.Request.Context(), sqlDB)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load distribution status"})
			return
		}
		context.JSON(http.StatusOK, status)
	}
}

// DistributeHandler assigns up to count pending analyses automatically (distribution.Distribute);
// dryRun previews it without recording anything.
func DistributeHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var request distribution.Request
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid distribution request"})
			return
		}
		result, err := distribution.Distribute(context.Request.Context(), sqlDB, request)
		if errors.Is(err, distribution.ErrInvalidCount) || errors.Is(err, analyses.ErrInvalidType) {
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not distribute analyses"})
			return
		}
		context.JSON(http.StatusOK, result)
	}
}

type openAnalysisRequest struct {
	Type        analyses.Type   `json:"type"`
	Pairs       []analyses.Pair `json:"pairs"`
	RequestID   int64           `json:"requestId"`   // the request it answers; 0 the case's latest
	Complements int64           `json:"complements"` // the analysis whose issued report this one complements
}

// OpenAnalysisHandler is a coordinator opening an analysis on a case (analyses.Open): a comparison,
// on the given pairs or none yet, or a review. It starts pending.
func OpenAnalysisHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var request openAnalysisRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid analysis"})
			return
		}
		caseID := context.Param("caseId")
		_, err := analyses.OpenWith(context.Request.Context(), sqlDB, caseID, request.Type, request.Pairs, currentUser(context).ID,
			analyses.Options{RequestID: request.RequestID, Complements: request.Complements})
		switch {
		case errors.Is(err, analyses.ErrCaseNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, analyses.ErrInvalidType), errors.Is(err, analyses.ErrWrongModality), errors.Is(err, analyses.ErrNotOffered),
			errors.Is(err, analyses.ErrUnexpectedPairs), errors.Is(err, analyses.ErrInvalidPair),
			errors.Is(err, analyses.ErrUnknownRequest), errors.Is(err, analyses.ErrBadComplement):
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not open analysis"})
		default:
			writeCaseAnalyses(context, sqlDB, caseID, http.StatusCreated)
		}
	}
}

type addPairsRequest struct {
	Pairs []analyses.Pair `json:"pairs" binding:"required,min=1"`
}

// AddPairsHandler is a coordinator adding pairs to a comparison at its examination stage
// (analyses.AddPairs): from what the request sent, or from the matcher's candidates on the case.
func AddPairsHandler(sqlDB *sql.DB) gin.HandlerFunc {
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
		caseID := context.Param("caseId")
		err = analyses.AddPairs(context.Request.Context(), sqlDB, caseID, id, request.Pairs)
		switch {
		case errors.Is(err, analyses.ErrAnalysisNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, analyses.ErrNotComparison), errors.Is(err, analyses.ErrInvalidPair):
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, analyses.ErrPairsClosed):
			context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not add the pairs"})
		default:
			writeCaseAnalyses(context, sqlDB, caseID, http.StatusOK)
		}
	}
}

// ListCaseAnalysesHandler returns a case's analyses with their examiner, pairs and report.
func ListCaseAnalysesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		writeCaseAnalyses(context, sqlDB, context.Param("caseId"), http.StatusOK)
	}
}

func writeCaseAnalyses(context *gin.Context, sqlDB *sql.DB, caseID string, status int) {
	list, err := analyses.List(context.Request.Context(), sqlDB, caseID)
	if errors.Is(err, analyses.ErrCaseNotFound) {
		context.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
		return
	}
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list analyses"})
		return
	}
	context.JSON(status, list)
}

// caseAnalysis reads the :analysisId of a cases/:caseId route and checks that it belongs to that
// case, writing the error response itself when it does not (ok false).
func caseAnalysis(context *gin.Context, sqlDB *sql.DB) (analyses.Target, bool) {
	id, err := strconv.ParseInt(context.Param("analysisId"), 10, 64)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid analysis id"})
		return analyses.Target{}, false
	}
	target, err := analyses.GetTarget(context.Request.Context(), db.New(sqlDB), id)
	if errors.Is(err, analyses.ErrAnalysisNotFound) || (err == nil && target.CaseID != context.Param("caseId")) {
		context.JSON(http.StatusNotFound, gin.H{"error": "analysis not found"})
		return analyses.Target{}, false
	}
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load analysis"})
		return analyses.Target{}, false
	}
	return target, true
}

// AnalysisAssignmentsHandler returns an analysis's assignment history, oldest first; the last is
// current.
func AnalysisAssignmentsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		history, err := distribution.History(context.Request.Context(), sqlDB, target.ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load assignment history"})
			return
		}
		context.JSON(http.StatusOK, history)
	}
}

type assignAnalysisRequest struct {
	// UserID is the new examiner; null unassigns the analysis back to pending.
	UserID *int64 `json:"userId"`
}

// AssignAnalysisHandler is a coordinator's manual assignment of the current step of one analysis
// (distribution.Assign).
func AssignAnalysisHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, err := strconv.ParseInt(context.Param("analysisId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid analysis id"})
			return
		}
		var request assignAnalysisRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment"})
			return
		}
		err = distribution.Assign(context.Request.Context(), sqlDB, id, request.UserID, currentUser(context).ID)
		switch {
		case errors.Is(err, analyses.ErrAnalysisNotFound), errors.Is(err, distribution.ErrUserNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, distribution.ErrNotExaminer), errors.Is(err, distribution.ErrOtherStep):
			context.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		case errors.Is(err, distribution.ErrUnchanged), errors.Is(err, distribution.ErrClosed):
			context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not assign analysis"})
		default:
			history, err := distribution.History(context.Request.Context(), sqlDB, id)
			if err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load assignment history"})
				return
			}
			context.JSON(http.StatusOK, history)
		}
	}
}

// PriorDecidersHandler lists the user ids of those who have already worked on the pairs the
// analysis's current step decides (distribution.PriorDeciders), so the assignment screen can warn a
// coordinator before giving the step to one of them.
func PriorDecidersHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, err := strconv.ParseInt(context.Param("analysisId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid analysis id"})
			return
		}
		ids, err := distribution.PriorDeciders(context.Request.Context(), sqlDB, id)
		switch {
		case errors.Is(err, analyses.ErrAnalysisNotFound):
			context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list prior deciders"})
		default:
			context.JSON(http.StatusOK, gin.H{"userIds": ids})
		}
	}
}

// MyAnalysesHandler lists the analyses the signed-in user is currently responsible for.
func MyAnalysesHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		mine, err := distribution.AnalysesOf(context.Request.Context(), sqlDB, currentUser(context).ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list your analyses"})
			return
		}
		context.JSON(http.StatusOK, mine)
	}
}
