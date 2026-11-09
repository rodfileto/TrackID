-- name: UpsertBiometricCase :one
-- (xmax = 0) tells a real insert apart from a row that already existed: RowsAffected() is
-- 1 either way, so callers that need to report insert-vs-update counts need this instead.
-- A new case takes the unit named, or the one that receives new cases. An existing one keeps its own,
-- so running an import again doesn't undo a transfer.
INSERT INTO biometric_cases (case_id, case_type, modality, description, unit_id)
VALUES ($1, $2, $3, $4, COALESCE(sqlc.narg('unit_id')::bigint, default_case_unit()))
ON CONFLICT (case_id) DO UPDATE SET
    case_type = EXCLUDED.case_type,
    modality = EXCLUDED.modality,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id, (xmax = 0) AS inserted;

-- name: MaxCaseNumberForYear :one
-- Returns the highest case_number already used for a year, or 0 when none yet.
SELECT COALESCE(MAX(case_number), 0)::int4 AS max_number
FROM biometric_cases
WHERE case_year = sqlc.arg(case_year)::int4;

-- name: CreateBiometricCase :one
INSERT INTO biometric_cases (case_id, case_type, modality, description, case_year, case_number)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, case_id, case_type, modality, description;

-- name: GetBiometricCaseByCaseID :one
SELECT id, case_id, case_type, modality, description, report_mode
FROM biometric_cases
WHERE case_id = $1;

-- name: ListCaseFilesByBiometricCase :many
-- With where each file came from, if recorded: the request that sent it in or the collection that
-- gathered it, and the vestige it was taken from.
SELECT f.id, f.biometric_case_id, f.category, f.media_type, f.hash_id, f.filename, f.source_path, f.storage_ref,
       f.content_type, f.size_bytes, f.created_at,
       r.id AS request_id, r.document_reference AS request_document, r.received_on AS request_received_on,
       cc.id AS collection_id, cc.sequence AS collection_sequence, cc.collected_from AS collection_collected_from,
       v.id AS vestige_id, v.sequence AS vestige_sequence
FROM case_files f
LEFT JOIN case_requests r ON r.id = f.case_request_id
LEFT JOIN case_collections cc ON cc.id = f.collection_id
LEFT JOIN case_vestiges v ON v.id = f.vestige_id
WHERE f.biometric_case_id = $1
ORDER BY f.id;

-- name: GetCaseFile :one
SELECT id, category, filename, storage_ref, content_type
FROM case_files
WHERE id = $1 AND biometric_case_id = $2;

-- name: GetCaseFileByHash :one
SELECT id, category, media_type, filename, storage_ref, content_type, size_bytes, created_at
FROM case_files
WHERE biometric_case_id = $1 AND category = $2 AND hash_id = $3;

-- name: CountCaseTracesByCaseFile :one
-- Traces marked on an evidence file, via case_evidences.case_file_id -- used
-- to guard against deleting an evidence file that traces (and their
-- biometric_features/embeddings) still depend on.
SELECT count(*) FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
WHERE ce.case_file_id = $1;

-- name: DeleteCaseFile :execrows
DELETE FROM case_files WHERE id = $1 AND biometric_case_id = $2 AND category = 'evidence';

-- name: UpsertCaseFile :one
INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (biometric_case_id, category, hash_id) DO UPDATE SET
    media_type = EXCLUDED.media_type,
    filename = EXCLUDED.filename,
    source_path = EXCLUDED.source_path,
    storage_ref = EXCLUDED.storage_ref,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    updated_at = NOW()
RETURNING id, created_at;

