// Package notifications stores what a user should be told and signals their browser
// (docs/realtime-plan.md). Rows hold a kind and a reference only; what the user reads is built
// when they read it, under the access they have then.
package notifications

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/realtime"
)

const (
	// Channel is the Postgres NOTIFY channel that carries new notifications' IDs to the API.
	Channel = "trackid_live"
	// EventChanged tells a user's browsers to fetch their notifications again.
	EventChanged = "notifications.changed"
	// KindAnalysisAssigned has an analysis ID as its subject.
	KindAnalysisAssigned = "ANALYSIS_ASSIGNED"
	// KindPersonChanged has a followed person's person_id as its subject, and KindClusterChanged a followed
	// AUTOMATIC cluster's ID (follows package). Each reports a range of the subject's changes.
	KindPersonChanged  = "PERSON_CHANGED"
	KindClusterChanged = "CLUSTER_CHANGED"
	// ListLimit is how many notifications a user is shown.
	ListLimit = 30
)

// Item is one stored notification. FirstChangeID and LastChangeID bound the subject changes a PERSON_CHANGED or
// CLUSTER_CHANGED notification reports; they are zero for other kinds.
type Item struct {
	ID            int64
	Kind          string
	Subject       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Read          bool
	FirstChangeID int64
	LastChangeID  int64
}

// Create records a notification on q, which should be the transaction of the action it reports:
// the row and the signal then both exist only if that action commits, whichever process runs it.
func Create(ctx context.Context, q *db.Queries, userID int64, kind, subject string) error {
	id, err := q.InsertNotification(ctx, db.InsertNotificationParams{UserID: userID, Kind: kind, Subject: subject})
	if err != nil {
		return err
	}
	return Signal(ctx, q, id)
}

// Signal tells the API, once q's transaction commits, that a notification is new or has changed.
func Signal(ctx context.Context, q *db.Queries, id int64) error {
	return q.SignalLive(ctx, db.SignalLiveParams{Channel: Channel, Payload: strconv.FormatInt(id, 10)})
}

// Relay returns the handler for realtime.Listen on Channel: it finds who a new notification is
// for and signals their browsers.
func Relay(sqlDB *sql.DB, publisher realtime.Publisher) func(context.Context, string) {
	q := db.New(sqlDB)
	return func(ctx context.Context, payload string) {
		id, err := strconv.ParseInt(payload, 10, 64)
		if err != nil {
			log.Printf("notifications: ignoring signal %q: %v", payload, err)
			return
		}
		userID, err := q.GetNotificationRecipient(ctx, id)
		if err != nil {
			log.Printf("notifications: could not find the recipient of %d: %v", id, err)
			return
		}
		publisher.Publish(realtime.UserTopic(userID), realtime.Event{Type: EventChanged})
	}
}

// List returns a user's latest notifications and how many of all theirs are unread.
func List(ctx context.Context, sqlDB *sql.DB, userID int64) ([]Item, int64, error) {
	q := db.New(sqlDB)
	rows, err := q.ListNotifications(ctx, db.ListNotificationsParams{UserID: userID, Limit: ListLimit})
	if err != nil {
		return nil, 0, err
	}
	unread, err := q.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Item, len(rows))
	for i, r := range rows {
		items[i] = Item{ID: r.ID, Kind: r.Kind, Subject: r.Subject, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			Read: r.ReadAt.Valid, FirstChangeID: r.FirstChangeID.Int64, LastChangeID: r.LastChangeID.Int64}
	}
	return items, unread, nil
}

// MarkRead marks one of the user's notifications read. Another user's is left alone.
func MarkRead(ctx context.Context, sqlDB *sql.DB, id, userID int64) error {
	return db.New(sqlDB).MarkNotificationRead(ctx, db.MarkNotificationReadParams{ID: id, UserID: userID})
}

// MarkAllRead marks all of the user's notifications read.
func MarkAllRead(ctx context.Context, sqlDB *sql.DB, userID int64) error {
	return db.New(sqlDB).MarkAllNotificationsRead(ctx, userID)
}
