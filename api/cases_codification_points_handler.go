package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
)

func traceIDParam(context *gin.Context) (int64, bool) {
	traceID, err := strconv.ParseInt(context.Param("traceId"), 10, 64)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid trace id"})
		return 0, false
	}
	return traceID, true
}

func ListPointsHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
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

		points, err := cases.ListPoints(context.Request.Context(), db, caseID, evidenceID, traceID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			if errors.Is(err, cases.ErrNoCodification) || errors.Is(err, cases.ErrPointsNotSupported) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list points"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"points": points})
	}
}

// pointInput is one point in an AddPointsHandler/UpdatePointHandler request
// body. Coordinates aren't tagged "required" -- gin's validator treats a
// zero value as absent, which would wrongly reject a point legitimately
// touching the image's origin edge (x/y == 0).
type pointInput struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	PointType string  `json:"pointType"`
	Angle     float64 `json:"angle"`
}

func (p pointInput) toDomain() cases.PointInput {
	return cases.PointInput{X: p.X, Y: p.Y, PointType: p.PointType, Angle: p.Angle}
}

func AddPointsHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
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

		var body struct {
			Points []pointInput `json:"points" binding:"required,min=1"`
		}
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "at least one point with x/y is required"})
			return
		}

		inputs := make([]cases.PointInput, 0, len(body.Points))
		for _, p := range body.Points {
			inputs = append(inputs, p.toDomain())
		}

		points, err := cases.AddPoints(context.Request.Context(), db, caseID, evidenceID, traceID, inputs)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			if errors.Is(err, cases.ErrNoCodification) || errors.Is(err, cases.ErrPointsNotSupported) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not add points"})
			return
		}
		context.JSON(http.StatusCreated, gin.H{"points": points})
	}
}

func UpdatePointHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
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
		pointID, err := strconv.ParseInt(context.Param("pointId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid point id"})
			return
		}

		var body pointInput
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "x/y are required"})
			return
		}

		err = cases.UpdatePoint(context.Request.Context(), db, caseID, evidenceID, traceID, pointID, body.toDomain())
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "point not found"})
				return
			}
			if errors.Is(err, cases.ErrNoCodification) || errors.Is(err, cases.ErrPointsNotSupported) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not update point"})
			return
		}
		context.JSON(http.StatusOK, gin.H{})
	}
}

func DeletePointHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
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
		pointID, err := strconv.ParseInt(context.Param("pointId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid point id"})
			return
		}

		err = cases.DeletePoint(context.Request.Context(), db, caseID, evidenceID, traceID, pointID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "point not found"})
				return
			}
			if errors.Is(err, cases.ErrNoCodification) || errors.Is(err, cases.ErrPointsNotSupported) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete point"})
			return
		}
		context.JSON(http.StatusOK, gin.H{})
	}
}
