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

// ReadyHandler reports whether the API can serve: the database, object storage and the task queue
// each answer (GET /ready). Liveness stays /health, which needs none of them. A dependency that
// isn't configured is reported but doesn't fail the check, since development runs without some;
// production refuses to start without them (config.CheckProduction).
func ReadyHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		status, ready := gin.H{}, true
		check := func(name string, configured bool, ping func() error) {
			switch {
			case !configured:
				status[name] = "not configured"
			case ping() != nil:
				status[name], ready = "unavailable", false
			default:
				status[name] = "ok"
			}
		}
		check("database", deps.DB != nil, func() error { return deps.DB.PingContext(ctx) })
		if deps.DB == nil {
			ready = false
		}
		check("storage", deps.Storage != nil, func() error { return deps.Storage.Ping(ctx) })
		check("queue", deps.Queue != nil, deps.Queue.Ping)
		code := http.StatusOK
		if !ready {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, status)
	}
}
