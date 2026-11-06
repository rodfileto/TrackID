-- Intelligence operations (db/migrations/017_operations.sql, docs/operations-plan.md).

-- name: CreateOperation :one
INSERT INTO operations (slug, name, description, created_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (slug) DO NOTHING
RETURNING *;

-- name: ListOperations :many
-- Active ones first, the newest first within each.
SELECT * FROM operations
ORDER BY status = 'CLOSED', created_at DESC, id DESC;

-- name: GetOperationBySlug :one
SELECT * FROM operations WHERE slug = $1;

-- name: UpdateOperation :one
UPDATE operations SET
    name = $2,
    description = $3,
    status = $4,
    closed_at = CASE WHEN $4 = 'CLOSED' THEN COALESCE(closed_at, NOW()) END,
    updated_at = NOW()
WHERE slug = $1
RETURNING *;
