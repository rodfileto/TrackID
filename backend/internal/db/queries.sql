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

-- name: UpsertCriminalCase :exec
INSERT INTO criminal_cases
    (case_id, case_type, description, responsible_user, comparison_type, related_reference, related_reference_kind)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (case_id) DO UPDATE SET
    case_type = EXCLUDED.case_type,
    description = EXCLUDED.description,
    responsible_user = EXCLUDED.responsible_user,
    comparison_type = EXCLUDED.comparison_type,
    related_reference = EXCLUDED.related_reference,
    related_reference_kind = EXCLUDED.related_reference_kind,
    updated_at = NOW();

-- name: CountCriminalCases :one
SELECT COUNT(*) FROM criminal_cases;

-- name: ListCriminalCases :many
SELECT case_id, case_type, description, responsible_user, comparison_type, related_reference, related_reference_kind
FROM criminal_cases
ORDER BY case_id
LIMIT $1 OFFSET $2;

-- name: UpsertInfoBioEnrollment :exec
INSERT INTO infobio_enrollments
    (nif, numero_identificacao, rin, nome, data_nascimento, cpf, nome_pai, nome_mae, container_number, nist_path, storage_ref, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (nif) DO UPDATE SET
    numero_identificacao = EXCLUDED.numero_identificacao,
    rin = EXCLUDED.rin,
    nome = EXCLUDED.nome,
    data_nascimento = EXCLUDED.data_nascimento,
    cpf = EXCLUDED.cpf,
    nome_pai = EXCLUDED.nome_pai,
    nome_mae = EXCLUDED.nome_mae,
    container_number = EXCLUDED.container_number,
    nist_path = EXCLUDED.nist_path,
    storage_ref = EXCLUDED.storage_ref,
    source = EXCLUDED.source,
    confirmed_at = NOW(),
    updated_at = NOW();

-- name: GetInfoBioEnrollment :one
SELECT nif, numero_identificacao, rin, nome, data_nascimento, cpf, nome_pai, nome_mae, container_number, nist_path, storage_ref, source, confirmed_at
FROM infobio_enrollments
WHERE nif = $1;
