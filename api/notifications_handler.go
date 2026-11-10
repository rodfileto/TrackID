package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/follows"
	"github.com/rodfileto/trackid/notifications"
	"github.com/rodfileto/trackid/realtime"
)

// notificationView is one notification as its reader may see it now. The analysis fields are
// present only while the reader still holds that analysis. Blind says to show no case identity,
// as on the My analyses page. Changes is what a followed person or cluster went through, present
// only while the reader is an intelligence analyst, who may open any of it.
type notificationView struct {
	ID           int64            `json:"id"`
	Kind         string           `json:"kind"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
	Read         bool             `json:"read"`
	CaseID       string           `json:"caseId,omitempty"`
	AnalysisID   int64            `json:"analysisId,omitempty"`
	AnalysisType analyses.Type    `json:"analysisType,omitempty"`
	Step         analyses.Step    `json:"step,omitempty"`
	Blind        bool             `json:"blind,omitempty"`
	Changes      *follows.Summary `json:"changes,omitempty"`
}

// ListNotificationsHandler returns the signed-in user's latest notifications and unread count.
func ListNotificationsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		user := currentUser(context)
		ctx := context.Request.Context()
		items, unread, err := notifications.List(ctx, sqlDB, user.ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list notifications"})
			return
		}
		held, err := distribution.AnalysesOf(ctx, sqlDB, user.ID)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list notifications"})
			return
		}
		byID := make(map[string]distribution.AssignedAnalysis, len(held))
		for _, analysis := range held {
			byID[strconv.FormatInt(analysis.AnalysisID, 10)] = analysis
		}
		summaries := map[int64]follows.Summary{}
		if user.IntelligenceAccess() {
			if summaries, err = follows.Summarize(ctx, sqlDB, items); err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list notifications"})
				return
			}
		}
		views := make([]notificationView, len(items))
		for i, item := range items {
			views[i] = notificationView{ID: item.ID, Kind: item.Kind, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Read: item.Read}
			if summary, ok := summaries[item.ID]; ok {
				views[i].Changes = &summary
			}
			if analysis, ok := byID[item.Subject]; ok && item.Kind == notifications.KindAnalysisAssigned {
				views[i].CaseID, views[i].AnalysisID = analysis.CaseID, analysis.AnalysisID
				views[i].AnalysisType, views[i].Step, views[i].Blind = analysis.Type, analysis.Step, analysis.Blind
			}
		}
		context.JSON(http.StatusOK, gin.H{"items": views, "unread": unread})
	}
}

// MarkNotificationReadHandler marks one of the signed-in user's notifications read.
func MarkNotificationReadHandler(sqlDB *sql.DB, hub *realtime.Hub) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, err := strconv.ParseInt(context.Param("notificationId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
			return
		}
		user := currentUser(context)
		if err := notifications.MarkRead(context.Request.Context(), sqlDB, id, user.ID); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not update the notification"})
			return
		}
		signalNotificationsChanged(hub, user.ID)
		context.Status(http.StatusNoContent)
	}
}

// MarkAllNotificationsReadHandler marks all of the signed-in user's notifications read.
func MarkAllNotificationsReadHandler(sqlDB *sql.DB, hub *realtime.Hub) gin.HandlerFunc {
	return func(context *gin.Context) {
		user := currentUser(context)
		if err := notifications.MarkAllRead(context.Request.Context(), sqlDB, user.ID); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not update the notifications"})
			return
		}
		signalNotificationsChanged(hub, user.ID)
		context.Status(http.StatusNoContent)
	}
}

// signalNotificationsChanged keeps the user's other tabs in step.
func signalNotificationsChanged(hub *realtime.Hub, userID int64) {
	if hub != nil {
		hub.Publish(realtime.UserTopic(userID), realtime.Event{Type: notifications.EventChanged})
	}
}
