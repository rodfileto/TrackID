-- +goose Up
-- Facial reports (docs/facial-reports-plan.md): the requests a case answers, the role and
-- suitability of its traces, the laudo's conclusion scale, and the report documents.

-- A request is an authority asking the institute to examine material: a police station's ofício,
-- a prosecutor's or a court's request. It is not the occurrence (the event): one occurrence can
-- lead to several requests, and a civil case has a request but no occurrence. cited_reference is
-- the occurrence or inquiry number as the request cites it; occurrence_id the occurrence it was
-- resolved to, if any. Requests are numbered per case (sequence).
CREATE TABLE case_requests (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    authority TEXT NOT NULL,
    document_reference TEXT NOT NULL,
    document_date DATE,
    received_on DATE NOT NULL,
    due_on DATE,
    cited_reference TEXT,
    occurrence_id BIGINT REFERENCES occurrences(id) ON DELETE SET NULL,
    notes TEXT,
    registered_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_requests_case_sequence_key UNIQUE (biometric_case_id, sequence),
    CONSTRAINT case_requests_due_check CHECK (due_on IS NULL OR due_on >= received_on)
);
CREATE INDEX case_requests_due_idx ON case_requests (due_on);

-- The questions (quesitos) a request asks, in the order it asks them. A laudo answers each.
CREATE TABLE case_request_questions (
    id BIGSERIAL PRIMARY KEY,
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    question TEXT NOT NULL,
    CONSTRAINT case_request_questions_sequence_key UNIQUE (case_request_id, sequence)
);

-- The material a request sent in: its evidence files (case_files.hash_id is their SHA-256).
ALTER TABLE case_files ADD COLUMN case_request_id BIGINT REFERENCES case_requests(id) ON DELETE SET NULL;
CREATE INDEX case_files_case_request_id_idx ON case_files (case_request_id);

-- The request an analysis answers (NULL for analyses opened before requests existed).
ALTER TABLE case_analyses ADD COLUMN case_request_id BIGINT REFERENCES case_requests(id) ON DELETE SET NULL;

