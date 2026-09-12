package database

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

func HealthHandler(db *sql.DB) gin.HandlerFunc {
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
