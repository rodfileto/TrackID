package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const contextUserKey = "auth_user"

// RequireAuth verifies the Bearer token and stores the claims in the gin
// context for handlers to read via CurrentUser.
func RequireAuth(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := BearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "missing or malformed Authorization header"})
			return
		}

		claims, err := svc.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "invalid or expired token"})
			return
		}

		c.Set(contextUserKey, claims)
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get(contextUserKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*Claims)
	return claims, ok
}
