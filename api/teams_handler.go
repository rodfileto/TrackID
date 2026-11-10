package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/teams"
)

// GetTeamsWebhookHandler returns the signed-in user's Teams webhook status, never the URL.
func GetTeamsWebhookHandler(service *teams.Service) gin.HandlerFunc {
	return func(context *gin.Context) {
		status, err := service.GetStatus(context.Request.Context(), currentUser(context).ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the Teams webhook"})
			return
		}
		context.JSON(http.StatusOK, status)
	}
}

type setTeamsWebhookRequest struct {
	URL string `json:"url" binding:"required"`
}

// SetTeamsWebhookHandler sets the signed-in user's Teams webhook.
func SetTeamsWebhookHandler(service *teams.Service) gin.HandlerFunc {
	return func(context *gin.Context) {
		var req setTeamsWebhookRequest
		if err := context.ShouldBindJSON(&req); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
			return
		}
		if !teamsAnswer(context, service.SetWebhook(context.Request.Context(), currentUser(context).ID, req.URL)) {
			return
		}
		GetTeamsWebhookHandler(service)(context)
	}
}

// ClearTeamsWebhookHandler removes the signed-in user's Teams webhook.
func ClearTeamsWebhookHandler(service *teams.Service) gin.HandlerFunc {
	return func(context *gin.Context) {
		if teamsAnswer(context, service.ClearWebhook(context.Request.Context(), currentUser(context).ID)) {
			context.Status(http.StatusNoContent)
		}
	}
}

// TestTeamsWebhookHandler sends the signed-in user a test message through their webhook.
func TestTeamsWebhookHandler(service *teams.Service) gin.HandlerFunc {
	return func(context *gin.Context) {
		if teamsAnswer(context, service.SendTest(context.Request.Context(), currentUser(context).ID)) {
			context.Status(http.StatusNoContent)
		}
	}
}

// teamsAnswer answers a failed Teams request and returns whether err was nil.
func teamsAnswer(context *gin.Context, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, teams.ErrOff):
		context.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, teams.ErrInvalidURL), errors.Is(err, teams.ErrNoWebhook):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		// A failed test send: what went wrong is the user's to see, and never holds the URL.
		context.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	}
	return false
}
