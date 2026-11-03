-- Requests and received material (docs/facial-reports-plan.md, phase 1).

-- name: InsertCaseRequest :one
INSERT INTO case_requests (authority, document_reference, document_date, received_on, due_on,
    cited_reference, cited_inquiry_number, occurrence_id, notes, registered_by)
VALUES (@authority::text, @document_reference::text, sqlc.narg('document_date')::date, @received_on::date,
        sqlc.narg('due_on')::date, sqlc.narg('cited_reference')::text, sqlc.narg('cited_inquiry_number')::text,
        sqlc.narg('occurrence_id')::bigint,
        sqlc.narg('notes')::text, sqlc.narg('registered_by')::bigint)
RETURNING id;

-- name: LinkCaseRequest :one
-- A request joins a case as its next request, numbered after its last one (lock the case first:
-- LockBiometricCase).
INSERT INTO case_request_cases (case_request_id, biometric_case_id, sequence)
SELECT @case_request_id::bigint, @biometric_case_id::bigint, COALESCE(MAX(sequence), 0) + 1
FROM case_request_cases
WHERE biometric_case_id = @biometric_case_id::bigint
RETURNING sequence;

-- name: InsertCaseRequestQuestion :exec
INSERT INTO case_request_questions (case_request_id, sequence, question) VALUES ($1, $2, $3);

-- name: ListCaseRequests :many
-- A case's requests in order, with the occurrence each was linked to and who registered it.
SELECT r.id, rc.sequence, r.authority, r.document_reference, r.document_date, r.received_on, r.due_on, r.cited_reference,
       r.cited_inquiry_number, r.occurrence_id, o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number,
       o.inquiry_number,
       r.notes, u.username AS registered_by, r.created_at
FROM case_requests r
JOIN case_request_cases rc ON rc.case_request_id = r.id
JOIN biometric_cases bc ON bc.id = rc.biometric_case_id
LEFT JOIN occurrences o ON o.id = r.occurrence_id
LEFT JOIN users u ON u.id = r.registered_by
WHERE bc.case_id = $1
ORDER BY rc.sequence;

-- name: ListCaseRequestCases :many
-- Every case each of a case's requests opened or joined, this one included.
SELECT rc.case_request_id, obc.case_id, obc.modality
FROM case_request_cases rc
JOIN biometric_cases obc ON obc.id = rc.biometric_case_id
WHERE rc.case_request_id IN (
    SELECT mine.case_request_id FROM case_request_cases mine
    JOIN biometric_cases bc ON bc.id = mine.biometric_case_id
    WHERE bc.case_id = $1)
ORDER BY rc.case_request_id, obc.case_id;

-- name: ListRequests :many
-- Every request, overdue and soonest due first: what the coordinator has to answer. open counts
-- the analyses still open that answer it.
SELECT r.id, r.authority, r.document_reference, r.received_on, r.due_on, r.cited_reference, r.cited_inquiry_number,
       o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number,
       (SELECT COUNT(*) FROM case_analyses ca WHERE ca.case_request_id = r.id AND ca.status = 'OPEN')::bigint AS open_analyses,
       COALESCE((SELECT array_agg(bc.case_id ORDER BY bc.case_id)
                 FROM case_request_cases rc JOIN biometric_cases bc ON bc.id = rc.biometric_case_id
                 WHERE rc.case_request_id = r.id), '{}')::text[] AS case_ids,
       COALESCE((SELECT array_agg(ca.analysis_type ORDER BY ca.id)
                 FROM case_analyses ca WHERE ca.case_request_id = r.id), '{}')::text[] AS tasks
FROM case_requests r
LEFT JOIN occurrences o ON o.id = r.occurrence_id
ORDER BY r.due_on NULLS LAST, r.received_on DESC, r.id DESC
LIMIT @row_limit::int;

-- name: ListCaseRequestQuestions :many
SELECT q.case_request_id, q.sequence, q.question
FROM case_request_questions q
JOIN case_request_cases rc ON rc.case_request_id = q.case_request_id
JOIN biometric_cases bc ON bc.id = rc.biometric_case_id
WHERE bc.case_id = $1
ORDER BY rc.sequence, q.sequence;

-- name: AttachRequestFiles :execrows
-- Marks a case's evidence files as material the request sent in.
UPDATE case_files SET case_request_id = @case_request_id::bigint, updated_at = NOW()
WHERE biometric_case_id = @biometric_case_id::bigint AND category IN ('evidence', 'video') AND id = ANY(@file_ids::bigint[]);

-- name: ListRequestFiles :many
-- Every evidence file of a case with the request that sent it, if any.
SELECT f.id, f.case_request_id, f.filename, f.hash_id, f.content_type, f.size_bytes, f.created_at
FROM case_files f
JOIN biometric_cases bc ON bc.id = f.biometric_case_id
WHERE bc.case_id = $1 AND f.category IN ('evidence', 'video')
ORDER BY f.id;

-- name: GetCaseForRequest :one
SELECT id, case_id, case_type, modality, description FROM biometric_cases WHERE case_id = $1;

-- name: GetOccurrenceNumbers :one
SELECT agency, occurrence_number, inquiry_number, occurrence_year, occurrence_seq FROM occurrences WHERE id = $1;

-- name: SetBiometricCaseOccurrenceIfNone :execrows
UPDATE biometric_cases SET occurrence_id = $2, updated_at = NOW() WHERE id = $1 AND occurrence_id IS NULL;

-- name: FindOccurrenceByReference :many
-- Occurrences a request's cited number names exactly: the TrackID number ("2026.0001") or the
-- police agency's own number. More than one row is ambiguous.
SELECT o.id
FROM occurrences o
WHERE (o.occurrence_year::text || '.' || lpad(o.occurrence_seq::text, 4, '0')) = @reference::text
   OR o.occurrence_number = @reference::text
ORDER BY o.id
LIMIT 2;

-- name: FindOccurrenceByInquiryNumber :many
-- Occurrences a request's cited inquiry number names exactly. More than one row is ambiguous (an
-- inquiry can gather several occurrences).
SELECT o.id
FROM occurrences o
WHERE o.inquiry_number = @inquiry_number::text
ORDER BY o.id
LIMIT 2;

-- name: GetCaseRequestOfCase :one
SELECT case_request_id FROM case_request_cases WHERE case_request_id = $1 AND biometric_case_id = $2;

-- name: AttachUnrequestedAnalyses :execrows
-- Analyses of a case opened before it had any request (its automatic review) answer its first one.
UPDATE case_analyses SET case_request_id = $2 WHERE biometric_case_id = $1 AND case_request_id IS NULL;

-- name: LatestCaseRequest :one
SELECT case_request_id FROM case_request_cases WHERE biometric_case_id = $1 ORDER BY sequence DESC LIMIT 1;
