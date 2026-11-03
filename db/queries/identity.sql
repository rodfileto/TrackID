-- name: UpsertPerson :one
INSERT INTO persons (person_id, meta)
VALUES ($1, $2)
ON CONFLICT (person_id) DO UPDATE SET
    meta = EXCLUDED.meta,
    updated_at = NOW()
RETURNING id;

-- name: UpsertIdentityDocument :one
INSERT INTO identity_documents (person_id, document_number, document_type, fiscal_number)
VALUES ($1, $2, $3, $4)
ON CONFLICT (document_type, document_number) DO UPDATE SET
    person_id = EXCLUDED.person_id,
    fiscal_number = EXCLUDED.fiscal_number,
    updated_at = NOW()
RETURNING id;

-- name: UpsertIdentityRegister :one
INSERT INTO identity_registers
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
INSERT INTO identity_files (register_id, file_type, sequence, source_path, storage_ref, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (register_id, file_type, sequence) DO UPDATE SET
    source_path = EXCLUDED.source_path,
    storage_ref = EXCLUDED.storage_ref,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    updated_at = NOW()
RETURNING id;

-- Person profile (see the person package): read-only views assembled from
-- the enrollment chain, cluster_members, and the evidence hierarchy. Nothing
-- here writes; a person's cluster/case links are always derived fresh from
-- biometric_decisions-backed cluster_members, never cached.

-- name: GetPersonByPersonID :one
SELECT id, person_id, meta, created_at, updated_at
FROM persons
WHERE person_id = $1;

-- name: GetIdentityFileForPerson :one
-- Scopes an identity_files row to the given person (through
-- identity_registers -> identity_documents), same scoping GetCaseFile does
-- for a case's evidence files -- so a caller can't download a file that
-- doesn't belong to the person named in the URL.
SELECT f.id, f.file_type, f.source_path, f.storage_ref, f.content_type
FROM identity_files f
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE f.id = sqlc.arg(identity_file_id) AND p.person_id = sqlc.arg(person_id);

-- name: ListIdentityChainByPerson :many
-- Every document -> register recorded for one person, left-joined down to
-- each register's files and the KNOWN biometric feature a file yielded. Both
-- joins are LEFT so a register shows up even before any file has been
-- uploaded for it, and a file shows up even when its type (e.g. "pdf")
-- yields no feature -- a caller distinguishes the two by identity_file_id/
-- biometric_feature_id being NULL. Mirrors ListKnownIdentityChain's join
-- shape, scoped to one person instead of every person.
SELECT
    d.id AS document_id, d.document_number, d.document_type, d.fiscal_number,
    r.id AS register_id, r.register_number, r.name,
    r.parent_1_name, r.parent_1_gender, r.parent_2_name, r.parent_2_gender,
    r.birth_date, r.meta AS register_meta,
    f.id AS identity_file_id, f.file_type, f.sequence, f.source_path, f.storage_ref, f.content_type, f.size_bytes,
    bf.id AS biometric_feature_id, bf.feature_type
FROM identity_documents d
JOIN identity_registers r ON r.document_id = d.id
LEFT JOIN identity_files f ON f.register_id = r.id
LEFT JOIN biometric_features bf ON bf.identity_file_id = f.id
WHERE d.person_id = $1
ORDER BY d.id, r.id, f.sequence;

-- name: SearchPersonsByName :many
-- Finds every identity_registers row whose name contains the search term
-- (case-insensitive substring match -- see
-- db/migrations/002_identity.sql for the trigram index this
-- relies on), joined up to its person and document. A person with several
-- documents/registers can surface more than once, once per matching
-- register -- useful signal on its own (an alias, or the same name spelled
-- differently across enrollments), so callers are not deduplicated here.
SELECT p.person_id, r.name, r.register_number, d.document_type, d.document_number
FROM identity_registers r
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
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
-- Reads one layer (@basis): the person views read REVIEWED.
SELECT p.person_id, bc.modality, COUNT(DISTINCT bc.id) AS case_count
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_files f ON f.register_id = r.id
JOIN cluster_members known_cm ON known_cm.feature_id = f.id::text AND known_cm.basis = @basis
JOIN cluster_members quest_cm ON quest_cm.cluster_id = known_cm.cluster_id
JOIN case_traces ct ON quest_cm.feature_id = 'TRACE:' || ct.id || '#feature'
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE p.person_id = ANY(@person_ids::text[])
GROUP BY p.person_id, bc.modality
ORDER BY p.person_id, bc.modality;

-- name: ListEnrolledReferences :many
-- Enrolled records used as reference material in a report: who, by which document and register,
-- enrolled when.
SELECT f.id AS identity_file_id, p.person_id, r.name, r.register_number, d.document_type, d.document_number,
       r.created_at AS enrolled_at, f.content_type
FROM identity_files f
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE f.id = ANY(@identity_file_ids::bigint[]);
