-- +goose Up
-- A biometric case is worked through one or more analyses: rounds of examination, each of a
-- type with its own methodology, examiner and report. For faces (FISWG):
--   FACIAL_REVIEW     -- one-to-many: the examiner reviews the facial recognition system's
--                        candidates for the case's faces; the result is an investigative lead or a
--                        negative result, not a forensic examination. Every FACIAL case gets one
--                        automatically (requested_by NULL).
--   FACIAL_COMPARISON -- one-to-one forensic examination of specific pairs (case_analysis_pairs),
--                        opened by an admin (requested_by); it leads to a forensic report (laudo).
-- Fingerprint analyses (crime scene, laboratory, documents) will join the same table.
--
-- An analysis is OPEN while it is worked and CLOSED when its report is issued.
CREATE TABLE case_analyses (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    analysis_type TEXT NOT NULL CHECK (analysis_type IN ('FACIAL_REVIEW', 'FACIAL_COMPARISON')),
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED')),
    requested_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    CONSTRAINT case_analyses_case_sequence_key UNIQUE (biometric_case_id, sequence),
    CONSTRAINT case_analyses_closed_check CHECK ((status = 'CLOSED') = (closed_at IS NOT NULL))
);
CREATE INDEX case_analyses_open_idx ON case_analyses (status, created_at, id);

