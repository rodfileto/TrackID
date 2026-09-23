-- name: CreateUser :one
INSERT INTO users (nome, ultimo_nome, matricula, cargo, username, email, password_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, nome, ultimo_nome, matricula, cargo, username, email;

-- name: GetUserByMatricula :one
SELECT id, nome, ultimo_nome, matricula, cargo, username, email, password_hash
FROM users
WHERE matricula = $1;

-- name: GetUserByID :one
SELECT id, nome, ultimo_nome, matricula, cargo, username, email
FROM users
WHERE id = $1;

-- name: UpsertBiometricCase :one
-- (xmax = 0) tells a real insert apart from a row that already existed: RowsAffected() is
-- 1 either way, so callers that need to report insert-vs-update counts need this instead.
INSERT INTO biometric_cases (case_id, case_type, modality, description)
VALUES ($1, $2, $3, $4)
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
RETURNING case_id, case_type, modality, description;

-- name: CountBiometricCases :one
SELECT COUNT(*) FROM biometric_cases;

-- name: ListBiometricCases :many
SELECT case_id, case_type, modality, description
FROM biometric_cases
ORDER BY case_id
LIMIT $1 OFFSET $2;

-- name: GetBiometricCaseByCaseID :one
SELECT id, case_id, case_type, modality, description
FROM biometric_cases
WHERE case_id = $1;

-- name: UpdateBiometricCaseDescription :exec
UPDATE biometric_cases SET description = $2, updated_at = NOW()
WHERE id = $1;

-- name: ListBiometricCaseIDsByModality :many
SELECT case_id FROM biometric_cases WHERE modality = $1 ORDER BY case_id;

-- name: UpsertPerson :one
INSERT INTO person (person_id, meta)
VALUES ($1, $2)
ON CONFLICT (person_id) DO UPDATE SET
    meta = EXCLUDED.meta,
    updated_at = NOW()
RETURNING id;

-- name: UpsertIdentityDocument :one
INSERT INTO identity_document (person_id, document_number, document_type, fiscal_number)
VALUES ($1, $2, $3, $4)
ON CONFLICT (document_type, document_number) DO UPDATE SET
    person_id = EXCLUDED.person_id,
    fiscal_number = EXCLUDED.fiscal_number,
    updated_at = NOW()
RETURNING id;

-- name: UpsertIdentityRegister :one
INSERT INTO identity_register
    (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender, birth_date, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (register_number) DO UPDATE SET
    document_id = EXCLUDED.document_id,
    name = EXCLUDED.name,
    parent_1_name = EXCLUDED.parent_1_name,
    parent_1_gender = EXCLUDED.parent_1_gender,
    parent_2_name = EXCLUDED.parent_2_name,
    parent_2_gender = EXCLUDED.parent_2_gender,
    birth_date = EXCLUDED.birth_date,
    meta = EXCLUDED.meta,
    updated_at = NOW()
RETURNING id;

-- name: UpsertIdentityFile :one
INSERT INTO identity_file (register_id, file_type, sequence, source_path, storage_ref, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (register_id, file_type, sequence) DO UPDATE SET
    source_path = EXCLUDED.source_path,
    storage_ref = EXCLUDED.storage_ref,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    updated_at = NOW()
RETURNING id;

-- name: UpsertBiometricFeatureFromIdentityFile :one
INSERT INTO biometricfeature (feature_type, provenance, identity_file_id)
VALUES ($1, $2, $3)
ON CONFLICT (identity_file_id) DO UPDATE SET
    feature_type = EXCLUDED.feature_type,
    provenance = EXCLUDED.provenance,
    updated_at = NOW()
RETURNING id;

-- name: UpsertBiometricFeatureFromCaseTrace :one
INSERT INTO biometricfeature (feature_type, provenance, case_trace_id)
VALUES ($1, $2, $3)
ON CONFLICT (case_trace_id) DO UPDATE SET
    feature_type = EXCLUDED.feature_type,
    provenance = EXCLUDED.provenance,
    updated_at = NOW()
RETURNING id;

-- name: InsertBiometricDecision :one
INSERT INTO biometric_decisions
    (feature_a_id, feature_b_id, modality, role, decision, system_source, username, confidence, threshold, notes,
     comparison_type, related_reference, related_reference_kind, responsible_user)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, feature_a_id, feature_b_id, modality, role, decision, system_source, username, confidence, threshold, notes,
    comparison_type, related_reference, related_reference_kind, responsible_user, decided_at, created_at;

-- name: ListBiometricDecisions :many
SELECT feature_a_id, feature_b_id, modality, role, decision, system_source, username, confidence
FROM biometric_decisions
ORDER BY feature_a_id, feature_b_id, decided_at;

-- name: ListBiometricFeatures :many
SELECT id, feature_type, provenance, identity_file_id, case_trace_id
FROM biometricfeature
ORDER BY id;

-- name: ListCaseFilesByBiometricCase :many
SELECT id, biometric_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes, created_at
FROM case_files
WHERE biometric_case_id = $1
ORDER BY id;

-- name: GetCaseFile :one
SELECT id, category, filename, storage_ref, content_type
FROM case_files
WHERE id = $1 AND biometric_case_id = $2;

-- name: CountCaseTracesByCaseFile :one
-- Traces marked on an evidence file, via case_evidences.case_file_id -- used
-- to guard against deleting an evidence file that traces (and their
-- biometricfeature/embeddings) still depend on.
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
LEFT JOIN biometricfeature bf ON bf.case_trace_id = ct.id
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
    ce.sequence AS evidence_sequence, cf.id AS evidence_file_id, cf.filename AS evidence_filename
FROM case_codifications cd
JOIN case_traces ct ON ct.id = cd.trace_id
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN case_files cf ON cf.id = ce.case_file_id
WHERE ce.biometric_case_id = $1
ORDER BY ce.sequence, ct.sequence, cd.sequence;

-- name: DeleteCaseTrace :execrows
-- Scoped to the case + evidence file so a trace can only be deleted through
-- the case/evidence it actually belongs to. case_codifications and
-- biometricfeature (and, through it, feature_embeddings) cascade off
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

-- name: UpsertFeatureEmbedding :one
-- embedding is passed as pgvector's text input format ("[v1,v2,...]") and cast explicitly,
-- so no pgvector-aware driver/codec is needed -- consistent with this package's plain
-- database/sql usage elsewhere.
INSERT INTO feature_embeddings (biometricfeature_id, embedding_type, embedding, model_version)
VALUES (sqlc.arg(biometricfeature_id), sqlc.arg(embedding_type), sqlc.arg(embedding)::vector, sqlc.arg(model_version))
ON CONFLICT (biometricfeature_id, embedding_type) DO UPDATE SET
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
-- Also returns the biometricfeature this codification's trace already has
-- (created at trace-marking time by UpsertBiometricFeatureFromCaseTrace) --
-- feature_embeddings hangs off it.
SELECT
    cod.id AS codification_id,
    cod.codification_type,
    bf.id AS biometricfeature_id,
    ccf.storage_ref AS codification_storage_ref,
    tcf.storage_ref AS trace_crop_storage_ref,
    ecf.storage_ref AS evidence_storage_ref,
    tr.box_x1, tr.box_y1, tr.box_x2, tr.box_y2
FROM case_codifications cod
JOIN case_traces tr ON tr.id = cod.trace_id
JOIN case_evidences ce ON ce.id = tr.evidence_id
JOIN biometricfeature bf ON bf.case_trace_id = tr.id
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
    COALESCE(fe.embedding::text, '')::text AS embedding
FROM case_codifications cd
JOIN case_traces ct ON ct.id = cd.trace_id
JOIN case_evidences ce ON ce.id = ct.evidence_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
LEFT JOIN case_files ccf ON ccf.id = cd.case_file_id
LEFT JOIN biometricfeature bf ON bf.case_trace_id = ct.id
LEFT JOIN feature_embeddings fe ON fe.biometricfeature_id = bf.id AND fe.embedding_type = sqlc.arg(embedding_type)
WHERE cd.id = sqlc.arg(codification_id) AND ce.biometric_case_id = sqlc.arg(biometric_case_id);

-- name: ListUnmatchedFeatureEmbeddings :many
SELECT id, biometricfeature_id, embedding_type, embedding::text AS embedding, model_version
FROM feature_embeddings
WHERE embedding_type = $1 AND matched_at IS NULL
ORDER BY id;

-- name: FindNearestFeatureEmbeddings :many
SELECT fe.id, fe.biometricfeature_id, fe.embedding <=> sqlc.arg(embedding)::vector AS distance
FROM feature_embeddings fe
WHERE fe.embedding_type = sqlc.arg(embedding_type) AND fe.id != sqlc.arg(exclude_id)
ORDER BY fe.embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(result_limit);

-- name: FindNearestPersonsByFaceEmbedding :many
-- Nearest enrolled (KNOWN) face embeddings to a query vector, joined up to
-- the person/register/document that enrolled them -- the face-search
-- counterpart to SearchPersonsByName. Only KNOWN features match: a
-- QUESTIONED (case_trace) embedding has no identity_file row, so the join
-- to identity_file excludes it. A person can surface more than once, once
-- per matching register, same as SearchPersonsByName. Also returns the
-- matched identity_file itself (id + content_type) -- the enrollment photo
-- the embedding was computed from -- so a caller can render it as a
-- thumbnail (see DownloadIdentityFileHandler).
SELECT p.person_id, r.name, r.register_number, d.document_type, d.document_number,
       f.id AS identity_file_id, f.content_type,
       fe.embedding <=> sqlc.arg(embedding)::vector AS distance
FROM feature_embeddings fe
JOIN biometricfeature bf ON bf.id = fe.biometricfeature_id
JOIN identity_file f ON f.id = bf.identity_file_id
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE fe.embedding_type = sqlc.arg(embedding_type)
ORDER BY fe.embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(result_limit);

-- name: FindNearestCasesByFaceEmbedding :many
-- Nearest QUESTIONED (case evidence) face embeddings to a query vector,
-- joined up to the biometric case that owns the matching trace -- the
-- case-evidence counterpart to FindNearestPersonsByFaceEmbedding. Only
-- QUESTIONED features match: a KNOWN (identity_file) embedding has no
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
JOIN biometricfeature bf ON bf.id = fe.biometricfeature_id
JOIN case_traces ct ON ct.id = bf.case_trace_id
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
LEFT JOIN case_files tcf ON tcf.id = ct.case_file_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
WHERE fe.embedding_type = sqlc.arg(embedding_type)
ORDER BY fe.embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(result_limit);

-- name: GetIdentityFeatureSource :one
-- Resolves the image to embed for a KNOWN identity biometricfeature: its
-- identity_file's stored photo. The identity-enrollment counterpart to
-- GetCodificationSource -- an identity_file IS the face record already (a
-- mugshot/ID photo), so there's no codification/crop indirection to resolve.
SELECT bf.id AS biometricfeature_id, bf.feature_type, f.storage_ref
FROM biometricfeature bf
JOIN identity_file f ON f.id = bf.identity_file_id
WHERE bf.id = sqlc.arg(biometricfeature_id);

-- name: ListKnownFaceFeaturesMissingEmbedding :many
-- Every KNOWN FACE_RECORD biometricfeature (an enrolled identity photo) with
-- no embedding of the given type yet -- what
-- cmd/backfill-identity-face-embeddings enqueues embedding computation for.
SELECT bf.id
FROM biometricfeature bf
WHERE bf.feature_type = 'FACE_RECORD'
  AND bf.identity_file_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM feature_embeddings fe
      WHERE fe.biometricfeature_id = bf.id AND fe.embedding_type = sqlc.arg(embedding_type)
  )
ORDER BY bf.id;

-- name: MarkFeatureEmbeddingMatched :exec
UPDATE feature_embeddings SET matched_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: InsertCaseDecision :one
INSERT INTO case_decisions (biometric_case_id, decision, system_source, username, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, biometric_case_id, decision, system_source, username, notes, decided_at, created_at;

-- name: ListCaseDecisionsByBiometricCase :many
SELECT id, biometric_case_id, decision, system_source, username, notes, decided_at, created_at
FROM case_decisions
WHERE biometric_case_id = $1
ORDER BY decided_at;

-- name: ListAllBiometricCases :many
SELECT case_id, case_type, modality, description
FROM biometric_cases
ORDER BY case_id;

-- name: ListQuestionedFeatures :many
SELECT bf.case_trace_id, bf.feature_type, bc.case_id
FROM biometricfeature bf
JOIN case_traces tr ON tr.id = bf.case_trace_id
JOIN case_evidences ev ON ev.id = tr.evidence_id
JOIN biometric_cases bc ON bc.id = ev.biometric_case_id
WHERE bf.provenance = 'QUESTIONED'
ORDER BY bf.case_trace_id;

-- name: ListPersonIDs :many
SELECT person_id FROM person ORDER BY person_id;

-- name: ListKnownIdentityChain :many
SELECT
    p.person_id,
    d.id AS document_id, d.document_number, d.document_type, d.fiscal_number,
    r.id AS register_id, r.register_number, r.name,
    r.parent_1_name, r.parent_1_gender, r.parent_2_name, r.parent_2_gender,
    r.birth_date,
    f.id AS identity_file_id, bf.feature_type, f.source_path, f.storage_ref, f.content_type, f.size_bytes
FROM biometricfeature bf
JOIN identity_file f ON f.id = bf.identity_file_id
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE bf.provenance = 'KNOWN'
ORDER BY bf.id;

-- name: ListKnownFeaturePersons :many
SELECT p.person_id, f.id AS identity_file_id
FROM biometricfeature bf
JOIN identity_file f ON f.id = bf.identity_file_id
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE bf.provenance = 'KNOWN';

-- name: ListClusters :many
SELECT id, modality
FROM clusters
ORDER BY id;

-- name: ListClusterMembers :many
SELECT cluster_id, feature_id
FROM cluster_members;

-- name: ListClusterMembersJoined :many
SELECT c.id AS cluster_id, c.modality, m.feature_id
FROM clusters c
JOIN cluster_members m ON m.cluster_id = c.id
ORDER BY c.id, m.feature_id;

-- name: CreateCluster :one
INSERT INTO clusters (modality) VALUES ($1) RETURNING id;

-- name: InsertClusterMember :exec
INSERT INTO cluster_members (cluster_id, feature_id) VALUES ($1, $2);

-- name: DeleteClusterMember :exec
DELETE FROM cluster_members WHERE feature_id = $1;

-- name: DeleteCluster :exec
DELETE FROM clusters WHERE id = $1;

-- name: InsertClusterMerge :exec
INSERT INTO cluster_merges (from_cluster_id, to_cluster_id, reason) VALUES ($1, $2, $3);

-- Person profile (see the person package): read-only views assembled from
-- the enrollment chain, cluster_members, and the evidence hierarchy. Nothing
-- here writes; a person's cluster/case links are always derived fresh from
-- biometric_decisions-backed cluster_members, never cached.

-- name: GetPersonByPersonID :one
SELECT id, person_id, meta, created_at, updated_at
FROM person
WHERE person_id = $1;

-- name: GetIdentityFileForPerson :one
-- Scopes an identity_file to the given person (through
-- identity_register -> identity_document), same scoping GetCaseFile does
-- for a case's evidence files -- so a caller can't download a file that
-- doesn't belong to the person named in the URL.
SELECT f.id, f.file_type, f.source_path, f.storage_ref, f.content_type
FROM identity_file f
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE f.id = sqlc.arg(identity_file_id) AND p.person_id = sqlc.arg(person_id);

-- name: ListIdentityChainByPerson :many
-- Every document -> register recorded for one person, left-joined down to
-- each register's files and the KNOWN biometricfeature a file yielded. Both
-- joins are LEFT so a register shows up even before any file has been
-- uploaded for it, and a file shows up even when its type (e.g. "pdf")
-- yields no feature -- a caller distinguishes the two by identity_file_id/
-- biometricfeature_id being NULL. Mirrors ListKnownIdentityChain's join
-- shape, scoped to one person instead of every person.
SELECT
    d.id AS document_id, d.document_number, d.document_type, d.fiscal_number,
    r.id AS register_id, r.register_number, r.name,
    r.parent_1_name, r.parent_1_gender, r.parent_2_name, r.parent_2_gender,
    r.birth_date, r.meta AS register_meta,
    f.id AS identity_file_id, f.file_type, f.sequence, f.source_path, f.storage_ref, f.content_type, f.size_bytes,
    bf.id AS biometricfeature_id, bf.feature_type
FROM identity_document d
JOIN identity_register r ON r.document_id = d.id
LEFT JOIN identity_file f ON f.register_id = r.id
LEFT JOIN biometricfeature bf ON bf.identity_file_id = f.id
WHERE d.person_id = $1
ORDER BY d.id, r.id, f.sequence;

-- name: ListClusterIDsForFeatureIDs :many
-- Resolves a set of graph feature ids (graph.KnownFeatureID /
-- graph.QuestionedFeatureID) to the clusters they belong to.
-- cluster_members.feature_id is UNIQUE, so each input feature id contributes
-- at most one row.
SELECT DISTINCT cluster_id
FROM cluster_members
WHERE feature_id = ANY(@feature_ids::text[])
ORDER BY cluster_id;

-- name: ListClusterMembershipsForFeatureIDs :many
-- Same lookup as ListClusterIDsForFeatureIDs but keeping feature_id on each
-- row, so a caller can map each of its own feature ids back to the cluster
-- it landed in (see cases.ListTraceClusters).
SELECT feature_id, cluster_id
FROM cluster_members
WHERE feature_id = ANY(@feature_ids::text[]);

-- name: ListBiometricDecisionsForFeatures :many
-- Every biometric_decisions row touching any of the given feature ids, seen
-- from that feature's side (biometric_decision_sides): the raw chain a caller
-- groups by counterpart_id to derive that pair's status (see
-- cluster.DeriveEdgeStatus) and who/what decided it (see
-- cases.ListCaseClusters). A decision between two of the given features
-- appears once from each side.
SELECT feature_id, counterpart_id, role, decision, system_source, username, confidence
FROM biometric_decision_sides
WHERE feature_id = ANY(@feature_ids::text[])
ORDER BY feature_id, counterpart_id, decided_at;

-- name: ListClustersByIDs :many
SELECT id, modality, created_at
FROM clusters
WHERE id = ANY(@cluster_ids::bigint[])
ORDER BY id;

-- name: ListClusterMembersByClusterIDs :many
SELECT cluster_id, feature_id
FROM cluster_members
WHERE cluster_id = ANY(@cluster_ids::bigint[])
ORDER BY cluster_id, feature_id;

-- name: ListCasesByCaseTraceIDs :many
-- Resolves a set of case_trace ids (parsed back out of QUESTIONED cluster
-- member feature ids -- see graph.QuestionedFeatureID) to their biometric
-- cases. One case can own several of the given traces (rows are not
-- de-duplicated by case) so a caller can re-attribute each case back to the
-- cluster that supplied the matching trace.
SELECT bc.case_id, bc.modality, bc.description, ct.id AS case_trace_id
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE ct.id = ANY(@case_trace_ids::bigint[])
ORDER BY bc.case_id, ct.id;

-- name: ListKnownClusterMembers :many
-- Resolves a set of identity_file ids (KNOWN cluster member feature ids --
-- see graph.KnownFeatureID) to the enrollment they belong to, for rendering
-- a cluster's membership (see person.buildClusters). Same join shape as
-- FindNearestPersonsByFaceEmbedding, minus the embedding distance.
SELECT p.person_id, r.name, r.register_number, d.document_type, d.document_number,
       f.id AS identity_file_id, f.content_type
FROM identity_file f
JOIN identity_register r ON r.id = f.register_id
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE f.id = ANY(@identity_file_ids::bigint[]);

-- name: ListQuestionedClusterMembers :many
-- Resolves a set of case_trace ids (parsed back out of QUESTIONED cluster
-- member feature ids -- see graph.QuestionedFeatureID) to the case evidence
-- they belong to, for rendering a cluster's membership (see
-- person.buildClusters). Same join and thumbnail-source resolution as
-- FindNearestCasesByFaceEmbedding, minus the embedding distance.
SELECT bc.case_id, bc.modality, bc.description, ct.id AS case_trace_id,
       tcf.id AS trace_crop_file_id, ecf.id AS evidence_file_id,
       ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
LEFT JOIN case_files tcf ON tcf.id = ct.case_file_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
WHERE ct.id = ANY(@case_trace_ids::bigint[]);

-- name: SearchPersonsByName :many
-- Finds every identity_register whose name contains the search term
-- (case-insensitive substring match -- see
-- migrations/021_add_person_name_search.sql for the trigram index this
-- relies on), joined up to its person and document. A person with several
-- documents/registers can surface more than once, once per matching
-- register -- useful signal on its own (an alias, or the same name spelled
-- differently across enrollments), so callers are not deduplicated here.
SELECT p.person_id, r.name, r.register_number, d.document_type, d.document_number
FROM identity_register r
JOIN identity_document d ON d.id = r.document_id
JOIN person p ON p.id = d.person_id
WHERE r.name ILIKE $1
ORDER BY r.name
LIMIT $2;

-- name: CountPersonCasesByModality :many
-- For each of the given persons, the number of distinct biometric cases
-- linked through their resolved biometric clusters, grouped by modality --
-- the "N facial / N fingerprint" badge a person search result shows. Same
-- KNOWN feature -> cluster -> QUESTIONED trace -> case resolution as
-- person.ListCases/buildCases (feature_id text format from
-- graph.KnownFeatureID/QuestionedFeatureID), batched across every requested
-- person in one query instead of one loadPersonAndClusterIDs call per row.
SELECT p.person_id, bc.modality, COUNT(DISTINCT bc.id) AS case_count
FROM person p
JOIN identity_document d ON d.person_id = p.id
JOIN identity_register r ON r.document_id = d.id
JOIN identity_file f ON f.register_id = r.id
JOIN cluster_members known_cm ON known_cm.feature_id = f.id::text
JOIN cluster_members quest_cm ON quest_cm.cluster_id = known_cm.cluster_id
JOIN case_traces ct ON quest_cm.feature_id = 'TRACE:' || ct.id || '#feature'
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE p.person_id = ANY(@person_ids::text[])
GROUP BY p.person_id, bc.modality
ORDER BY p.person_id, bc.modality;

-- name: UpsertBiometricTemplate :one
-- Re-extraction replaces the template and clears matched_at, so the new template goes
-- through matching again (as UpsertFeatureEmbedding does for embeddings).
INSERT INTO biometric_templates (biometricfeature_id, template_type, template, model_version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (biometricfeature_id, template_type) DO UPDATE SET
    template = EXCLUDED.template,
    model_version = EXCLUDED.model_version,
    matched_at = NULL,
    updated_at = NOW()
RETURNING id;

-- name: ListBiometricTemplates :many
-- Every template of a type: the gallery biometricmatch.RunTemplates scores each unmatched
-- template against. matched_at IS NULL marks the ones still to be probed.
SELECT id, biometricfeature_id, template, (matched_at IS NULL)::boolean AS unmatched
FROM biometric_templates
WHERE template_type = $1
ORDER BY id;

-- name: MarkBiometricTemplateMatched :exec
UPDATE biometric_templates SET matched_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: ListKnownFingerprintFeaturesMissingTemplate :many
-- Every KNOWN FINGERPRINT_TEMPLATE biometricfeature (an enrolled ten-print) with no
-- template of the given type yet -- what cmd/backfill-fingerprint-templates enqueues
-- extraction for.
SELECT bf.id
FROM biometricfeature bf
WHERE bf.feature_type = 'FINGERPRINT_TEMPLATE'
  AND bf.identity_file_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM biometric_templates bt
      WHERE bt.biometricfeature_id = bf.id AND bt.template_type = sqlc.arg(template_type)
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
