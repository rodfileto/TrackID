-- +goose Up
-- Reports (docs/facial-reports-plan.md): the laudo of each analysis, its draft versions and its
-- deliveries.
-- One report per analysis, its type fixed by the analysis type (analyses.ReportType). It is a
-- DRAFT until its examiner issues it, which numbers it "<year>.<sequence>" per report type and
-- closes the analysis.
--
-- A draft is a .docx the server generates from the template and the data, which the examiner then
-- edits (in its unlocked regions only); every save is a new version (analysis_report_versions,
-- docx_file_id the latest). Issuing freezes the data (snapshot), checks the last version's locked
-- text against it, renders the PDF on the server and keeps both files: case_file_id is the PDF (a
-- case_files 'forensic_report'). complements_report_id is the report a complementary one adds to.
CREATE TABLE analysis_reports (
    id BIGSERIAL PRIMARY KEY,
    case_analysis_id BIGINT NOT NULL UNIQUE REFERENCES case_analyses(id) ON DELETE CASCADE,
    report_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ISSUED')),
    report_year INTEGER,
    report_number INTEGER,
    issued_by BIGINT REFERENCES users(id),
    issued_at TIMESTAMPTZ,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    docx_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    snapshot JSONB,
    complements_report_id BIGINT REFERENCES analysis_reports(id),
    CONSTRAINT analysis_reports_report_type_check CHECK (report_type IN (
        'FACIAL_REVIEW_REPORT', 'FACIAL_COMPARISON_REPORT', 'FINGERPRINT_COMPARISON_REPORT',
        'FINGERPRINT_LAB_EXAMINATION_REPORT', 'CRIME_SCENE_EXAMINATION_REPORT')),
    CONSTRAINT analysis_reports_snapshot_check CHECK (status = 'DRAFT' OR snapshot IS NOT NULL),
    CONSTRAINT analysis_reports_issued_check CHECK (
        (status = 'DRAFT' AND report_year IS NULL AND report_number IS NULL AND issued_by IS NULL AND issued_at IS NULL)
        OR
        (status = 'ISSUED' AND report_year IS NOT NULL AND report_number IS NOT NULL AND issued_by IS NOT NULL AND issued_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX analysis_reports_number_key ON analysis_reports (report_type, report_year, report_number);


-- Every saved version of a report's draft. generated: the server built it from the data (the
-- first draft, or a refresh after the data changed); otherwise the examiner saved it.
CREATE TABLE analysis_report_versions (
    id BIGSERIAL PRIMARY KEY,
    analysis_report_id BIGINT NOT NULL REFERENCES analysis_reports(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    case_file_id BIGINT NOT NULL REFERENCES case_files(id),
    generated BOOLEAN NOT NULL,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT analysis_report_versions_key UNIQUE (analysis_report_id, version)
);

-- When, how and to whom an issued report was sent.
CREATE TABLE report_deliveries (
    id BIGSERIAL PRIMARY KEY,
    analysis_report_id BIGINT NOT NULL REFERENCES analysis_reports(id) ON DELETE CASCADE,
    delivered_on DATE NOT NULL,
    method TEXT NOT NULL,
    recipient TEXT NOT NULL,
    notes TEXT,
    recorded_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE report_deliveries;
DROP TABLE analysis_report_versions;
DROP TABLE analysis_reports;