-- name: CreateCaseFile :one
INSERT INTO case_files (biometric_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, created_at;

-- name: UpsertCaseEvidence :one
INSERT INTO case_evidences (biometric_case_id, sequence, case_file_id, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (biometric_case_id, sequence) DO UPDATE SET
    case_file_id = EXCLUDED.case_file_id,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id;

-- name: GetCaseEvidenceByCaseFile :one
SELECT id FROM case_evidences
WHERE biometric_case_id = $1 AND case_file_id = $2;

-- name: CreateCaseEvidenceForFile :one
-- Case files added through AddEvidence only get a case_files row -- case_traces
-- hangs off case_evidences (MODEL.md section 2.2), so trace detection creates
-- the case_evidences row for a case_file on first use. sequence is the next
-- free slot for the case, since case_files added this way never carry one.
INSERT INTO case_evidences (biometric_case_id, sequence, case_file_id)
SELECT $1, COALESCE(MAX(sequence), 0) + 1, $2
FROM case_evidences
WHERE biometric_case_id = $1
RETURNING id;

-- name: MaxCaseTraceSequence :one
SELECT COALESCE(MAX(sequence), 0)::smallint AS max_sequence
FROM case_traces
WHERE evidence_id = $1;

-- name: ListCaseTracesByCaseFile :many
SELECT ct.id, ct.sequence, ct.trace_type, ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2, ct.detection_score,
    bf.id AS feature_id
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
LEFT JOIN biometric_features bf ON bf.case_trace_id = ct.id
WHERE ce.biometric_case_id = $1 AND ce.case_file_id = $2
ORDER BY ct.sequence;

-- name: ListCaseCodificationsByBiometricCase :many
-- Every codification recorded across every trace of a case, with enough
-- about its trace (box, sequence), the trace's evidence file (sequence, id,
-- filename) and its own saved image (case_file_id, if the analyst adjusted
-- and saved one via SaveCodificationImage) for a caller to render a
-- thumbnail and label it "<evidence sequence>-<trace sequence>-<codification
-- sequence>" without a second round trip per codification.
SELECT cd.id AS codification_id, cd.sequence AS codification_sequence, cd.codification_type, cd.case_file_id AS codification_file_id,
    ct.id AS trace_id, ct.sequence AS trace_sequence, ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2,
    ce.sequence AS evidence_sequence, cf.id AS evidence_file_id, cf.filename AS evidence_filename,
    cd.label,
    (cd.id = COALESCE(ct.recognition_codification_id,
        (SELECT c1.id FROM case_codifications c1 WHERE c1.trace_id = ct.id AND c1.sequence = 1)))::boolean AS for_recognition
FROM case_codifications cd
JOIN case_traces ct ON ct.id = cd.trace_id
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN case_files cf ON cf.id = ce.case_file_id
WHERE ce.biometric_case_id = $1
ORDER BY ce.sequence, ct.sequence, cd.sequence;

-- name: DeleteCaseTrace :execrows
-- Scoped to the case + evidence file so a trace can only be deleted through
-- the case/evidence it actually belongs to. case_codifications and
-- biometric_features (and, through it, feature_embeddings) cascade off
-- case_traces, so this is the only delete needed to fully remove a trace.
DELETE FROM case_traces ct
USING case_evidences ce
WHERE ct.id = $1
  AND ct.evidence_id = ce.id
  AND ce.biometric_case_id = $2
  AND ce.case_file_id = $3;

-- name: UpsertCaseTrace :one
INSERT INTO case_traces (evidence_id, sequence, trace_type, box_x1, box_y1, box_x2, box_y2, detection_score, case_file_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (evidence_id, sequence) DO UPDATE SET
    trace_type = EXCLUDED.trace_type,
    box_x1 = EXCLUDED.box_x1,
    box_y1 = EXCLUDED.box_y1,
    box_x2 = EXCLUDED.box_x2,
    box_y2 = EXCLUDED.box_y2,
    detection_score = EXCLUDED.detection_score,
    case_file_id = EXCLUDED.case_file_id,
    updated_at = NOW()
RETURNING id;

-- name: UpsertCaseCodification :one
INSERT INTO case_codifications (trace_id, sequence, codification_type)
VALUES ($1, $2, $3)
ON CONFLICT (trace_id, sequence) DO UPDATE SET
    codification_type = EXCLUDED.codification_type,
    updated_at = NOW()
RETURNING id;

-- name: GetCaseCodificationByTrace :one
SELECT id FROM case_codifications WHERE trace_id = $1 AND sequence = 1;

-- name: SetCaseCodificationFile :exec
UPDATE case_codifications SET case_file_id = $2, updated_at = NOW() WHERE id = $1;

-- name: GetCaseTraceForFile :one
-- Scopes a trace to the case + evidence file it belongs to, the same way
-- GetCaseFile scopes a file to a case -- used to authorize codification/point
-- operations reached via /cases/:caseId/evidences/:evidenceId/traces/:traceId.
SELECT ct.id, ct.trace_type
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
WHERE ct.id = $1 AND ce.biometric_case_id = $2 AND ce.case_file_id = $3;

-- name: MaxCodificationPointSequence :one
SELECT COALESCE(MAX(sequence), 0)::smallint AS max_sequence
FROM case_codification_points
WHERE codification_id = $1;

-- name: CreateCodificationPoint :one
INSERT INTO case_codification_points (codification_id, sequence, x, y, point_type, angle)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: ListCodificationPoints :many
SELECT id, sequence, x, y, point_type, angle
FROM case_codification_points
WHERE codification_id = $1
ORDER BY sequence;

-- name: UpdateCodificationPoint :execrows
UPDATE case_codification_points
SET x = $3, y = $4, point_type = $5, angle = $6, updated_at = NOW()
WHERE id = $1 AND codification_id = $2;

-- name: DeleteCodificationPoint :execrows
DELETE FROM case_codification_points
WHERE id = $1 AND codification_id = $2;

-- Codification versions (case_codifications, db/migrations/005_cases.sql).

-- name: ListTraceCodifications :many
SELECT cd.id, cd.sequence, cd.codification_type, cd.label, cd.recipe, cd.case_file_id, cd.created_at,
    u.id AS created_by_id, u.username AS created_by_username,
    COALESCE(TRIM(u.first_name || ' ' || u.last_name), '')::text AS created_by_name,
    cf.hash_id AS image_sha256,
    EXISTS (SELECT 1 FROM codification_embeddings e WHERE e.codification_id = cd.id)::boolean AS has_embedding,
    (cd.id = COALESCE(ct.recognition_codification_id,
        (SELECT c1.id FROM case_codifications c1 WHERE c1.trace_id = ct.id AND c1.sequence = 1)))::boolean AS for_recognition
FROM case_codifications cd
JOIN case_traces ct ON ct.id = cd.trace_id
LEFT JOIN users u ON u.id = cd.created_by
LEFT JOIN case_files cf ON cf.id = cd.case_file_id
WHERE cd.trace_id = $1
ORDER BY cd.sequence;

-- name: NextCodificationSequence :one
SELECT (COALESCE(MAX(sequence), 0) + 1)::smallint AS next FROM case_codifications WHERE trace_id = $1;

-- name: InsertCodificationVersion :one
INSERT INTO case_codifications (trace_id, sequence, codification_type, label, recipe, created_by, case_file_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetTraceCodification :one
SELECT cd.id, cd.sequence, cd.codification_type, cd.case_file_id, cf.storage_ref, cf.content_type
FROM case_codifications cd
LEFT JOIN case_files cf ON cf.id = cd.case_file_id
WHERE cd.id = $1 AND cd.trace_id = $2;

-- name: SetTraceRecognitionCodification :exec
UPDATE case_traces SET recognition_codification_id = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteCodificationVersion :execrows
-- Only a rendered version (never the original), and not the one the matcher uses.
DELETE FROM case_codifications cd
USING case_traces ct
WHERE cd.id = $1 AND cd.trace_id = $2 AND ct.id = cd.trace_id AND cd.sequence > 1
  AND ct.recognition_codification_id IS DISTINCT FROM cd.id;

-- name: LockBiometricCase :one
-- Serializes the analyses of one case, so two at once cannot take the same sequence.
SELECT id FROM biometric_cases WHERE id = $1 FOR UPDATE;

-- name: GetCaseByCaseID :one
SELECT id, case_id, modality FROM biometric_cases WHERE case_id = $1;

-- name: ListCaseTraceIDs :many
SELECT ct.id
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
WHERE ce.biometric_case_id = $1;

-- name: ListCaseTraceMaterial :many
-- Every trace of a case with its evidence file, role and suitability.
SELECT ct.id, ct.sequence, ce.sequence AS evidence_sequence, ce.case_file_id, ct.trace_type, ct.trace_role,
       ct.suitability, ct.suitability_notes, su.username AS suitability_by, ct.suitability_at,
       ct.case_file_id AS crop_file_id, ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2,
       vf.filename AS source_video, vf.hash_id AS source_video_sha256, ce.source_timestamp_ms, ce.source_frame
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
LEFT JOIN users su ON su.id = ct.suitability_by
LEFT JOIN case_files vf ON vf.id = ce.source_video_file_id
WHERE bc.case_id = $1
ORDER BY ce.sequence, ct.sequence;

-- name: GetCaseTraceOfCase :one
SELECT ct.id, ct.trace_role, ct.suitability
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
WHERE ct.id = $1 AND ce.biometric_case_id = $2;

-- name: SetCaseTraceRole :exec
-- A reference carries no suitability: its value is not assessed.
UPDATE case_traces SET trace_role = $2,
    suitability = CASE WHEN $2 = 'REFERENCE' THEN NULL ELSE suitability END,
    suitability_notes = CASE WHEN $2 = 'REFERENCE' THEN NULL ELSE suitability_notes END,
    suitability_by = CASE WHEN $2 = 'REFERENCE' THEN NULL ELSE suitability_by END,
    suitability_at = CASE WHEN $2 = 'REFERENCE' THEN NULL ELSE suitability_at END,
    updated_at = NOW()
WHERE id = $1;

-- name: SetCaseTraceSuitability :exec
UPDATE case_traces SET suitability = $2, suitability_notes = $3, suitability_by = $4, suitability_at = NOW(), updated_at = NOW()
WHERE id = $1 AND trace_role = 'QUESTIONED';

-- Video (docs/facial-reports-plan.md, phase 5).

-- name: InsertCaseVideo :exec
INSERT INTO case_videos (case_file_id) VALUES ($1) ON CONFLICT (case_file_id) DO NOTHING;

-- name: StartCaseVideo :exec
UPDATE case_videos SET status = 'PROCESSING', error = NULL WHERE case_file_id = $1;

-- name: FinishCaseVideo :exec
UPDATE case_videos SET status = 'DONE', duration_seconds = $2, frame_rate = $3, tracks = $4, error = NULL, processed_at = NOW()
WHERE case_file_id = $1;

-- name: ResetCaseVideo :exec
UPDATE case_videos SET status = 'PENDING', error = NULL WHERE case_file_id = $1;

-- name: FailCaseVideo :exec
UPDATE case_videos SET status = 'FAILED', error = $2, processed_at = NOW() WHERE case_file_id = $1;

-- name: GetCaseVideo :one
SELECT v.case_file_id, v.status, v.duration_seconds, v.frame_rate, v.tracks, v.error, v.created_at, v.processed_at,
       f.biometric_case_id, f.filename, f.hash_id, f.storage_ref, f.content_type, f.size_bytes, bc.case_id
FROM case_videos v
JOIN case_files f ON f.id = v.case_file_id
JOIN biometric_cases bc ON bc.id = f.biometric_case_id
WHERE v.case_file_id = $1;

-- name: ListCaseVideos :many
SELECT v.case_file_id, v.status, v.duration_seconds, v.frame_rate, v.tracks, v.error, v.created_at, v.processed_at,
       f.filename, f.hash_id, f.content_type, f.size_bytes, f.case_request_id, f.collection_id
FROM case_videos v
JOIN case_files f ON f.id = v.case_file_id
JOIN biometric_cases bc ON bc.id = f.biometric_case_id
WHERE bc.case_id = $1
ORDER BY v.case_file_id;

-- name: SetEvidenceVideoSource :execrows
UPDATE case_evidences SET source_video_file_id = $2, source_timestamp_ms = $3, source_frame = $4, updated_at = NOW()
WHERE case_file_id = $1;
