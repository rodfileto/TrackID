-- Scene collections, vestiges and seals (019_scene_collection.sql).

-- name: InsertCaseCollection :one
-- The case's next collection; the caller holds the case's lock (LockBiometricCase).
INSERT INTO case_collections (biometric_case_id, sequence, location_id, collected_from, collected_to, collected_by,
    nothing_collected, notes, registered_by)
VALUES (@biometric_case_id::bigint,
    (SELECT COALESCE(MAX(sequence), 0) + 1 FROM case_collections WHERE biometric_case_id = @biometric_case_id::bigint),
    sqlc.narg('location_id'), @collected_from, sqlc.narg('collected_to'), @collected_by::bigint,
    @nothing_collected::boolean, sqlc.narg('notes'), sqlc.narg('registered_by'))
RETURNING id, sequence;

-- name: ListCaseCollections :many
-- A case's collections in order, with who collected and registered each and the request that
-- formalized it, if one has.
SELECT cc.id, cc.sequence, cc.location_id, cc.collected_from, cc.collected_to, cc.nothing_collected, cc.notes,
       cc.created_at, cc.collected_by, cu.username AS collected_by_name, ru.username AS registered_by_name,
       cc.case_request_id, r.document_reference AS request_document, r.received_on AS request_received_on,
       l.name AS location_name, l.address AS location_address
FROM case_collections cc
JOIN biometric_cases bc ON bc.id = cc.biometric_case_id
JOIN users cu ON cu.id = cc.collected_by
LEFT JOIN users ru ON ru.id = cc.registered_by
LEFT JOIN case_requests r ON r.id = cc.case_request_id
LEFT JOIN locations l ON l.id = cc.location_id
WHERE bc.case_id = $1
ORDER BY cc.sequence;

-- name: GetCaseCollectionOfCase :one
SELECT id, case_request_id, nothing_collected FROM case_collections WHERE id = $1 AND biometric_case_id = $2;

-- name: FormalizeCollections :execrows
-- The request that formalizes these collections of a case, among those still awaiting one.
UPDATE case_collections SET case_request_id = @case_request_id::bigint
WHERE biometric_case_id = @biometric_case_id::bigint AND id = ANY(@collection_ids::bigint[]) AND case_request_id IS NULL;

-- name: ListAwaitingCollections :many
-- Collections still awaiting the request that formalizes them, oldest first.
SELECT cc.id, cc.sequence, cc.collected_from, cc.nothing_collected, cc.created_at, bc.case_id,
       cu.username AS collected_by_name,
       o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number, o.inquiry_number
FROM case_collections cc
JOIN biometric_cases bc ON bc.id = cc.biometric_case_id
JOIN users cu ON cu.id = cc.collected_by
LEFT JOIN occurrences o ON o.id = bc.occurrence_id
WHERE cc.case_request_id IS NULL
ORDER BY cc.created_at, cc.id;

-- name: InsertCaseVestige :one
-- The case's next vestige; the caller holds the case's lock (LockBiometricCase).
INSERT INTO case_vestiges (biometric_case_id, sequence, description, case_request_id, collection_id, created_by)
VALUES (@biometric_case_id::bigint,
    (SELECT COALESCE(MAX(sequence), 0) + 1 FROM case_vestiges WHERE biometric_case_id = @biometric_case_id::bigint),
    @description::text, sqlc.narg('case_request_id'), sqlc.narg('collection_id'), sqlc.narg('created_by'))
RETURNING id, sequence;

-- name: ListCaseVestiges :many
SELECT v.id, v.sequence, v.description, v.case_request_id, v.collection_id, v.created_at
FROM case_vestiges v
JOIN biometric_cases bc ON bc.id = v.biometric_case_id
WHERE bc.case_id = $1
ORDER BY v.sequence;

-- name: GetCaseVestigeOfCase :one
SELECT id, case_request_id, collection_id FROM case_vestiges WHERE id = $1 AND biometric_case_id = $2;

