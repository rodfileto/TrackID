-- name: CreateUser :one
INSERT INTO users (first_name, last_name, staff_number, job_title, username, email, password_hash, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, first_name, last_name, staff_number, job_title, username, email, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator, is_active, avatar_updated_at;

-- name: GetUserByStaffNumber :one
SELECT id, first_name, last_name, staff_number, job_title, username, email, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator, is_active, avatar_updated_at, password_hash
FROM users
WHERE staff_number = $1;

-- name: GetUserByID :one
SELECT id, first_name, last_name, staff_number, job_title, username, email, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator, is_active, avatar_updated_at
FROM users
WHERE id = $1;

-- name: ListUsers :many
SELECT id, first_name, last_name, staff_number, job_title, username, email, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator, is_active, avatar_updated_at
FROM users
ORDER BY last_name, first_name, id;

-- name: UpdateUserAccess :one
-- Username, staff_number and email are not updatable: decisions name their decider by username.
UPDATE users
SET account_role = $2, examines_fingerprint = $3, examines_facial = $4, intelligence_analyst = $5, coordinator = $6,
    is_active = $7, updated_at = NOW()
WHERE id = $1
RETURNING id, first_name, last_name, staff_number, job_title, username, email, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator, is_active, avatar_updated_at;

-- name: SetUserPassword :execrows
UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1;

-- name: UpdateUserDetails :one
-- What a user may change about themselves (PATCH /api/me): their name and job title.
UPDATE users SET first_name = $2, last_name = $3, job_title = $4, updated_at = NOW()
WHERE id = $1
RETURNING id, first_name, last_name, staff_number, job_title, username, email, account_role, examines_fingerprint, examines_facial, intelligence_analyst, coordinator, is_active, avatar_updated_at;

-- name: SetUserAvatar :execrows
-- A NULL avatar removes it (and its timestamp).
UPDATE users SET avatar = sqlc.narg(avatar)::bytea,
    avatar_updated_at = CASE WHEN sqlc.narg(avatar)::bytea IS NULL THEN NULL ELSE NOW() END, updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: GetUserAvatar :one
SELECT avatar, avatar_updated_at FROM users WHERE id = $1 AND avatar IS NOT NULL;

-- name: GetUserPasswordHash :one
SELECT password_hash FROM users WHERE id = $1;

-- name: InsertAccessLog :exec
INSERT INTO access_log (user_id, username, kind, subject, route) VALUES ($1, $2, $3, $4, $5);

-- name: GetUserNames :many
SELECT id, username, first_name, last_name, staff_number, job_title FROM users WHERE id = ANY(@ids::bigint[]);

-- name: GetUsersByUsername :many
SELECT id, username, first_name, last_name, staff_number, job_title FROM users WHERE username = ANY(@usernames::text[]);

-- name: SetTeamsWebhook :exec
UPDATE users SET teams_webhook = $2, teams_webhook_set_at = NOW(), teams_last_error = NULL, teams_failed_at = NULL
WHERE id = $1;

-- name: ClearTeamsWebhook :exec
UPDATE users SET teams_webhook = NULL, teams_webhook_set_at = NULL, teams_last_error = NULL, teams_failed_at = NULL
WHERE id = $1;

-- name: GetTeamsWebhook :one
SELECT teams_webhook, teams_webhook_set_at, teams_last_error, teams_failed_at FROM users WHERE id = $1;

-- name: RecordTeamsFailure :exec
UPDATE users SET teams_last_error = $2, teams_failed_at = NOW() WHERE id = $1;

-- name: ClearTeamsFailure :exec
UPDATE users SET teams_last_error = NULL, teams_failed_at = NULL WHERE id = $1 AND teams_failed_at IS NOT NULL;
