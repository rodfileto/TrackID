-- Articles (db/migrations/010_articles.sql, docs/person-article-plan.md).

-- name: EnsureArticle :exec
-- Creates the subject's article row if it has none; a save then locks it (LockArticle).
INSERT INTO articles (subject_kind, subject_ref) VALUES ($1, $2)
ON CONFLICT (subject_kind, subject_ref) DO NOTHING;

-- name: LockArticle :one
-- The article row, locked for the rest of the transaction: saves of one article run one at a time.
SELECT id, current_revision_id FROM articles
WHERE subject_kind = $1 AND subject_ref = $2
FOR UPDATE;

-- name: GetArticleBySubject :one
SELECT id, current_revision_id, created_at, updated_at FROM articles
WHERE subject_kind = $1 AND subject_ref = $2;

-- name: InsertArticleRevision :one
INSERT INTO article_revisions (article_id, revision_number, parent_revision_id, body_sha256, body_chars, edit_summary, author_id, template_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, created_at;

-- name: InsertArticleRevisionBody :exec
INSERT INTO article_revision_bodies (revision_id, body, body_text) VALUES ($1, $2, $3);

-- name: SetArticleCurrentRevision :exec
UPDATE articles SET current_revision_id = $2, updated_at = NOW() WHERE id = $1;

-- name: GetArticleRevision :one
-- One revision of an article by its number, with its author and its text. The text is LEFT
-- joined: a redacted revision (the plan's decision 8) keeps its record but has no body.
SELECT r.id, r.revision_number, r.parent_revision_id, r.body_sha256, r.body_chars, r.edit_summary, r.created_at,
    u.id AS author_id, u.username AS author_username, u.first_name AS author_first_name, u.last_name AS author_last_name,
    b.body, b.body_text, r.template_id
FROM article_revisions r
JOIN users u ON u.id = r.author_id
LEFT JOIN article_revision_bodies b ON b.revision_id = r.id
WHERE r.article_id = $1 AND r.revision_number = $2;

-- name: GetArticleRevisionNumber :one
SELECT revision_number FROM article_revisions WHERE id = $1;

-- name: ListArticleRevisions :many
-- An article's history, newest first, without the text.
SELECT r.id, r.revision_number, r.parent_revision_id, r.body_chars, r.edit_summary, r.created_at,
    u.id AS author_id, u.username AS author_username, u.first_name AS author_first_name, u.last_name AS author_last_name,
    (b.revision_id IS NOT NULL)::boolean AS has_body, r.template_id
FROM article_revisions r
JOIN users u ON u.id = r.author_id
LEFT JOIN article_revision_bodies b ON b.revision_id = r.id
WHERE r.article_id = $1
ORDER BY r.revision_number DESC;

-- name: InsertArticleTemplate :one
-- Logs one draft generated from the records (db/migrations/010_articles.sql).
INSERT INTO article_templates (subject_kind, subject_ref, generator, language, body, body_sha256, generated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, generated_at;

-- name: GetArticleTemplate :one
SELECT id, subject_kind, subject_ref, generator, generated_by, generated_at FROM article_templates WHERE id = $1;

-- name: ListPersonsLinkedToCases :many
-- The enrolled persons one cluster layer (@basis) links to any of the given cases (a KNOWN
-- feature and a QUESTIONED trace of the case in one cluster), other than @person_id: the case
-- side of person.ListCases, for a draft's associates. REVIEWED gives the links people
-- confirmed; AUTOMATIC every link the matcher made too.
SELECT DISTINCT p.person_id, bc.case_id
FROM biometric_cases bc
JOIN case_evidences ce ON ce.biometric_case_id = bc.id
JOIN case_traces ct ON ct.evidence_id = ce.id
JOIN cluster_members quest_cm ON quest_cm.feature_id = 'TRACE:' || ct.id || '#feature' AND quest_cm.basis = @basis::text
JOIN cluster_members known_cm ON known_cm.cluster_id = quest_cm.cluster_id AND known_cm.basis = @basis::text
JOIN identity_files f ON known_cm.feature_id = f.id::text
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE bc.case_id = ANY(@case_ids::text[]) AND p.person_id <> @person_id::text
ORDER BY p.person_id, bc.case_id;

-- name: DeleteArticleMentions :exec
DELETE FROM article_mentions WHERE article_id = $1;

-- name: InsertArticleMentions :exec
-- The current revision's mentions, as three arrays of equal length.
INSERT INTO article_mentions (article_id, kind, ref, mention_count)
SELECT @article_id::bigint, unnest(@kinds::text[]), unnest(@refs::text[]), unnest(@counts::int[]);

-- name: ListArticleMentionKeys :many
SELECT kind, ref FROM article_mentions WHERE article_id = $1;

-- name: ListBacklinks :many
-- The articles whose current revision mentions any of the given targets (kinds[i], refs[i]).
SELECT a.subject_kind, a.subject_ref, m.kind, m.ref, m.mention_count, a.updated_at
FROM article_mentions m
JOIN articles a ON a.id = m.article_id
WHERE (m.kind, m.ref) IN (SELECT unnest(@kinds::text[]), unnest(@refs::text[]))
ORDER BY a.updated_at DESC, a.subject_ref, m.kind, m.ref;

-- Mention targets: resolving the keys an article holds (List*) and the @ menu's search
-- (SearchMention*). Each pair returns the same columns.

-- name: ListMentionCases :many
SELECT case_id, case_type, modality, description, created_at
FROM biometric_cases WHERE case_id = ANY(@case_ids::text[]);

-- name: SearchMentionCases :many
SELECT case_id, case_type, modality, description, created_at
FROM biometric_cases
WHERE case_id ILIKE '%' || sqlc.arg(q)::text || '%' OR description ILIKE '%' || sqlc.arg(q)::text || '%'
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListMentionOccurrences :many
SELECT id, occurrence_year, occurrence_seq, agency, occurrence_number, inquiry_number, occurred_from, status, description
FROM occurrences WHERE id = ANY(@ids::bigint[]);

-- name: SearchMentionOccurrences :many
SELECT id, occurrence_year, occurrence_seq, agency, occurrence_number, inquiry_number, occurred_from, status, description
FROM occurrences
WHERE (occurrence_year::text || '.' || lpad(occurrence_seq::text, 4, '0')) ILIKE '%' || sqlc.arg(q)::text || '%'
   OR occurrence_number ILIKE '%' || sqlc.arg(q)::text || '%'
   OR inquiry_number ILIKE '%' || sqlc.arg(q)::text || '%'
   OR description ILIKE '%' || sqlc.arg(q)::text || '%'
ORDER BY occurred_from DESC NULLS LAST, id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListMentionInquiries :many
-- An inquiry is the occurrences that share its number.
SELECT inquiry_number::text AS inquiry_number, COUNT(*)::int AS occurrence_count,
    (COUNT(occurred_from) > 0)::bool AS has_dates,
    COALESCE(MIN(occurred_from), 'epoch')::timestamptz AS first_occurred,
    COALESCE(MAX(occurred_from), 'epoch')::timestamptz AS last_occurred
FROM occurrences WHERE inquiry_number = ANY(@numbers::text[])
GROUP BY inquiry_number;

-- name: SearchMentionInquiries :many
SELECT inquiry_number::text AS inquiry_number, COUNT(*)::int AS occurrence_count,
    (COUNT(occurred_from) > 0)::bool AS has_dates,
    COALESCE(MIN(occurred_from), 'epoch')::timestamptz AS first_occurred,
    COALESCE(MAX(occurred_from), 'epoch')::timestamptz AS last_occurred
FROM occurrences
WHERE inquiry_number IS NOT NULL AND inquiry_number ILIKE '%' || sqlc.arg(q)::text || '%'
GROUP BY inquiry_number
ORDER BY MAX(occurred_from) DESC NULLS LAST
LIMIT sqlc.arg(max_rows);

-- name: ListMentionPersons :many
-- Each person with the name on their first register (the person page's display name), empty when
-- nothing is enrolled yet.
SELECT p.id AS person_number, p.person_id,
    COALESCE((SELECT r.name FROM identity_documents d JOIN identity_registers r ON r.document_id = d.id
              WHERE d.person_id = p.id ORDER BY d.id, r.id LIMIT 1), '')::text AS name
FROM persons p WHERE p.person_id = ANY(@person_ids::text[]);

-- name: SearchMentionPersons :many
-- Persons by a name on any register (the trigram index) or by their exact person id. Callers
-- send a non-empty term: an empty one would scan every register.
WITH hits AS (
    SELECT d.person_id AS pid FROM identity_registers r JOIN identity_documents d ON d.id = r.document_id
    WHERE r.name ILIKE '%' || sqlc.arg(q)::text || '%'
    UNION
    SELECT p.id FROM persons p WHERE p.person_id = sqlc.arg(q)::text
)
SELECT p.id AS person_number, p.person_id,
    COALESCE((SELECT r.name FROM identity_documents d JOIN identity_registers r ON r.document_id = d.id
              WHERE d.person_id = p.id ORDER BY d.id, r.id LIMIT 1), '')::text AS name
FROM persons p JOIN hits ON hits.pid = p.id
ORDER BY name, p.person_id
LIMIT sqlc.arg(max_rows);

-- name: ListMentionClusters :many
SELECT c.id, c.modality, c.basis, c.created_at,
    (SELECT COUNT(*) FROM cluster_members m WHERE m.cluster_id = c.id)::int AS member_count
FROM clusters c WHERE c.id = ANY(@ids::bigint[]);

-- name: SearchMentionClusters :many
-- Clusters whose id starts with the digits typed.
SELECT c.id, c.modality, c.basis, c.created_at,
    (SELECT COUNT(*) FROM cluster_members m WHERE m.cluster_id = c.id)::int AS member_count
FROM clusters c
WHERE c.id::text LIKE sqlc.arg(q)::text || '%'
ORDER BY c.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListClusterMergesFrom :many
-- Where each of the given (removed) clusters was merged into, latest merge first.
SELECT from_cluster_id, to_cluster_id FROM cluster_merges
WHERE from_cluster_id = ANY(@ids::bigint[])
ORDER BY merged_at DESC, id DESC;

-- name: ListMentionRequests :many
-- A request with the cases it opened or joined (case_request_cases), in case order.
SELECT cr.id, cr.authority, cr.document_reference, cr.received_on,
    COALESCE((SELECT array_agg(bc.case_id ORDER BY bc.case_id) FROM case_request_cases rc
              JOIN biometric_cases bc ON bc.id = rc.biometric_case_id WHERE rc.case_request_id = cr.id), '{}')::text[] AS case_ids
FROM case_requests cr
WHERE cr.id = ANY(@ids::bigint[]);

-- name: SearchMentionRequests :many
SELECT cr.id, cr.authority, cr.document_reference, cr.received_on,
    COALESCE((SELECT array_agg(bc.case_id ORDER BY bc.case_id) FROM case_request_cases rc
              JOIN biometric_cases bc ON bc.id = rc.biometric_case_id WHERE rc.case_request_id = cr.id), '{}')::text[] AS case_ids
FROM case_requests cr
WHERE cr.document_reference ILIKE '%' || sqlc.arg(q)::text || '%'
   OR cr.authority ILIKE '%' || sqlc.arg(q)::text || '%'
   OR EXISTS (SELECT 1 FROM case_request_cases rc JOIN biometric_cases bc ON bc.id = rc.biometric_case_id
              WHERE rc.case_request_id = cr.id AND bc.case_id ILIKE '%' || sqlc.arg(q)::text || '%')
ORDER BY cr.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListMentionReports :many
-- Issued laudos only: a draft has no number and may still change.
SELECT ar.id, ar.report_type, ar.report_year, ar.report_number, ar.issued_at, bc.case_id
FROM analysis_reports ar
JOIN case_analyses ca ON ca.id = ar.case_analysis_id
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
WHERE ar.id = ANY(@ids::bigint[]) AND ar.status = 'ISSUED';

-- name: SearchMentionReports :many
SELECT ar.id, ar.report_type, ar.report_year, ar.report_number, ar.issued_at, bc.case_id
FROM analysis_reports ar
JOIN case_analyses ca ON ca.id = ar.case_analysis_id
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
WHERE ar.status = 'ISSUED'
  AND ((ar.report_number::text || '/' || ar.report_year::text) ILIKE '%' || sqlc.arg(q)::text || '%'
       OR bc.case_id ILIKE '%' || sqlc.arg(q)::text || '%')
ORDER BY ar.issued_at DESC, ar.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListCaseMentionTargets :many
-- What an article can mention of one case (articles.CaseTargets): the case, its occurrence and
-- that occurrence's inquiry, its requests and its issued laudos.
SELECT 'CASE'::text AS kind, bc.case_id::text AS ref FROM biometric_cases bc WHERE bc.case_id = @case_id::text
UNION ALL
SELECT 'OCCURRENCE', o.id::text FROM biometric_cases bc JOIN occurrences o ON o.id = bc.occurrence_id
WHERE bc.case_id = @case_id::text
UNION ALL
SELECT 'INQUIRY', o.inquiry_number FROM biometric_cases bc JOIN occurrences o ON o.id = bc.occurrence_id
WHERE bc.case_id = @case_id::text AND o.inquiry_number IS NOT NULL
UNION ALL
SELECT 'REQUEST', rc.case_request_id::text FROM biometric_cases bc JOIN case_request_cases rc ON rc.biometric_case_id = bc.id
WHERE bc.case_id = @case_id::text
UNION ALL
SELECT 'REPORT', ar.id::text FROM biometric_cases bc JOIN case_analyses ca ON ca.biometric_case_id = bc.id
JOIN analysis_reports ar ON ar.case_analysis_id = ca.id
WHERE bc.case_id = @case_id::text AND ar.status = 'ISSUED';
