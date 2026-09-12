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
    (case_id, case_type, description)
VALUES ($1, $2, $3)
ON CONFLICT (case_id) DO UPDATE SET
    case_type = EXCLUDED.case_type,
    description = EXCLUDED.description,
    updated_at = NOW();

-- name: CountCriminalCases :one
SELECT COUNT(*) FROM criminal_cases;

-- name: ListCriminalCases :many
SELECT case_id, case_type, description
FROM criminal_cases
ORDER BY case_id
LIMIT $1 OFFSET $2;

-- name: UpsertComparison :exec
INSERT INTO comparisons
    (evidence_a, evidence_b, case_type, comparison_type, responsible_user)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (evidence_a, evidence_b) DO UPDATE SET
    case_type = EXCLUDED.case_type,
    comparison_type = EXCLUDED.comparison_type,
    responsible_user = EXCLUDED.responsible_user,
    updated_at = NOW();

-- name: ListComparisons :many
SELECT evidence_a, evidence_b, case_type, comparison_type, responsible_user
FROM comparisons
ORDER BY evidence_a, evidence_b
LIMIT $1 OFFSET $2;
