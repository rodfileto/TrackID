// Package oauth wires goth for social login (Google, Microsoft), finds or
// creates the corresponding local user, and hands back to auth for JWT
// issuance -- OAuth only replaces how the user proves identity, sessions
// still work the same way afterward.
package oauth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
	"github.com/markbates/goth/providers/microsoftonline"

	"trackid-backend/internal/auth"
	"trackid-backend/internal/config"
	"trackid-backend/internal/db"
)

// Setup registers whichever providers have credentials configured. Call once
// at startup; providers with empty client ID/secret are skipped.
func Setup(cfg config.Config) {
	gothic.Store = sessions.NewCookieStore([]byte(cfg.SessionSecret))

	var providers []goth.Provider
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		providers = append(providers, google.New(
			cfg.GoogleClientID, cfg.GoogleClientSecret,
			cfg.BaseURL+"/api/v1/auth/google/callback",
			"email", "profile",
		))
	}
	if cfg.MicrosoftClientID != "" && cfg.MicrosoftClientSecret != "" {
		providers = append(providers, microsoftonline.New(
			cfg.MicrosoftClientID, cfg.MicrosoftClientSecret,
			cfg.BaseURL+"/api/v1/auth/microsoftonline/callback",
		))
	}
	goth.UseProviders(providers...)
}

type Handler struct {
	db          *db.DB
	authSvc     *auth.Service
	frontendURL string
}

func NewHandler(database *db.DB, authSvc *auth.Service, frontendURL string) *Handler {
	return &Handler{db: database, authSvc: authSvc, frontendURL: frontendURL}
}

// withProvider makes gin's :provider path param visible to gothic, which
// reads it from the URL query string.
func withProvider(c *gin.Context) {
	q := c.Request.URL.Query()
	q.Set("provider", c.Param("provider"))
	c.Request.URL.RawQuery = q.Encode()
}

// BeginAuth redirects the browser to the provider's login page.
// GET /api/v1/auth/:provider
func (h *Handler) BeginAuth(c *gin.Context) {
	withProvider(c)
	gothic.BeginAuthHandler(c.Writer, c.Request)
}

// CompleteAuth handles the provider's redirect back, finds-or-creates the
// local user, and redirects to the frontend with a session token.
// GET /api/v1/auth/:provider/callback
func (h *Handler) CompleteAuth(c *gin.Context) {
	withProvider(c)
	gothUser, err := gothic.CompleteUserAuth(c.Writer, c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "oauth login failed"})
		return
	}

	user, err := h.findOrCreateUser(c.Request.Context(), gothUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not complete login"})
		return
	}

	username := ""
	if user.Username != nil {
		username = *user.Username
	}
	token, err := h.authSvc.CreateToken(user.ID, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not issue token"})
		return
	}

	c.Redirect(http.StatusFound, h.frontendURL+"/oauth-callback?token="+token)
}

func (h *Handler) findOrCreateUser(ctx context.Context, gothUser goth.User) (*db.User, error) {
	if user, err := h.db.GetUserByIdentity(ctx, gothUser.Provider, gothUser.UserID); err == nil {
		return user, nil
	} else if err != db.ErrNotFound {
		return nil, err
	}

	if gothUser.Email != "" {
		if user, err := h.db.GetUserByEmail(ctx, gothUser.Email); err == nil {
			if linkErr := h.db.LinkIdentity(ctx, uuid.NewString(), user.ID, gothUser.Provider, gothUser.UserID); linkErr != nil {
				return nil, linkErr
			}
			return user, nil
		} else if err != db.ErrNotFound {
			return nil, err
		}
	}

	userID := uuid.NewString()
	if err := h.db.CreateOAuthUser(ctx, userID, gothUser.Email, gothUser.Provider, gothUser.UserID, uuid.NewString()); err != nil {
		return nil, err
	}
	return h.db.GetUserByID(ctx, userID)
}