-- trace_role: QUESTIONED is material to identify; REFERENCE is known material sent in for
-- comparison (e.g. a suspect's photo). A reference is never an unidentified face of the case.
--
-- suitability is the examiner's value determination of a QUESTIONED trace (FISWG analysis phase):
--   SUITABLE   -- the image supports a forensic comparison
--   LEAD_ONLY  -- good enough for an investigative lead, not for a comparison
--   UNSUITABLE -- of no value
-- NULL until assessed; set together with who assessed it and when.
ALTER TABLE case_traces
    ADD COLUMN trace_role TEXT NOT NULL DEFAULT 'QUESTIONED' CHECK (trace_role IN ('QUESTIONED', 'REFERENCE')),
    ADD COLUMN suitability TEXT CHECK (suitability IN ('SUITABLE', 'LEAD_ONLY', 'UNSUITABLE')),
    ADD COLUMN suitability_notes TEXT,
    ADD COLUMN suitability_by BIGINT REFERENCES users(id),
    ADD COLUMN suitability_at TIMESTAMPTZ,
    ADD CONSTRAINT case_traces_suitability_assessed_check CHECK (
        (suitability IS NULL AND suitability_by IS NULL AND suitability_at IS NULL)
        OR (suitability IS NOT NULL AND suitability_by IS NOT NULL AND suitability_at IS NOT NULL)),
    ADD CONSTRAINT case_traces_reference_suitability_check CHECK (trace_role = 'QUESTIONED' OR suitability IS NULL);

-- A laudo's conclusion on a pair, on the graded scale; NULL outside a FACIAL_COMPARISON. Every
-- level maps onto decision (analyses.ConclusionDecision), so the rounds logic reads decision alone.
-- +goose StatementBegin
CREATE FUNCTION conclusion_decision(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE
        WHEN conclusion IN ('IDENTIFICATION', 'STRONG_SUPPORT', 'MODERATE_SUPPORT', 'LIMITED_SUPPORT') THEN 'POSITIVE'
        WHEN conclusion = 'INCONCLUSIVE' THEN 'INCONCLUSIVE'
        WHEN conclusion IN ('LIMITED_EXCLUSION', 'EXCLUSION') THEN 'NEGATIVE'
    END
$$;
-- +goose StatementEnd

ALTER TABLE biometric_decisions
    ADD COLUMN conclusion TEXT,
    ADD CONSTRAINT biometric_decisions_conclusion_check CHECK (
        conclusion IS NULL OR (role != 'SYSTEM' AND conclusion_decision(conclusion) = decision));
ALTER TABLE analysis_decision_drafts
    ADD COLUMN conclusion TEXT,
    ADD CONSTRAINT analysis_decision_drafts_conclusion_check CHECK (
        conclusion IS NULL OR conclusion_decision(conclusion) = decision);

CREATE OR REPLACE VIEW biometric_decision_sides AS
SELECT id, feature_a_id AS feature_id, feature_b_id AS counterpart_id,
       modality, role, case_analysis_id, decision, system_source, username, confidence, threshold,
       notes, related_reference, related_reference_kind, decided_at, created_at, conclusion
FROM biometric_decisions
UNION ALL
SELECT id, feature_b_id AS feature_id, feature_a_id AS counterpart_id,
       modality, role, case_analysis_id, decision, system_source, username, confidence, threshold,
       notes, related_reference, related_reference_kind, decided_at, created_at, conclusion
FROM biometric_decisions;

-- The lowest laudo conclusion that counts as an identification, as data: one append-only version
-- per row, the latest current. With none, STRONG_SUPPORT.
CREATE TABLE identification_levels (
    id BIGSERIAL PRIMARY KEY,
    minimum_conclusion TEXT NOT NULL CHECK (conclusion_decision(minimum_conclusion) = 'POSITIVE'),
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Report documents. A draft is a .docx the server generates from the template and the data, which
-- the examiner then edits (in its unlocked regions only); every save is a new version. Issuing
-- freezes the data (snapshot), checks the last version's locked text against it, renders the PDF
-- on the server and keeps both files. case_file_id (the PDF) is joined by docx_file_id.
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image'));

ALTER TABLE analysis_reports
    ADD COLUMN docx_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    ADD COLUMN snapshot JSONB,
    ADD COLUMN complements_report_id BIGINT REFERENCES analysis_reports(id),
    ADD CONSTRAINT analysis_reports_snapshot_check CHECK (status = 'DRAFT' OR snapshot IS NOT NULL);

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
ALTER TABLE analysis_reports DROP CONSTRAINT analysis_reports_snapshot_check,
    DROP COLUMN complements_report_id, DROP COLUMN snapshot, DROP COLUMN docx_file_id;
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'document', 'forensic_report', 'face_crop', 'codification_image'));
DROP TABLE identification_levels;
DROP VIEW biometric_decision_sides;
CREATE VIEW biometric_decision_sides AS
SELECT id, feature_a_id AS feature_id, feature_b_id AS counterpart_id,
       modality, role, case_analysis_id, decision, system_source, username, confidence, threshold,
       notes, related_reference, related_reference_kind, decided_at, created_at
FROM biometric_decisions
UNION ALL
SELECT id, feature_b_id AS feature_id, feature_a_id AS counterpart_id,
       modality, role, case_analysis_id, decision, system_source, username, confidence, threshold,
       notes, related_reference, related_reference_kind, decided_at, created_at
FROM biometric_decisions;
ALTER TABLE analysis_decision_drafts DROP COLUMN conclusion;
ALTER TABLE biometric_decisions DROP COLUMN conclusion;
DROP FUNCTION conclusion_decision(TEXT);
ALTER TABLE case_traces DROP CONSTRAINT case_traces_reference_suitability_check, DROP CONSTRAINT case_traces_suitability_assessed_check,
    DROP COLUMN suitability_at, DROP COLUMN suitability_by, DROP COLUMN suitability_notes, DROP COLUMN suitability, DROP COLUMN trace_role;
ALTER TABLE case_analyses DROP COLUMN case_request_id;
ALTER TABLE case_files DROP COLUMN case_request_id;
DROP TABLE case_request_questions;
DROP TABLE case_requests;
