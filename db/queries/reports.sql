-- name: InsertAnalysisReport :exec
INSERT INTO analysis_reports (case_analysis_id, report_type) VALUES ($1, $2);

-- name: CaseUnitOfAnalysis :one
SELECT bc.unit_id FROM case_analyses ca JOIN biometric_cases bc ON bc.id = ca.biometric_case_id WHERE ca.id = $1;

-- name: MaxReportNumber :one
-- The last number of a unit's sequence for the report type's discipline in a year.
SELECT COALESCE(MAX(report_number), 0)::integer
FROM analysis_reports
WHERE issuing_unit_id = @issuing_unit_id::bigint AND discipline = report_discipline(@report_type::text)
    AND report_year = @report_year::int;

-- name: IssueAnalysisReport :one
UPDATE analysis_reports
SET status = 'ISSUED', report_year = $2, report_number = $3, issued_by = $4, issued_at = $5, issuing_unit_id = $6
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

-- name: RecordImportedReport :execrows
-- An analysis's report as issued elsewhere; again, it refreshes one recorded before.
UPDATE analysis_reports SET status = 'ISSUED', imported = TRUE, report_year = sqlc.narg('report_year')::int,
    report_number = sqlc.narg('report_number')::int, issued_at = @issued_at, case_file_id = sqlc.narg('case_file_id')::bigint,
    snapshot = @snapshot, issuing_unit_id = @issuing_unit_id::bigint
WHERE case_analysis_id = @case_analysis_id AND (status = 'DRAFT' OR imported);

-- name: ReportNumberTaken :one
SELECT EXISTS (SELECT 1 FROM analysis_reports WHERE issuing_unit_id = @issuing_unit_id::bigint
    AND discipline = report_discipline(@report_type::text) AND report_year = @report_year::int
    AND report_number = @report_number::int AND case_analysis_id <> @case_analysis_id)::bool AS taken;

-- name: GetOccurrenceForReport :one
-- The occurrence a scene laudo describes: when, where and its inquiry.
SELECT o.occurred_from, o.occurred_to, o.inquiry_number, o.description, o.agency, o.occurrence_number,
       l.name AS place_name, l.address, l.neighbourhood, l.place_type, l.precision, l.uf AS location_uf,
       COALESCE(m.name, '')::text AS municipality,
       COALESCE(ST_X(l.geom::geometry), 0)::float8 AS lon, COALESCE(ST_Y(l.geom::geometry), 0)::float8 AS lat,
       (l.id IS NOT NULL)::bool AS has_place
FROM occurrences o
LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
LEFT JOIN municipalities m ON m.code = l.municipality_code
WHERE o.id = $1;
