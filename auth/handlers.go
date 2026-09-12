package auth

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/rodrigorfcm/trackid/db"
)

type UserProfile struct {
	ID         int64  `json:"id"`
	Nome       string `json:"nome"`
	UltimoNome string `json:"ultimo_nome"`
	Matricula  string `json:"matricula"`
	Cargo      string `json:"cargo"`
	Username   string `json:"username"`
	Email      string `json:"email"`
}

type registerRequest struct {
	Nome       string `json:"nome" binding:"required"`
	UltimoNome string `json:"ultimo_nome" binding:"required"`
	Matricula  string `json:"matricula" binding:"required"`
	Cargo      string `json:"cargo" binding:"required"`
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Matricula string `json:"matricula" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  UserProfile `json:"user"`
}

func RegisterHandler(repo *Repository, emailDomain string) gin.HandlerFunc {
	return func(context *gin.Context) {
		if repo == nil {
			context.Status(http.StatusServiceUnavailable)
			return
		}
		var request registerRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid registration data"})
			return
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not secure password"})
			return
		}
		username := strings.ToLower(strings.TrimSpace(request.Username))
		email := username
		if emailDomain != "" {
			email = username + "@" + emailDomain
		}
		user, err := repo.CreateUser(context.Request.Context(), db.CreateUserParams{
			Nome:         request.Nome,
			UltimoNome:   request.UltimoNome,
			Matricula:    request.Matricula,
			Cargo:        request.Cargo,
			Username:     username,
			Email:        email,
			PasswordHash: string(passwordHash),
		})
		if err != nil {
			if strings.Contains(err.Error(), "users_matricula_key") {
				context.JSON(http.StatusConflict, gin.H{"error": "matricula already exists"})
				return
			}
			if strings.Contains(err.Error(), "users_username_key") || strings.Contains(err.Error(), "users_email_key") {
				context.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user"})
			return
		}
		context.JSON(http.StatusCreated, user)
	}
}

func LoginHandler(repo *Repository, jwtSecret string) gin.HandlerFunc {
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
		token, err := issueToken(authenticated.Profile, jwtSecret)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not create token"})
			return
		}
		context.JSON(http.StatusOK, loginResponse{Token: token, User: authenticated.Profile})
	}
}

func MeHandler(repo *Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		if repo == nil {
			context.Status(http.StatusServiceUnavailable)
			return
		}
		userID, ok := context.Get("userID")
		if !ok {
			context.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		id, err := strconv.ParseInt(userID.(string), 10, 64)
		if err != nil {
			context.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		user, err := repo.FindByID(context.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			context.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load profile"})
			return
		}
		context.JSON(http.StatusOK, user)
	}
}
