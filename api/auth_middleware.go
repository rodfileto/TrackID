package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/rodfileto/trackid/auth"
)

// Middleware authenticates the request's Bearer token and, on success, sets the
// "userID" and "username" values on the context.
func Middleware(jwtSecret string) gin.HandlerFunc {
	return func(context *gin.Context) {
		header := context.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || jwtSecret == "" {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid token"})
			return
		}
		token, err := jwt.Parse(strings.TrimPrefix(header, "Bearer "), func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		context.Set("userID", claims["sub"])
		if username, ok := claims["username"].(string); ok {
			context.Set("username", username)
		}
		context.Next()
	}
}

// currentUserKey is the context key ActiveUser stores the signed-in auth.UserProfile under.
const currentUserKey = "currentUser"

// ActiveUser runs after Middleware: it loads the token's user and refuses one that no longer
// exists or was deactivated, so deactivating an account takes effect on its next request
// rather than when its token expires. Handlers read the profile with currentUser.
func ActiveUser(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		if repo == nil {
			context.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		userID, _ := context.Get("userID")
		subject, _ := userID.(string)
		id, err := strconv.ParseInt(subject, 10, 64)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		user, err := repo.FindByID(context.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not load user"})
			return
		}
		if !user.IsActive {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "account is deactivated"})
			return
		}
		context.Set(currentUserKey, user)
		context.Next()
	}
}

// RequireAdmin lets through only an ADMIN or SUPER_ADMIN. It runs after ActiveUser.
func RequireAdmin() gin.HandlerFunc {
	return func(context *gin.Context) {
		if !currentUser(context).AccountRole.IsAdmin() {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		context.Next()
	}
}

func currentUser(context *gin.Context) auth.UserProfile {
	user, _ := context.Get(currentUserKey)
	profile, _ := user.(auth.UserProfile)
	return profile
}
