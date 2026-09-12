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
INSERT INTO criminal_cases (case_id, case_type, description)
VALUES ($1, $2, $3)
ON CONFLICT (case_id) DO UPDATE SET
    case_type = EXCLUDED.case_type,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id;

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
INSERT INTO identity_document (person_id, document_number, document_type, cpf)
VALUES ($1, $2, $3, $4)
ON CONFLICT (document_type, document_number) DO UPDATE SET
    person_id = EXCLUDED.person_id,
    cpf = EXCLUDED.cpf,
    updated_at = NOW()
RETURNING id;

-- name: UpsertIdentityRegister :one
INSERT INTO identity_register
    (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender, data_nascimento, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (register_number) DO UPDATE SET
    document_id = EXCLUDED.document_id,
    name = EXCLUDED.name,
    parent_1_name = EXCLUDED.parent_1_name,
    parent_1_gender = EXCLUDED.parent_1_gender,
    parent_2_name = EXCLUDED.parent_2_name,
    parent_2_gender = EXCLUDED.parent_2_gender,
    data_nascimento = EXCLUDED.data_nascimento,
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
SELECT id, criminal_case_id, category, media_type, hash_id, filename, source_path, storage_ref, content_type, size_bytes
FROM case_files
WHERE criminal_case_id = $1
ORDER BY id;

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
RETURNING id;

-- name: UpsertCaseEvidence :one
INSERT INTO case_evidences (criminal_case_id, sequence, case_file_id, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (criminal_case_id, sequence) DO UPDATE SET
    case_file_id = EXCLUDED.case_file_id,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id;

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
