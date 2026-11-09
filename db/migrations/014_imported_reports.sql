-- +goose Up
-- A report issued elsewhere and brought in by an import: its document is the original PDF, and it
-- may have neither a number nor anyone here who issued it, so those are only required of a report
-- issued in TrackID. Importing again refreshes it, which an issued report otherwise never allows.
ALTER TABLE analysis_reports ADD COLUMN imported BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE analysis_reports DROP CONSTRAINT analysis_reports_issued_check;
ALTER TABLE analysis_reports ADD CONSTRAINT analysis_reports_issued_check CHECK (
    (status = 'DRAFT' AND report_year IS NULL AND report_number IS NULL AND issued_by IS NULL AND issued_at IS NULL)
    OR
    (status = 'ISSUED' AND issued_at IS NOT NULL AND (imported OR (report_year IS NOT NULL AND report_number IS NOT NULL AND issued_by IS NOT NULL)))
);

-- +goose Down
-- Fails while an imported report has no number or no issuer.
ALTER TABLE analysis_reports DROP CONSTRAINT analysis_reports_issued_check;
ALTER TABLE analysis_reports ADD CONSTRAINT analysis_reports_issued_check CHECK (
    (status = 'DRAFT' AND report_year IS NULL AND report_number IS NULL AND issued_by IS NULL AND issued_at IS NULL)
    OR
    (status = 'ISSUED' AND report_year IS NOT NULL AND report_number IS NOT NULL AND issued_by IS NOT NULL AND issued_at IS NOT NULL)
);
ALTER TABLE analysis_reports DROP COLUMN imported;
