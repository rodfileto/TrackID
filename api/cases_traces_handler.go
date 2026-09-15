package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
)

func ListTracesHandler(db *sql.DB) gin.HandlerFunc {
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

		traces, err := cases.ListTraces(context.Request.Context(), db, caseID, evidenceID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "evidence not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list traces"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"traces": traces})
	}
}

// traceInput is one box in a CreateTracesHandler request body. Coordinates
// aren't tagged "required" -- gin's validator treats a zero value as absent,
// which would wrongly reject a box legitimately touching the image's origin
// edge (boxX1/boxY1 == 0).
type traceInput struct {
	BoxX1 float64 `json:"boxX1"`
	BoxY1 float64 `json:"boxY1"`
	BoxX2 float64 `json:"boxX2"`
	BoxY2 float64 `json:"boxY2"`
	Score float64 `json:"score"`
}

func CreateTracesHandler(db *sql.DB) gin.HandlerFunc {
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

		var body struct {
			Traces []traceInput `json:"traces" binding:"required,min=1"`
		}
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "at least one trace with boxX1/boxY1/boxX2/boxY2 is required"})
			return
		}

		detections := make([]cases.TraceDetection, 0, len(body.Traces))
		for _, t := range body.Traces {
			detections = append(detections, cases.TraceDetection{
				BoxX1: t.BoxX1,
				BoxY1: t.BoxY1,
				BoxX2: t.BoxX2,
				BoxY2: t.BoxY2,
				Score: t.Score,
			})
		}

		traces, err := cases.CreateTraces(context.Request.Context(), db, caseID, evidenceID, detections)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "evidence not found"})
				return
			}
			if errors.Is(err, cases.ErrUnsupportedCaseType) || errors.Is(err, cases.ErrNotImage) {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not create traces"})
			return
		}
		context.JSON(http.StatusCreated, gin.H{"traces": traces})
	}
}

func DeleteTraceHandler(db *sql.DB) gin.HandlerFunc {
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
		traceID, err := strconv.ParseInt(context.Param("traceId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid trace id"})
			return
		}

		err = cases.DeleteTrace(context.Request.Context(), db, caseID, evidenceID, traceID)
		if err != nil {
			if errors.Is(err, cases.ErrNotFound) {
				context.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete trace"})
			return
		}
		context.JSON(http.StatusOK, gin.H{})
	}
}
