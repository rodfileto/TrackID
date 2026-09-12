package auth

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func issueToken(user UserProfile, jwtSecret string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{"sub": strconv.FormatInt(user.ID, 10), "matricula": user.Matricula, "username": user.Username, "iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
}

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
