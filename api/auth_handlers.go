package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/rodfileto/trackid/auth"
)

type loginRequest struct {
	StaffNumber string `json:"staff_number" binding:"required"`
	Password    string `json:"password" binding:"required"`
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
		authenticated, err := repo.FindByStaffNumber(context.Request.Context(), request.StaffNumber)
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
		token, err := IssueToken(authenticated.Profile, jwtSecret)
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

// UpdateMeHandler changes the signed-in user's name and job title.
func UpdateMeHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		var details auth.Details
		if err := context.ShouldBindJSON(&details); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		profile, err := repo.UpdateDetails(context.Request.Context(), currentUser(context).ID, details)
		switch {
		case errors.Is(err, auth.ErrInvalidDetails):
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not update the profile"})
		default:
			context.JSON(http.StatusOK, profile)
		}
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangeMyPasswordHandler changes the signed-in user's password, given the current one.
func ChangeMyPasswordHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		var request changePasswordRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "the current and the new password are required"})
			return
		}
		err := repo.ChangePassword(context.Request.Context(), currentUser(context).ID, request.CurrentPassword, request.NewPassword)
		switch {
		case errors.Is(err, auth.ErrWrongPassword):
			context.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, auth.ErrPasswordTooShort):
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not change the password"})
		default:
			context.Status(http.StatusNoContent)
		}
	}
}

// SetMyAvatarHandler stores the signed-in user's picture (multipart field "file").
func SetMyAvatarHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		file, err := context.FormFile("file")
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}
		if file.Size > auth.MaxAvatarUpload {
			context.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the picture is larger than 10 MB"})
			return
		}
		opened, err := file.Open()
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
			return
		}
		defer opened.Close()
		data, err := io.ReadAll(opened)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
			return
		}
		profile, err := repo.SetAvatar(context.Request.Context(), currentUser(context).ID, data)
		switch {
		case errors.Is(err, auth.ErrNotAnImage):
			context.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		case err != nil:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not store the picture"})
		default:
			context.JSON(http.StatusOK, profile)
		}
	}
}

// RemoveMyAvatarHandler deletes the signed-in user's picture.
func RemoveMyAvatarHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		profile, err := repo.RemoveAvatar(context.Request.Context(), currentUser(context).ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove the picture"})
			return
		}
		context.JSON(http.StatusOK, profile)
	}
}

// UserAvatarHandler returns a user's picture to any signed-in user. The page asks for it with
// the profile's avatar_updated_at as a version, so it may be cached.
func UserAvatarHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, err := strconv.ParseInt(context.Param("userId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		data, _, err := repo.Avatar(context.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			context.JSON(http.StatusNotFound, gin.H{"error": "no picture"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the picture"})
			return
		}
		context.Header("Cache-Control", "private, max-age=86400")
		context.Data(http.StatusOK, "image/jpeg", data)
	}
}
