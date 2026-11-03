-- name: InsertAnalysisReport :exec
INSERT INTO analysis_reports (case_analysis_id, report_type) VALUES ($1, $2);

-- name: MaxReportNumber :one
SELECT COALESCE(MAX(report_number), 0)::integer
FROM analysis_reports
WHERE report_type = $1 AND report_year = $2;

-- name: IssueAnalysisReport :one
UPDATE analysis_reports
SET status = 'ISSUED', report_year = $2, report_number = $3, issued_by = $4, issued_at = $5
WHERE case_analysis_id = $1 AND status = 'DRAFT'
RETURNING report_type, report_year, report_number, issued_at;

-- name: LockReportNumbering :exec
-- Serializes report numbering (transaction-scoped): MaxReportNumber + 1 is then safe.
SELECT pg_advisory_xact_lock(7461002);

-- Report documents (phase 4).

-- name: GetAnalysisReport :one
SELECT ar.id, ar.case_analysis_id, ar.report_type, ar.status, ar.report_year, ar.report_number, ar.issued_at,
       ib.username AS issued_by, ar.case_file_id, ar.docx_file_id, ar.snapshot, ar.complements_report_id
FROM analysis_reports ar
LEFT JOIN users ib ON ib.id = ar.issued_by
WHERE ar.case_analysis_id = $1;

-- name: GetReportNumber :one
SELECT report_type, report_year, report_number FROM analysis_reports WHERE id = $1;

-- name: LatestReportVersion :one
SELECT v.id, v.version, v.case_file_id, v.generated, u.username AS created_by, v.created_at, f.storage_ref
FROM analysis_report_versions v
JOIN case_files f ON f.id = v.case_file_id
LEFT JOIN users u ON u.id = v.created_by
WHERE v.analysis_report_id = $1
ORDER BY v.version DESC
LIMIT 1;

-- name: InsertReportVersion :one
INSERT INTO analysis_report_versions (analysis_report_id, version, case_file_id, generated, created_by)
SELECT @analysis_report_id::bigint, COALESCE(MAX(version), 0) + 1, @case_file_id::bigint, @generated::bool,
       sqlc.narg('created_by')::bigint
FROM analysis_report_versions
WHERE analysis_report_id = @analysis_report_id::bigint
RETURNING id, version;

-- name: SetReportDocuments :exec
UPDATE analysis_reports SET case_file_id = $2, docx_file_id = $3, snapshot = $4 WHERE id = $1;

-- name: SetReportComplements :exec
UPDATE analysis_reports SET complements_report_id = $2 WHERE id = $1;

-- name: InsertReportDelivery :one
INSERT INTO report_deliveries (analysis_report_id, delivered_on, method, recipient, notes, recorded_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at;

-- name: ListReportDeliveries :many
SELECT d.id, d.delivered_on, d.method, d.recipient, d.notes, u.username AS recorded_by, d.created_at
FROM report_deliveries d
JOIN users u ON u.id = d.recorded_by
WHERE d.analysis_report_id = $1
ORDER BY d.delivered_on, d.id;

-- name: GetCaseForReport :one
SELECT bc.id, bc.case_id, bc.case_type, bc.modality, bc.description, bc.occurrence_id,
       o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number,
       COALESCE((SELECT string_agg(c.type_code, ', ' ORDER BY c.type_code) FROM occurrence_classifications c
                 WHERE c.occurrence_id = o.id), '')::text AS occurrence_types
FROM biometric_cases bc
LEFT JOIN occurrences o ON o.id = bc.occurrence_id
WHERE bc.id = $1;

-- name: ListCaseReports :many
-- The issued reports of a case, for a report's history section.
SELECT ar.id, ar.report_type, ar.report_year, ar.report_number, ar.issued_at, ca.id AS case_analysis_id, ca.analysis_type
FROM analysis_reports ar
JOIN case_analyses ca ON ca.id = ar.case_analysis_id
WHERE ca.biometric_case_id = $1 AND ar.status = 'ISSUED'
ORDER BY ar.issued_at, ar.id;