-- name: InsertVestigeSeal :one
INSERT INTO case_vestige_seals (vestige_id, seal_number, applied_at, applied_by)
VALUES (@vestige_id::bigint, @seal_number::text, @applied_at, sqlc.narg('applied_by'))
RETURNING id;

-- name: BreakVestigeSeal :execrows
-- Breaks the vestige's unbroken seal, if it has one.
UPDATE case_vestige_seals SET broken_at = @broken_at, broken_by = @broken_by::bigint
WHERE vestige_id = @vestige_id::bigint AND broken_at IS NULL;

-- name: ListCaseVestigeSeals :many
-- Every seal of a case's vestiges, in the order they were applied.
SELECT s.id, s.vestige_id, s.seal_number, s.applied_at, au.username AS applied_by_name, s.broken_at,
       bu.username AS broken_by_name
FROM case_vestige_seals s
JOIN case_vestiges v ON v.id = s.vestige_id
JOIN biometric_cases bc ON bc.id = v.biometric_case_id
LEFT JOIN users au ON au.id = s.applied_by
LEFT JOIN users bu ON bu.id = s.broken_by
WHERE bc.case_id = $1
ORDER BY s.vestige_id, s.applied_at, s.id;

-- name: LinkRequestVestiges :execrows
-- The vestiges of a case a request's questions are about.
INSERT INTO case_request_vestiges (case_request_id, vestige_id)
SELECT @case_request_id::bigint, v.id FROM case_vestiges v
WHERE v.biometric_case_id = @biometric_case_id::bigint AND v.id = ANY(@vestige_ids::bigint[])
ON CONFLICT DO NOTHING;

-- name: ListCaseRequestVestiges :many
SELECT rv.case_request_id, rv.vestige_id
FROM case_request_vestiges rv
JOIN case_vestiges v ON v.id = rv.vestige_id
JOIN biometric_cases bc ON bc.id = v.biometric_case_id
WHERE bc.case_id = $1
ORDER BY rv.case_request_id, v.sequence;

-- name: AttachCollectionFiles :execrows
-- Marks a case's evidence files, with no origin yet, as material the collection gathered.
UPDATE case_files SET collection_id = @collection_id::bigint, updated_at = NOW()
WHERE biometric_case_id = @biometric_case_id::bigint AND category IN ('evidence', 'video')
  AND case_request_id IS NULL AND collection_id IS NULL AND id = ANY(@file_ids::bigint[]);

-- name: SetFileVestige :execrows
UPDATE case_files SET vestige_id = sqlc.narg('vestige_id'), updated_at = NOW()
WHERE id = @id::bigint AND biometric_case_id = @biometric_case_id::bigint AND category IN ('evidence', 'video');

-- name: SetCaseReportMode :execrows
UPDATE biometric_cases SET report_mode = @report_mode::text, updated_at = NOW() WHERE id = @id::bigint;

-- name: CountIssuedCaseReports :one
SELECT COUNT(*) FROM analysis_reports ar
JOIN case_analyses ca ON ca.id = ar.case_analysis_id
WHERE ca.biometric_case_id = $1 AND ar.status = 'ISSUED';

-- name: GetCaseFileOrigin :one
SELECT id, case_request_id, collection_id FROM case_files
WHERE id = $1 AND biometric_case_id = $2 AND category IN ('evidence', 'video');

-- name: InheritFileOrigin :exec
-- A file derived from another (a frame taken from a video) came from where its source did.
UPDATE case_files f SET case_request_id = s.case_request_id, collection_id = s.collection_id, vestige_id = s.vestige_id,
    updated_at = NOW()
FROM case_files s
WHERE f.id = @id::bigint AND s.id = @source_id::bigint AND s.biometric_case_id = f.biometric_case_id
  AND f.case_request_id IS NULL AND f.collection_id IS NULL;
