-- name: InsertNotification :one
INSERT INTO notifications (user_id, kind, subject) VALUES ($1, $2, $3) RETURNING id;

-- name: SignalLive :exec
SELECT pg_notify(@channel::text, @payload::text);

-- name: GetNotificationRecipient :one
SELECT user_id FROM notifications WHERE id = $1;

-- name: ListNotifications :many
-- Latest first, a grouped notification counting from its last update.
SELECT id, kind, subject, created_at, updated_at, read_at, first_change_id, last_change_id FROM notifications
WHERE user_id = $1
ORDER BY updated_at DESC, id DESC
LIMIT $2;

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL;

-- name: UpsertSubjectNotification :one
-- A user keeps one unread notification per followed subject: further changes extend it.
INSERT INTO notifications (user_id, kind, subject, first_change_id, last_change_id)
VALUES (@user_id, @kind, @subject, @first_change_id, @last_change_id)
ON CONFLICT (user_id, kind, subject) WHERE read_at IS NULL AND kind IN ('PERSON_CHANGED', 'CLUSTER_CHANGED')
DO UPDATE SET last_change_id = GREATEST(notifications.last_change_id, EXCLUDED.last_change_id), updated_at = NOW()
RETURNING id;

-- name: GetTeamsDelivery :one
-- What deciding on a Teams message for one notification needs.
SELECT n.user_id, n.kind, n.read_at, n.teams_sent_at, u.teams_webhook, u.is_active
FROM notifications n
JOIN users u ON u.id = n.user_id
WHERE n.id = $1;

-- name: MarkTeamsSent :exec
UPDATE notifications SET teams_sent_at = NOW() WHERE id = $1;
