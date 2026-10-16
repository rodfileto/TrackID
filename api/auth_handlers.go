package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/rodfileto/trackid/auth"
)

type loginRequest struct {
	Matricula string `json:"matricula" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token string           `json:"token"`
	User  auth.UserProfile `json:"user"`
}

func LoginHandler(repo *auth.Repository, jwtSecret string) gin.HandlerFunc {
	return func(context *gin.Context) {
		if repo == nil {
			context.Status(http.StatusServiceUnavailable)
			return
		}
		if jwtSecret == "" {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "JWT_SECRET is not configured"})
			return
		}
		var request loginRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid login data"})
			return
		}
		authenticated, err := repo.FindByMatricula(context.Request.Context(), request.Matricula)
		if errors.Is(err, sql.ErrNoRows) || bcrypt.CompareHashAndPassword([]byte(authenticated.PasswordHash), []byte(request.Password)) != nil {
			context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not authenticate user"})
			return
		}
		if !authenticated.Profile.IsActive {
			context.JSON(http.StatusForbidden, gin.H{"error": "account is deactivated"})
			return
		}
		token, err := auth.IssueToken(authenticated.Profile, jwtSecret)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not create token"})
			return
		}
		context.JSON(http.StatusOK, loginResponse{Token: token, User: authenticated.Profile})
	}
}

// MeHandler returns the signed-in user's profile, as ActiveUser loaded it.
func MeHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		context.JSON(http.StatusOK, currentUser(context))
	}
}
