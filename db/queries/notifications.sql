-- name: InsertNotification :one
INSERT INTO notifications (user_id, kind, subject) VALUES ($1, $2, $3) RETURNING id;

-- name: SignalLive :exec
SELECT pg_notify(@channel::text, @payload::text);

-- name: GetNotificationRecipient :one
SELECT user_id FROM notifications WHERE id = $1;

-- name: ListNotifications :many
SELECT id, kind, subject, created_at, read_at FROM notifications
WHERE user_id = $1
ORDER BY id DESC
LIMIT $2;

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL;
