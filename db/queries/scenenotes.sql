-- Notes written at a scene and synced from the device (docs/scene-examination-plan.md).

-- name: UpsertSceneNote :one
-- A note synced again replaces what it said, while it is its author's and not yet a collection.
-- No row comes back when the id is someone else's note, or one already converted.
INSERT INTO scene_notes (client_id, author_id, place_type, place_name, lon, lat, address, neighbourhood,
    geocode_source, geocode_flags, called_at, examined_at, occurred_at, team, other_officers, notes, written_at)
VALUES (@client_id::uuid, @author_id::bigint, sqlc.narg('place_type'), sqlc.narg('place_name'), sqlc.narg('lon'), sqlc.narg('lat'),
    sqlc.narg('address'), sqlc.narg('neighbourhood'), sqlc.narg('geocode_source'), COALESCE(@geocode_flags::text[], '{}'),
    sqlc.narg('called_at'), sqlc.narg('examined_at'), sqlc.narg('occurred_at'), COALESCE(@team::bigint[], '{}'),
    sqlc.narg('other_officers'), sqlc.narg('notes'), @written_at::timestamptz)
ON CONFLICT (client_id) DO UPDATE SET
    place_type = EXCLUDED.place_type, place_name = EXCLUDED.place_name, lon = EXCLUDED.lon, lat = EXCLUDED.lat,
    address = EXCLUDED.address, neighbourhood = EXCLUDED.neighbourhood,
    geocode_source = EXCLUDED.geocode_source, geocode_flags = EXCLUDED.geocode_flags,
    called_at = EXCLUDED.called_at, examined_at = EXCLUDED.examined_at, occurred_at = EXCLUDED.occurred_at,
    team = EXCLUDED.team, other_officers = EXCLUDED.other_officers, notes = EXCLUDED.notes,
    written_at = EXCLUDED.written_at, updated_at = NOW()
WHERE scene_notes.author_id = EXCLUDED.author_id AND scene_notes.collection_id IS NULL
RETURNING id, (xmax = 0) AS inserted;

-- name: GetSceneNoteByClientID :one
SELECT id, author_id, collection_id FROM scene_notes WHERE client_id = @client_id::uuid;

-- name: GetSceneNote :one
SELECT n.id, n.client_id, n.author_id, u.username AS author_name, n.place_type, n.place_name, n.lon, n.lat, n.address,
    n.neighbourhood, n.geocode_source, n.geocode_flags, n.called_at, n.examined_at, n.occurred_at, n.team, n.other_officers,
    n.notes, n.written_at, n.synced_at, n.collection_id
FROM scene_notes n JOIN users u ON u.id = n.author_id
WHERE n.id = $1;

-- name: ListSceneNotes :many
-- An author's notes, or everyone's (everyone, for a coordinator), newest first; those already
-- converted only when asked.
SELECT n.id, n.client_id, n.author_id, u.username AS author_name, n.place_type, n.place_name, n.lon, n.lat, n.address,
    n.neighbourhood, n.geocode_source, n.geocode_flags, n.called_at, n.examined_at, n.occurred_at, n.team, n.other_officers,
    n.notes, n.written_at, n.synced_at, n.collection_id,
    (SELECT COUNT(*) FROM scene_note_audios a WHERE a.scene_note_id = n.id)::int4 AS audio_count
FROM scene_notes n JOIN users u ON u.id = n.author_id
WHERE (@everyone::bool OR n.author_id = @author_id::bigint)
  AND (@converted::bool OR n.collection_id IS NULL)
ORDER BY n.written_at DESC, n.id DESC
LIMIT 200;

-- name: DeleteSceneNote :execrows
-- A note not yet a collection is discarded with its audio rows; the caller removes nothing from
-- storage (object lock keeps it).
DELETE FROM scene_notes WHERE id = $1 AND collection_id IS NULL;

-- name: SetSceneNoteCollection :execrows
UPDATE scene_notes SET collection_id = @collection_id::bigint, updated_at = NOW()
WHERE id = @id::bigint AND collection_id IS NULL;

-- name: InsertSceneNoteAudio :one
INSERT INTO scene_note_audios (scene_note_id, client_id, title, content_type, size_bytes, sha256, storage_ref, recorded_at)
VALUES (@scene_note_id::bigint, @client_id::uuid, sqlc.narg('title'), @content_type::text, @size_bytes::bigint, @sha256::text,
    @storage_ref::text, sqlc.narg('recorded_at'))
RETURNING id;

-- name: GetSceneNoteAudioByClientID :one
SELECT id, scene_note_id, sha256 FROM scene_note_audios WHERE client_id = @client_id::uuid;

-- name: GetSceneNoteAudio :one
SELECT a.id, a.scene_note_id, a.title, a.content_type, a.size_bytes, a.sha256, a.storage_ref, a.recorded_at, n.author_id
FROM scene_note_audios a JOIN scene_notes n ON n.id = a.scene_note_id
WHERE a.id = $1;

-- name: ListSceneNoteAudios :many
SELECT id, client_id, title, content_type, size_bytes, sha256, storage_ref, recorded_at
FROM scene_note_audios WHERE scene_note_id = $1 ORDER BY recorded_at NULLS LAST, id;

-- name: ListUsernamesByIDs :many
SELECT id, username, first_name, last_name FROM users WHERE id = ANY(@ids::bigint[]) ORDER BY first_name, last_name, id;

-- name: InsertCollectionMember :exec
INSERT INTO case_collection_members (collection_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListCaseCollectionMembers :many
SELECT m.collection_id, u.id AS user_id, u.username, u.first_name, u.last_name
FROM case_collection_members m
JOIN case_collections cc ON cc.id = m.collection_id
JOIN biometric_cases bc ON bc.id = cc.biometric_case_id
JOIN users u ON u.id = m.user_id
WHERE bc.case_id = $1
ORDER BY m.collection_id, u.first_name, u.last_name, u.id;

-- name: InsertCollectionNoteFile :one
-- An audio note of a scene, as a file of the case whose origin is the collection.
INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename, storage_ref, content_type, size_bytes, collection_id)
VALUES (@biometric_case_id::bigint, 'collection_note', 'audio', @hash_id::text, @filename::text, @storage_ref::text,
    @content_type::text, @size_bytes::bigint, @collection_id::bigint)
ON CONFLICT (biometric_case_id, category, hash_id) DO NOTHING
RETURNING id;

-- name: ListCollectionNoteFiles :many
SELECT f.id, f.collection_id, f.filename, f.content_type, f.size_bytes, f.hash_id, f.created_at
FROM case_files f JOIN biometric_cases bc ON bc.id = f.biometric_case_id
WHERE bc.case_id = $1 AND f.category = 'collection_note'
ORDER BY f.collection_id, f.id;

-- name: GetCollectionNoteFile :one
SELECT f.id, f.filename, f.content_type, f.storage_ref
FROM case_files f JOIN biometric_cases bc ON bc.id = f.biometric_case_id
WHERE f.id = $1 AND bc.case_id = $2 AND f.category = 'collection_note';
