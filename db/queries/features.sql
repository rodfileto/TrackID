-- name: UpsertBiometricFeatureFromIdentityFile :one
INSERT INTO biometric_features (feature_type, provenance, identity_file_id)
VALUES ($1, $2, $3)
ON CONFLICT (identity_file_id) DO UPDATE SET
    feature_type = EXCLUDED.feature_type,
    provenance = EXCLUDED.provenance,
    updated_at = NOW()
RETURNING id;

-- name: UpsertBiometricFeatureFromCaseTrace :one
INSERT INTO biometric_features (feature_type, provenance, case_trace_id)
VALUES ($1, $2, $3)
ON CONFLICT (case_trace_id) DO UPDATE SET
    feature_type = EXCLUDED.feature_type,
    provenance = EXCLUDED.provenance,
    updated_at = NOW()
RETURNING id;

-- name: ListBiometricFeatures :many
SELECT id, feature_type, provenance, identity_file_id, case_trace_id
FROM biometric_features
ORDER BY id;

-- name: UpsertFeatureEmbedding :one
-- embedding is passed as pgvector's text input format ("[v1,v2,...]") and cast explicitly,
-- so no pgvector-aware driver/codec is needed -- consistent with this package's plain
-- database/sql usage elsewhere.
INSERT INTO feature_embeddings (biometric_feature_id, embedding_type, embedding, model_version)
VALUES (sqlc.arg(biometric_feature_id), sqlc.arg(embedding_type), sqlc.arg(embedding)::vector, sqlc.arg(model_version))
ON CONFLICT (biometric_feature_id, embedding_type) DO UPDATE SET
    embedding = EXCLUDED.embedding,
    model_version = EXCLUDED.model_version,
    matched_at = NULL,
    updated_at = NOW()
RETURNING id;

-- name: GetCodificationSource :one
-- Resolves the image to embed for a codification, in priority order: the
-- manually adjusted codification_image (case_codifications.case_file_id),
-- the trace's own face_crop (case_traces.case_file_id, set by an automated
-- import pipeline that already produced one), or -- for a trace marked by
-- hand and codified with neither -- the evidence image plus the trace's own
-- box (box_x1..y2), letting the caller crop+pad around just that face.
-- Also returns the biometric feature this codification's trace already has
-- (created at trace-marking time by UpsertBiometricFeatureFromCaseTrace) --
-- feature_embeddings hangs off it.
SELECT
    cod.id AS codification_id,
    cod.codification_type,
    bf.id AS biometric_feature_id,
    ccf.storage_ref AS codification_storage_ref,
    tcf.storage_ref AS trace_crop_storage_ref,
    ecf.storage_ref AS evidence_storage_ref,
    tr.box_x1, tr.box_y1, tr.box_x2, tr.box_y2,
    -- The version the matcher uses for this face: the one chosen, or the original.
    (cod.id = COALESCE(tr.recognition_codification_id,
        (SELECT c1.id FROM case_codifications c1 WHERE c1.trace_id = tr.id AND c1.sequence = 1)))::boolean AS for_recognition
FROM case_codifications cod
JOIN case_traces tr ON tr.id = cod.trace_id
JOIN case_evidences ce ON ce.id = tr.evidence_id
JOIN biometric_features bf ON bf.case_trace_id = tr.id
LEFT JOIN case_files ccf ON ccf.id = cod.case_file_id
LEFT JOIN case_files tcf ON tcf.id = tr.case_file_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
WHERE cod.id = $1;

