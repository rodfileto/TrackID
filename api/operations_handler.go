package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/operations"
)

type createOperationRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateOperationRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// ListOperationsHandler lists the intelligence operations.
func ListOperationsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		list, err := operations.List(c.Request.Context(), sqlDB)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list operations"})
			return
		}
		c.JSON(http.StatusOK, list)
	}
}

// CreateOperationHandler registers an operation.
func CreateOperationHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		var r createOperationRequest
		if err := c.ShouldBindJSON(&r); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		created, err := operations.Create(c.Request.Context(), sqlDB, operations.CreateInput(r), currentUser(c).ID)
		if err != nil {
			writeOperationError(c, err, "could not create operation")
			return
		}
		c.JSON(http.StatusCreated, created)
	}
}

// GetOperationHandler returns an operation by its slug.
func GetOperationHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		operation, err := operations.Get(c.Request.Context(), sqlDB, c.Param("slug"))
		if err != nil {
			writeOperationError(c, err, "could not read operation")
			return
		}
		c.JSON(http.StatusOK, operation)
	}
}

// UpdateOperationHandler changes an operation's name, description and status.
func UpdateOperationHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sqlDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		var r updateOperationRequest
		if err := c.ShouldBindJSON(&r); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		updated, err := operations.Update(c.Request.Context(), sqlDB, c.Param("slug"), operations.UpdateInput(r))
		if err != nil {
			writeOperationError(c, err, "could not update operation")
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func writeOperationError(c *gin.Context, err error, fallback string) {
	var invalid operations.ValidationError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": invalid.Error()})
	case errors.Is(err, operations.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, operations.ErrSlugTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}
