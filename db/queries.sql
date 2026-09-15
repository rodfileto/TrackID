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

-- name: UpsertCriminalCase :one
-- (xmax = 0) tells a real insert apart from a row that already existed: RowsAffected() is
-- 1 either way, so callers that need to report insert-vs-update counts need this instead.
INSERT INTO criminal_cases (case_id, case_type, description)
VALUES ($1, $2, $3)
ON CONFLICT (case_id) DO UPDATE SET
    case_type = EXCLUDED.case_type,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id, (xmax = 0) AS inserted;

-- name: MaxCaseNumberForYear :one
-- Returns the highest case_number already used for a year, or 0 when none yet.
SELECT COALESCE(MAX(case_number), 0)::int4 AS max_number
FROM criminal_cases
WHERE case_year = sqlc.arg(case_year)::int4;

-- name: CreateCriminalCase :one
INSERT INTO criminal_cases (case_id, case_type, description, case_year, case_number)
VALUES ($1, $2, $3, $4, $5)
RETURNING case_id, case_type, description;

-- name: CountCriminalCases :one
SELECT COUNT(*) FROM criminal_cases;

-- name: ListCriminalCases :many
SELECT case_id, case_type, description
FROM criminal_cases
ORDER BY case_id
LIMIT $1 OFFSET $2;

-- name: GetCriminalCaseByCaseID :one
SELECT id, case_id, case_type, description
FROM criminal_cases
WHERE case_id = $1;

-- name: UpdateCriminalCaseDescription :exec
UPDATE criminal_cases SET description = $2, updated_at = NOW()
WHERE id = $1;

-- name: ListCriminalCaseIDsByType :many
SELECT case_id FROM criminal_cases WHERE case_type = $1 ORDER BY case_id;

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

-- name: ListCaseFilesByCriminalCase :many
SELECT id, criminal_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes, created_at
FROM case_files
WHERE criminal_case_id = $1
ORDER BY id;

-- name: GetCaseFile :one
SELECT id, category, filename, storage_ref, content_type
FROM case_files
WHERE id = $1 AND criminal_case_id = $2;

-- name: CountCaseTracesByCaseFile :one
-- Traces marked on an evidence file, via case_evidences.case_file_id -- used
-- to guard against deleting an evidence file that traces (and their
-- biometricfeature/embeddings) still depend on.
SELECT count(*) FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
WHERE ce.case_file_id = $1;

-- name: DeleteCaseFile :execrows
DELETE FROM case_files WHERE id = $1 AND criminal_case_id = $2 AND category = 'evidence';

-- name: UpsertCaseFile :one
INSERT INTO case_files (criminal_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (criminal_case_id, category, hash_id) DO UPDATE SET
    media_type = EXCLUDED.media_type,
    filename = EXCLUDED.filename,
    source_path = EXCLUDED.source_path,
    storage_ref = EXCLUDED.storage_ref,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    updated_at = NOW()
RETURNING id, created_at;

-- name: CreateCaseFile :one
INSERT INTO case_files (criminal_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, created_at;

-- name: UpsertCaseEvidence :one
INSERT INTO case_evidences (criminal_case_id, sequence, case_file_id, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (criminal_case_id, sequence) DO UPDATE SET
    case_file_id = EXCLUDED.case_file_id,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id;

-- name: GetCaseEvidenceByCaseFile :one
SELECT id FROM case_evidences
WHERE criminal_case_id = $1 AND case_file_id = $2;

-- name: CreateCaseEvidenceForFile :one
-- Case files added through AddEvidence only get a case_files row -- case_traces
-- hangs off case_evidences (MODEL.md section 2.2), so trace detection creates
-- the case_evidences row for a case_file on first use. sequence is the next
-- free slot for the case, since case_files added this way never carry one.
INSERT INTO case_evidences (criminal_case_id, sequence, case_file_id)
SELECT $1, COALESCE(MAX(sequence), 0) + 1, $2
FROM case_evidences
WHERE criminal_case_id = $1
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
WHERE ce.criminal_case_id = $1 AND ce.case_file_id = $2
ORDER BY ct.sequence;

-- name: DeleteCaseTrace :execrows
-- Scoped to the case + evidence file so a trace can only be deleted through
-- the case/evidence it actually belongs to. case_codifications and
-- biometricfeature (and, through it, feature_embeddings) cascade off
-- case_traces, so this is the only delete needed to fully remove a trace.
DELETE FROM case_traces ct
USING case_evidences ce
WHERE ct.id = $1
  AND ct.evidence_id = ce.id
  AND ce.criminal_case_id = $2
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
WHERE ct.id = $1 AND ce.criminal_case_id = $2 AND ce.case_file_id = $3;

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

-- name: MarkFeatureEmbeddingMatched :exec
UPDATE feature_embeddings SET matched_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: InsertCaseDecision :one
INSERT INTO case_decisions (criminal_case_id, decision, system_source, username, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, criminal_case_id, decision, system_source, username, notes, decided_at, created_at;

-- name: ListCaseDecisionsByCriminalCase :many
SELECT id, criminal_case_id, decision, system_source, username, notes, decided_at, created_at
FROM case_decisions
WHERE criminal_case_id = $1
ORDER BY decided_at;

-- name: ListAllCriminalCases :many
SELECT case_id, case_type, description
FROM criminal_cases
ORDER BY case_id;

-- name: ListQuestionedFeatures :many
SELECT bf.case_trace_id, bf.feature_type, cc.case_id
FROM biometricfeature bf
JOIN case_traces tr ON tr.id = bf.case_trace_id
JOIN case_evidences ev ON ev.id = tr.evidence_id
JOIN criminal_cases cc ON cc.id = ev.criminal_case_id
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
SELECT id, case_type
FROM clusters
ORDER BY id;

-- name: ListClusterMembers :many
SELECT cluster_id, feature_id
FROM cluster_members;

-- name: ListClusterMembersJoined :many
SELECT c.id AS cluster_id, c.case_type, m.feature_id
FROM clusters c
JOIN cluster_members m ON m.cluster_id = c.id
ORDER BY c.id, m.feature_id;

-- name: CreateCluster :one
INSERT INTO clusters (case_type) VALUES ($1) RETURNING id;

-- name: InsertClusterMember :exec
INSERT INTO cluster_members (cluster_id, feature_id) VALUES ($1, $2);

-- name: DeleteClusterMember :exec
DELETE FROM cluster_members WHERE feature_id = $1;

-- name: DeleteCluster :exec
DELETE FROM clusters WHERE id = $1;

-- name: InsertClusterMerge :exec
INSERT INTO cluster_merges (from_cluster_id, to_cluster_id, reason) VALUES ($1, $2, $3);
