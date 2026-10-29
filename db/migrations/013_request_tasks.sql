-- +goose Up
-- Requests route to tasks (docs/request-routing-plan.md): an analysis is the task a request asks
-- of a case. The fingerprint comparison (confronto papiloscópico) joins the facial review and
-- comparison; the laboratory and scene examinations are reserved, not offered yet.
ALTER TABLE case_analyses DROP CONSTRAINT case_analyses_analysis_type_check,
    ADD CONSTRAINT case_analyses_analysis_type_check CHECK (analysis_type IN (
        'FACIAL_REVIEW', 'FACIAL_COMPARISON', 'FINGERPRINT_COMPARISON',
        'FINGERPRINT_LAB_EXAMINATION', 'CRIME_SCENE_EXAMINATION'));
ALTER TABLE analysis_reports DROP CONSTRAINT analysis_reports_report_type_check,
    ADD CONSTRAINT analysis_reports_report_type_check CHECK (report_type IN (
        'FACIAL_REVIEW_REPORT', 'FACIAL_COMPARISON_REPORT', 'FINGERPRINT_COMPARISON_REPORT',
        'FINGERPRINT_LAB_EXAMINATION_REPORT', 'CRIME_SCENE_EXAMINATION_REPORT'));

-- +goose Down
-- Analyses of the new types, and their reports, cannot be kept.
DELETE FROM case_analyses WHERE analysis_type NOT IN ('FACIAL_REVIEW', 'FACIAL_COMPARISON');
ALTER TABLE analysis_reports DROP CONSTRAINT analysis_reports_report_type_check,
    ADD CONSTRAINT analysis_reports_report_type_check CHECK (report_type IN ('FACIAL_REVIEW_REPORT', 'FACIAL_COMPARISON_REPORT'));
ALTER TABLE case_analyses DROP CONSTRAINT case_analyses_analysis_type_check,
    ADD CONSTRAINT case_analyses_analysis_type_check CHECK (analysis_type IN ('FACIAL_REVIEW', 'FACIAL_COMPARISON'));
