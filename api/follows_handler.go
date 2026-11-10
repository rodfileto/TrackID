package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/follows"
)

// followSubject resolves the :kind ("person" or "cluster") and :id of a follow route. It answers the request and
// returns false when it can't.
func followSubject(context *gin.Context, sqlDB *sql.DB) (follows.Subject, bool) {
	subject, err := follows.Resolve(context.Request.Context(), sqlDB, strings.ToUpper(context.Param("kind")), context.Param("id"))
	switch {
	case errors.Is(err, follows.ErrInvalidKind):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, follows.ErrNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case err != nil:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not find what to follow"})
	default:
		return subject, true
	}
	return follows.Subject{}, false
}

// followState is a subject's follow state. Subject is what is followed, which for a REVIEWED cluster is the
// AUTOMATIC cluster it sits inside.
type followState struct {
	Subject   follows.Subject `json:"subject"`
	Following bool            `json:"following"`
}

// GetFollowHandler says whether the signed-in user follows a person or a cluster.
func GetFollowHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		subject, ok := followSubject(context, sqlDB)
		if !ok {
			return
		}
		following, err := follows.IsFollowing(context.Request.Context(), sqlDB, currentUser(context).ID, subject)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the follow"})
			return
		}
		context.JSON(http.StatusOK, followState{Subject: subject, Following: following})
	}
}

// FollowHandler makes the signed-in user follow a person or a cluster.
func FollowHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		subject, ok := followSubject(context, sqlDB)
		if !ok {
			return
		}
		if err := follows.Add(context.Request.Context(), sqlDB, currentUser(context).ID, subject); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not follow"})
			return
		}
		context.JSON(http.StatusOK, followState{Subject: subject, Following: true})
	}
}

// UnfollowHandler stops the signed-in user following a person or a cluster.
func UnfollowHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		subject, ok := followSubject(context, sqlDB)
		if !ok {
			return
		}
		if err := follows.Remove(context.Request.Context(), sqlDB, currentUser(context).ID, subject); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not stop following"})
			return
		}
		context.JSON(http.StatusOK, followState{Subject: subject, Following: false})
	}
}

// ListFollowsHandler lists what the signed-in user follows.
func ListFollowsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := follows.List(context.Request.Context(), sqlDB, currentUser(context).ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the follows"})
			return
		}
		if list == nil {
			list = []follows.Follow{}
		}
		context.JSON(http.StatusOK, gin.H{"items": list})
	}
}
