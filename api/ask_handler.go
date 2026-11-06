package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/ask"
)

// The AI mode's limits, guards rather than features: the browser sends the conversation so far with
// each question, and nothing makes it short.
const (
	askMaxQuestion = 4 << 10
	askMaxTurns    = 20
	askMaxTurnText = 16 << 10
	askMaxFocus    = 300
	askMaxFocusRef = 300
)

// AskInfoHandler says whether the AI mode has a model, which, and where it runs.
func AskInfoHandler(sqlDB *sql.DB, model *ask.Model) gin.HandlerFunc {
	service := ask.NewService(sqlDB, model)
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, service.Info())
	}
}

type askRequest struct {
	Question string        `json:"question"`
	History  []ask.Message `json:"history"`
	// Focus is what the conversation has found so far (Answer.Entities). It only tells the model what
	// "she" or "the second case" refers to: the tools check access whatever it says.
	Focus []ask.Entity `json:"focus"`
}

// AskHandler answers one question. It is an intelligence route, so an analyst working blind cannot
// ask (requireCapability), and every question is recorded in ask_queries.
func AskHandler(sqlDB *sql.DB, model *ask.Model) gin.HandlerFunc {
	service := ask.NewService(sqlDB, model)
	return func(c *gin.Context) {
		if model == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no model is configured (ASK_ENDPOINT/ASK_MODEL)"})
			return
		}
		var body askRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		question := strings.TrimSpace(body.Question)
		if question == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type a question"})
			return
		}
		if len(question) > askMaxQuestion {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the question is longer than 4 KB"})
			return
		}
		if len(body.History) > askMaxTurns {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the conversation is too long: start a new one"})
			return
		}
		for _, turn := range body.History {
			if turn.Role != "user" && turn.Role != "assistant" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation"})
				return
			}
			if len(turn.Text) > askMaxTurnText {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the conversation is too long: start a new one"})
				return
			}
		}
		if len(body.Focus) > askMaxFocus {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the conversation is too long: start a new one"})
			return
		}
		for _, entity := range body.Focus {
			if !entity.Valid() || len(entity.Ref)+len(entity.Label) > askMaxFocusRef {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation"})
				return
			}
		}
		user := currentUser(c)
		answer, err := service.Ask(c.Request.Context(), ask.Asker{ID: user.ID, Username: user.Username}, question, body.History, body.Focus)
		if err != nil {
			switch {
			case errors.Is(err, ask.ErrRealData):
				c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "real_data"})
			case errors.Is(err, ask.ErrRefused):
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error(), "code": "refused"})
			case errors.Is(err, ask.ErrTruncated):
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "code": "truncated"})
			default:
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			}
			return
		}
		c.JSON(http.StatusOK, answer)
	}
}
