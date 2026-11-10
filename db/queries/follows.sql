-- Following persons and clusters (docs/follow-plan.md, migrations/023_follows.sql).

-- name: InsertFollow :exec
INSERT INTO follows (user_id, subject_kind, subject_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: DeleteFollow :exec
DELETE FROM follows WHERE user_id = $1 AND subject_kind = $2 AND subject_id = $3;

-- name: IsFollowing :one
SELECT EXISTS (SELECT 1 FROM follows WHERE user_id = $1 AND subject_kind = $2 AND subject_id = $3);

-- name: ListFollowsOfUser :many
SELECT subject_kind, subject_id, created_at FROM follows WHERE user_id = $1 ORDER BY created_at DESC;

-- name: ListActiveFollows :many
-- Who follows what, among those who may still be told: active intelligence analysts.
SELECT f.user_id, f.subject_kind, f.subject_id
FROM follows f
JOIN users u ON u.id = f.user_id
WHERE u.is_active AND u.intelligence_analyst;

-- name: MoveClusterFollows :exec
-- A followed cluster merged into another: its followers now follow the survivor.
WITH moved AS (
    DELETE FROM follows WHERE subject_kind = 'CLUSTER' AND subject_id = @from_cluster::text
    RETURNING user_id, created_at
)
INSERT INTO follows (user_id, subject_kind, subject_id, created_at)
SELECT user_id, 'CLUSTER', @to_cluster::text, created_at FROM moved
ON CONFLICT DO NOTHING;

-- name: ClusterBasis :one
SELECT basis FROM clusters WHERE id = $1;

-- name: AutomaticClusterOfReviewed :one
-- Every REVIEWED cluster sits inside one AUTOMATIC cluster: the one its members belong to there.
SELECT a.cluster_id
FROM cluster_members r
JOIN cluster_members a ON a.feature_id = r.feature_id AND a.basis = 'AUTOMATIC'
WHERE r.cluster_id = $1 AND r.basis = 'REVIEWED'
LIMIT 1;

-- name: PersonExists :one
SELECT EXISTS (SELECT 1 FROM persons WHERE person_id = $1);

-- name: TryFanoutLock :one
-- One fan-out at a time, whichever process runs it.
SELECT pg_try_advisory_xact_lock(hashtext('trackid_subject_changes'));

-- name: ListPendingChanges :many
SELECT id, subject_kind, subject_id, change, ref, amount
FROM subject_changes
WHERE fanned_out_at IS NULL
ORDER BY id;

-- name: InsertSubjectChange :one
INSERT INTO subject_changes (subject_kind, subject_id, change, ref, amount, fanned_out_at)
VALUES (@subject_kind, @subject_id, @change, @ref, @amount, sqlc.narg(fanned_out_at))
RETURNING id;

-- name: SignalSubjectChanges :exec
SELECT pg_notify('trackid_subject_changes', '');

-- name: MarkChangesFannedOut :exec
UPDATE subject_changes SET fanned_out_at = NOW() WHERE id = ANY(@ids::bigint[]);

-- name: DeleteOldSubjectChanges :execrows
-- Changes handled long ago that no unread notification still reports.
DELETE FROM subject_changes
WHERE fanned_out_at < @before
  AND id < COALESCE((SELECT MIN(first_change_id) FROM notifications WHERE read_at IS NULL AND first_change_id IS NOT NULL), 9223372036854775807);

-- name: ListKnownFeaturesOfPersons :many
-- The KNOWN feature ids (identity_files.id, as graph.KnownFeatureID writes them) of the given persons.
SELECT p.person_id, f.id::text AS feature_id
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_files f ON f.register_id = r.id
WHERE p.person_id = ANY(@person_ids::text[]);

-- name: GetPersonOfIdentityFile :one
SELECT p.person_id
FROM identity_files f
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE f.id = $1;

-- name: ListPersonsInClusters :many
-- The persons whose KNOWN features sit in the given clusters.
SELECT DISTINCT p.person_id
FROM cluster_members m
JOIN identity_files f ON f.id = CASE WHEN m.feature_id ~ '^[0-9]+$' THEN m.feature_id::bigint END
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE m.cluster_id = ANY(@cluster_ids::bigint[]);

-- name: ListSubjectChangesInRange :many
SELECT id, change, ref, amount
FROM subject_changes
WHERE subject_kind = @subject_kind AND subject_id = @subject_id AND id BETWEEN @first_id AND @last_id
ORDER BY id;

-- name: ListPersonLabels :many
-- What a notification calls a person: the number in their page's address and their first register's name.
SELECT p.person_id, p.id AS person_number,
       COALESCE((SELECT r.name FROM identity_documents d JOIN identity_registers r ON r.document_id = d.id
                 WHERE d.person_id = p.id ORDER BY r.id LIMIT 1), '')::text AS name
FROM persons p
WHERE p.person_id = ANY(@person_ids::text[]);