-- name: GetCodificationForComparison :one
-- One codification of a case with what comparing it needs: its stored
-- embedding of the given type when there is one, and otherwise the images to
-- compute one from -- the analyst's adjusted codification image if saved,
-- else the evidence image plus the trace's box.
SELECT
    cd.id AS codification_id,
    cd.codification_type,
    ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2,
    ecf.storage_ref AS evidence_storage_ref,
    ccf.storage_ref AS codification_storage_ref,
    -- '' when there's no stored embedding (sqlc can't see the LEFT JOIN's NULL).
    -- The codification's own embedding; the face's (the matcher's) only for the version it is of.
    COALESCE(cemb.embedding::text,
        CASE WHEN cd.id = COALESCE(ct.recognition_codification_id,
            (SELECT c1.id FROM case_codifications c1 WHERE c1.trace_id = ct.id AND c1.sequence = 1))
        THEN fe.embedding::text END, '')::text AS embedding
FROM case_codifications cd
JOIN case_traces ct ON ct.id = cd.trace_id
JOIN case_evidences ce ON ce.id = ct.evidence_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
LEFT JOIN case_files ccf ON ccf.id = cd.case_file_id
LEFT JOIN codification_embeddings cemb ON cemb.codification_id = cd.id AND cemb.embedding_type = sqlc.arg(embedding_type)
LEFT JOIN biometric_features bf ON bf.case_trace_id = ct.id
LEFT JOIN feature_embeddings fe ON fe.biometric_feature_id = bf.id AND fe.embedding_type = sqlc.arg(embedding_type)
WHERE cd.id = sqlc.arg(codification_id) AND ce.biometric_case_id = sqlc.arg(biometric_case_id);

-- name: UpsertCodificationEmbedding :exec
INSERT INTO codification_embeddings (codification_id, embedding_type, embedding, model_version)
VALUES (sqlc.arg(codification_id), sqlc.arg(embedding_type), sqlc.arg(embedding)::vector, sqlc.arg(model_version))
ON CONFLICT (codification_id, embedding_type) DO UPDATE SET
    embedding = EXCLUDED.embedding,
    model_version = EXCLUDED.model_version,
    updated_at = NOW();

-- name: GetCodificationEmbedding :one
SELECT embedding::text AS embedding, model_version
FROM codification_embeddings
WHERE codification_id = $1 AND embedding_type = $2;

-- name: ListUnmatchedFeatureEmbeddings :many
SELECT id, biometric_feature_id, embedding_type, embedding::text AS embedding, model_version
FROM feature_embeddings
WHERE embedding_type = $1 AND matched_at IS NULL AND match_path = 'SYNC'
ORDER BY id;

-- name: FindNearestFeatureEmbeddings :many
SELECT fe.id, fe.biometric_feature_id, fe.embedding <=> sqlc.arg(embedding)::vector AS distance
FROM feature_embeddings fe
WHERE fe.embedding_type = sqlc.arg(embedding_type) AND fe.id != sqlc.arg(exclude_id)
ORDER BY fe.embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(result_limit);

-- name: FindNearestPersonsByFaceEmbedding :many
-- Nearest enrolled (KNOWN) face embeddings to a query vector, joined up to
-- the person/register/document that enrolled them -- the face-search
-- counterpart to SearchPersonsByName. Only KNOWN features match: a
-- QUESTIONED (case_trace) embedding has no identity_files row, so the join
-- to identity_files excludes it. A person can surface more than once, once
-- per matching register, same as SearchPersonsByName. Also returns the
-- matched identity_files itself (id + content_type) -- the enrollment photo
-- the embedding was computed from -- so a caller can render it as a
-- thumbnail (see DownloadIdentityFileHandler).
SELECT p.person_id, r.name, r.register_number, d.document_type, d.document_number,
       f.id AS identity_file_id, f.content_type,
       fe.embedding <=> sqlc.arg(embedding)::vector AS distance
FROM feature_embeddings fe
JOIN biometric_features bf ON bf.id = fe.biometric_feature_id
JOIN identity_files f ON f.id = bf.identity_file_id
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE fe.embedding_type = sqlc.arg(embedding_type)
ORDER BY fe.embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(result_limit);

-- name: FindNearestCasesByFaceEmbedding :many
-- Nearest QUESTIONED (case evidence) face embeddings to a query vector,
-- joined up to the biometric case that owns the matching trace -- the
-- case-evidence counterpart to FindNearestPersonsByFaceEmbedding. Only
-- QUESTIONED features match: a KNOWN (identity_files) embedding has no
-- case_trace_id, so the join to case_traces excludes it. A case can surface
-- more than once, once per matching trace -- callers dedupe per case
-- themselves (see person.SearchByFace), keeping the best-scoring trace.
--
-- Also resolves what a caller needs to render the matched trace as a
-- thumbnail, same priority GetCodificationSource already uses: the trace's
-- own face_crop file (trace_crop_file_id) when an automated import pipeline
-- already produced one -- already just the face, no box needed -- or
-- otherwise the evidence file (evidence_file_id) plus the trace's own box
-- to crop it down to just this face.
SELECT bc.case_id, bc.modality, bc.description, ct.id AS case_trace_id,
       tcf.id AS trace_crop_file_id, ecf.id AS evidence_file_id,
       ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2,
       fe.embedding <=> sqlc.arg(embedding)::vector AS distance
FROM feature_embeddings fe
JOIN biometric_features bf ON bf.id = fe.biometric_feature_id
JOIN case_traces ct ON ct.id = bf.case_trace_id
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
LEFT JOIN case_files tcf ON tcf.id = ct.case_file_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
WHERE fe.embedding_type = sqlc.arg(embedding_type)
ORDER BY fe.embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(result_limit);

-- name: GetIdentityFeatureSource :one
-- Resolves the image to embed for a KNOWN identity biometric_features: its
-- identity_files's stored photo. The identity-enrollment counterpart to
-- GetCodificationSource -- an identity_files row IS the face record already (a
-- mugshot/ID photo), so there's no codification/crop indirection to resolve.
SELECT bf.id AS biometric_feature_id, bf.feature_type, f.storage_ref
FROM biometric_features bf
JOIN identity_files f ON f.id = bf.identity_file_id
WHERE bf.id = sqlc.arg(biometric_feature_id);

-- name: ListKnownFaceFeaturesMissingEmbedding :many
-- Every KNOWN FACE_RECORD biometric_features (an enrolled identity photo) with
-- no embedding of the given type yet -- what
-- cmd/backfill-identity-face-embeddings enqueues embedding computation for.
SELECT bf.id
FROM biometric_features bf
WHERE bf.feature_type = 'FACE_RECORD'
  AND bf.identity_file_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM feature_embeddings fe
      WHERE fe.biometric_feature_id = bf.id AND fe.embedding_type = sqlc.arg(embedding_type)
  )