-- The pairs a FACIAL_COMPARISON examines, as biometric_decisions keys them (graph feature ids,
-- feature_a_id < feature_b_id in byte order). A FACIAL_REVIEW covers every pair of its case's
-- faces and lists none here.
CREATE TABLE case_analysis_pairs (
    id BIGSERIAL PRIMARY KEY,
    case_analysis_id BIGINT NOT NULL REFERENCES case_analyses(id) ON DELETE CASCADE,
    feature_a_id TEXT NOT NULL,
    feature_b_id TEXT NOT NULL,
    CONSTRAINT case_analysis_pairs_order_check CHECK (feature_a_id COLLATE "C" < feature_b_id COLLATE "C"),
    CONSTRAINT case_analysis_pairs_key UNIQUE (case_analysis_id, feature_a_id, feature_b_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- analysis_assignments is the append-only history of who is responsible for each analysis: the
-- examiner who takes the first human step on its pairs (VERIFICATOR). Reviewers and
-- inconsistence examiners are chosen later and are not recorded here. An analysis's current
-- examiner is its latest row (current_analysis_assignments); one with no row, or whose latest
-- row has a NULL user_id (unassigned by an admin), is pending.
--   AUTOMATIC -- by distribution.Distribute (least-loaded active examiner of the modality);
--                assigned_by is NULL
--   MANUAL    -- by an admin, or by a user who created the case, who is assigned_by
CREATE TABLE analysis_assignments (
    id BIGSERIAL PRIMARY KEY,
    case_analysis_id BIGINT NOT NULL REFERENCES case_analyses(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id),
    method TEXT NOT NULL CHECK (method IN ('AUTOMATIC', 'MANUAL')),
    assigned_by BIGINT REFERENCES users(id),
    CONSTRAINT analysis_assignments_assigner_check CHECK (
        (method = 'AUTOMATIC' AND assigned_by IS NULL AND user_id IS NOT NULL)
        OR
        (method = 'MANUAL' AND assigned_by IS NOT NULL)
    ),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX analysis_assignments_analysis_idx ON analysis_assignments (case_analysis_id, id DESC);
CREATE INDEX analysis_assignments_user_idx ON analysis_assignments (user_id);

CREATE VIEW current_analysis_assignments AS
SELECT DISTINCT ON (case_analysis_id)
    id, case_analysis_id, user_id, method, assigned_by, assigned_at
FROM analysis_assignments
ORDER BY case_analysis_id, id DESC;

-- One report per analysis, its type fixed by the analysis type (analyses.ReportType). It is a
-- DRAFT until its examiner issues it, which numbers it "<year>.<sequence>" per report type and
-- closes the analysis. case_file_id points at the issued document (a case_files
-- 'forensic_report'), NULL until one is attached.
CREATE TABLE analysis_reports (
    id BIGSERIAL PRIMARY KEY,
    case_analysis_id BIGINT NOT NULL UNIQUE REFERENCES case_analyses(id) ON DELETE CASCADE,
    report_type TEXT NOT NULL CHECK (report_type IN ('FACIAL_REVIEW_REPORT', 'FACIAL_COMPARISON_REPORT')),
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ISSUED')),
    report_year INTEGER,
    report_number INTEGER,
    issued_by BIGINT REFERENCES users(id),
    issued_at TIMESTAMPTZ,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT analysis_reports_issued_check CHECK (
        (status = 'DRAFT' AND report_year IS NULL AND report_number IS NULL AND issued_by IS NULL AND issued_at IS NULL)
        OR
        (status = 'ISSUED' AND report_year IS NOT NULL AND report_number IS NOT NULL AND issued_by IS NOT NULL AND issued_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX analysis_reports_number_key ON analysis_reports (report_type, report_year, report_number);

-- Human decisions belong to the analysis they were made in, so one pair can be decided again in
-- a later round (a review, then a comparison). SYSTEM decisions belong to none. The one-row-per-
-- role rule now holds within each analysis (NULLS NOT DISTINCT keeps it for SYSTEM rows and for
-- human rows recorded before analyses existed).
ALTER TABLE biometric_decisions ADD COLUMN case_analysis_id BIGINT REFERENCES case_analyses(id);
ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_system_has_no_analysis_check
    CHECK (role != 'SYSTEM' OR case_analysis_id IS NULL);
ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_pair_role_key;
ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_pair_role_analysis_key
    UNIQUE NULLS NOT DISTINCT (feature_a_id, feature_b_id, role, case_analysis_id);
CREATE INDEX biometric_decisions_analysis_idx ON biometric_decisions (case_analysis_id);

DROP VIEW biometric_decision_sides;
CREATE VIEW biometric_decision_sides AS
SELECT id, feature_a_id AS feature_id, feature_b_id AS counterpart_id,
       modality, role, decision, system_source, username, confidence, threshold, notes,
       comparison_type, related_reference, related_reference_kind, responsible_user,
       decided_at, created_at, case_analysis_id
FROM biometric_decisions
UNION ALL
SELECT id, feature_b_id AS feature_id, feature_a_id AS counterpart_id,
       modality, role, decision, system_source, username, confidence, threshold, notes,
       comparison_type, related_reference, related_reference_kind, responsible_user,
       decided_at, created_at, case_analysis_id
FROM biometric_decisions;

-- Every FACIAL case already loaded gets its automatic review, as new ones do.
INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type)
SELECT id, 1, 'FACIAL_REVIEW' FROM biometric_cases WHERE modality = 'FACIAL';
INSERT INTO analysis_reports (case_analysis_id, report_type)
SELECT id, 'FACIAL_REVIEW_REPORT' FROM case_analyses;

-- +goose Down
DROP VIEW biometric_decision_sides;
CREATE VIEW biometric_decision_sides AS
SELECT id, feature_a_id AS feature_id, feature_b_id AS counterpart_id,
       modality, role, decision, system_source, username, confidence, threshold, notes,
       comparison_type, related_reference, related_reference_kind, responsible_user,
       decided_at, created_at
FROM biometric_decisions
UNION ALL
SELECT id, feature_b_id AS feature_id, feature_a_id AS counterpart_id,
       modality, role, decision, system_source, username, confidence, threshold, notes,
       comparison_type, related_reference, related_reference_kind, responsible_user,
       decided_at, created_at
FROM biometric_decisions;

-- Fails, as it should, if a pair was decided by the same role in two analyses.
DROP INDEX biometric_decisions_analysis_idx;
ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_pair_role_analysis_key;
ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_pair_role_key UNIQUE (feature_a_id, feature_b_id, role);
ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_system_has_no_analysis_check;
ALTER TABLE biometric_decisions DROP COLUMN case_analysis_id;

DROP TABLE analysis_reports;
DROP VIEW current_analysis_assignments;
DROP TABLE analysis_assignments;
DROP TABLE case_analysis_pairs;
DROP TABLE case_analyses;
