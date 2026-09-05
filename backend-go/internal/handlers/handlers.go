// Package handlers holds gin route handlers.
package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"trackid-backend/internal/auth"
	"trackid-backend/internal/db"
	"trackid-backend/internal/jobs"
	"trackid-backend/internal/oauth"
	"trackid-backend/internal/videoproc"
	"trackid-backend/internal/vision"
)

type Deps struct {
	DB      *db.DB
	AuthSvc *auth.Service
	OAuth   *oauth.Handler
	Jobs    *jobs.Store[videoproc.Result]
	Vision  *vision.Service
}

func RegisterRoutes(router *gin.Engine, deps Deps) {
	router.GET("/health", Health)

	authGroup := router.Group("/api/v1/auth")
	authGroup.POST("/register", deps.register)
	authGroup.POST("/login", deps.login)
	authGroup.GET("/:provider", deps.OAuth.BeginAuth)
	authGroup.GET("/:provider/callback", deps.OAuth.CompleteAuth)

	protected := router.Group("/api/v1")
	protected.Use(auth.RequireAuth(deps.AuthSvc))
	protected.GET("/auth/me", deps.me)
	protected.POST("/process-video", deps.processVideo)
	protected.GET("/process-video/:job_id", deps.getVideoJob)
}

func Health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

type credentialsBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (d Deps) register(c *gin.Context) {
	var body credentialsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "invalid request body"})
		return
	}

	username := strings.TrimSpace(body.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "username is required"})
		return
	}
	if len(body.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "password must be at least 8 characters"})
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not create user"})
		return
	}

	id := uuid.NewString()
	if err := d.DB.CreateUser(c.Request.Context(), id, username, hash); err != nil {
		if err == db.ErrAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"detail": "username already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not create user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "username": username})
}

func (d Deps) login(c *gin.Context) {
	var body credentialsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "invalid request body"})
		return
	}

	user, err := d.DB.GetUserByUsername(c.Request.Context(), strings.TrimSpace(body.Username))
	if err != nil || user.PasswordHash == nil || !auth.VerifyPassword(body.Password, *user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "invalid credentials"})
		return
	}

	username := ""
	if user.Username != nil {
		username = *user.Username
	}
	token, err := d.AuthSvc.CreateToken(user.ID, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not issue token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (d Deps) me(c *gin.Context) {
	claims, _ := auth.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{"user_id": claims.UserID, "username": claims.Username})
}