ORDER BY bf.id;

-- name: MarkFeatureEmbeddingMatched :exec
UPDATE feature_embeddings SET matched_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: UpsertBiometricTemplate :one
-- Re-extraction replaces the template and clears matched_at, so the new template goes
-- through matching again (as UpsertFeatureEmbedding does for embeddings).
INSERT INTO biometric_templates (biometric_feature_id, template_type, template, model_version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (biometric_feature_id, template_type) DO UPDATE SET
    template = EXCLUDED.template,
    model_version = EXCLUDED.model_version,
    matched_at = NULL,
    updated_at = NOW()
RETURNING id;

-- name: ListBiometricTemplates :many
-- Every template of a type: the gallery biometricmatch.RunTemplates scores each unmatched
-- template against. matched_at IS NULL marks the ones still to be probed.
SELECT id, biometric_feature_id, template, (matched_at IS NULL)::boolean AS unmatched
FROM biometric_templates
WHERE template_type = $1
ORDER BY id;

-- name: MarkBiometricTemplateMatched :exec
UPDATE biometric_templates SET matched_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: ListKnownFingerprintFeaturesMissingTemplate :many
-- Every KNOWN FINGERPRINT_TEMPLATE biometric_features (an enrolled ten-print) with no
-- template of the given type yet -- what cmd/backfill-fingerprint-templates enqueues
-- extraction for.
SELECT bf.id
FROM biometric_features bf
WHERE bf.feature_type = 'FINGERPRINT_TEMPLATE'
  AND bf.identity_file_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM biometric_templates bt
      WHERE bt.biometric_feature_id = bf.id AND bt.template_type = sqlc.arg(template_type)
  )
ORDER BY bf.id;

-- name: GetCurrentMatchThreshold :one
SELECT id, embedding_type, review_threshold, confirm_threshold, source, created_by, created_at
FROM match_thresholds
WHERE embedding_type = $1
ORDER BY id DESC
LIMIT 1;

-- name: InsertMatchThreshold :one
INSERT INTO match_thresholds (embedding_type, review_threshold, confirm_threshold, source, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, embedding_type, review_threshold, confirm_threshold, source, created_by, created_at;

-- name: ListMatchThresholds :many
SELECT id, embedding_type, review_threshold, confirm_threshold, source, created_by, created_at
FROM match_thresholds
WHERE embedding_type = $1
ORDER BY id DESC;

-- name: CountUnmatchedFeatureEmbeddings :one
-- The per-face sync's backlog: BULK embeddings wait for cmd/bulk-match instead.
SELECT COUNT(*) FROM feature_embeddings WHERE embedding_type = $1 AND matched_at IS NULL AND match_path = 'SYNC';

-- name: CountUnmatchedBiometricTemplates :one
SELECT COUNT(*) FROM biometric_templates WHERE template_type = $1 AND matched_at IS NULL;
