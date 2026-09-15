package auth

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IssueToken signs a 24-hour HS256 JWT for user.
func IssueToken(user UserProfile, jwtSecret string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{"sub": strconv.FormatInt(user.ID, 10), "matricula": user.Matricula, "username": user.Username, "iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
}
