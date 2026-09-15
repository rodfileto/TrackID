package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func healthHandler(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DatabaseHealthHandler reports whether the database connection is healthy.
func DatabaseHealthHandler(db *sql.DB) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if db == nil {
			ginContext.Status(http.StatusServiceUnavailable)
			return
		}

		requestContext, cancel := context.WithTimeout(ginContext.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(requestContext); err != nil {
			ginContext.Status(http.StatusServiceUnavailable)
			return
		}

		ginContext.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
