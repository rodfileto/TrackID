-- +goose Up
-- An open analysis goes through stages, each worked by one examiner in one step (the
-- biometric_decisions role their decisions take):
--   EXAMINATION   -- its examiner decides every pair (VERIFICATOR) and submits
--   VERIFICATION  -- a second examiner decides every pair again, blind to the first (REVIEWER)
--   INCONSISTENCE -- only when they disagreed on a pair: a third examiner decides those pairs
--   REPORT        -- every pair settled; the first examiner issues the report, closing it
-- An analysis with no pairs to decide goes from EXAMINATION straight to REPORT.
ALTER TABLE case_analyses ADD COLUMN stage TEXT NOT NULL DEFAULT 'EXAMINATION'
    CHECK (stage IN ('EXAMINATION', 'VERIFICATION', 'INCONSISTENCE', 'REPORT'));

-- Assignments are now per step: an analysis has one current examiner per step (the latest row
-- for that step), each distributed when its stage comes. One person never holds two steps of one
-- analysis.
ALTER TABLE analysis_assignments ADD COLUMN step TEXT NOT NULL DEFAULT 'VERIFICATOR'
    CHECK (step IN ('VERIFICATOR', 'REVIEWER', 'INCONSISTENCE'));
DROP INDEX analysis_assignments_analysis_idx;
CREATE INDEX analysis_assignments_analysis_idx ON analysis_assignments (case_analysis_id, step, id DESC);
DROP VIEW current_analysis_assignments;
CREATE VIEW current_analysis_assignments AS
SELECT DISTINCT ON (case_analysis_id, step)
    id, case_analysis_id, step, user_id, method, assigned_by, assigned_at
FROM analysis_assignments
ORDER BY case_analysis_id, step, id DESC;

-- An examiner's decisions are drafts while they work, changeable until they submit the step; the
-- submit writes them to biometric_decisions (append-only, one row per pair and role in the
-- analysis) and deletes them. Drafts are only ever shown to the examiner holding the step, and a
-- reassignment of the step discards them.
CREATE TABLE analysis_decision_drafts (
    case_analysis_id BIGINT NOT NULL REFERENCES case_analyses(id) ON DELETE CASCADE,
    step TEXT NOT NULL CHECK (step IN ('VERIFICATOR', 'REVIEWER', 'INCONSISTENCE')),
    feature_a_id TEXT NOT NULL,
    feature_b_id TEXT NOT NULL,
    CONSTRAINT analysis_decision_drafts_order_check CHECK (feature_a_id COLLATE "C" < feature_b_id COLLATE "C"),
    decision TEXT NOT NULL CHECK (decision IN ('POSITIVE', 'NEGATIVE', 'INCONCLUSIVE')),
    notes TEXT,
    user_id BIGINT NOT NULL REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (case_analysis_id, step, feature_a_id, feature_b_id)
);

-- +goose Down
DROP TABLE analysis_decision_drafts;
DROP VIEW current_analysis_assignments;
DROP INDEX analysis_assignments_analysis_idx;
-- 017 knew one examiner per analysis: the later steps' assignments go.
DELETE FROM analysis_assignments WHERE step <> 'VERIFICATOR';
ALTER TABLE analysis_assignments DROP COLUMN step;
CREATE INDEX analysis_assignments_analysis_idx ON analysis_assignments (case_analysis_id, id DESC);
CREATE VIEW current_analysis_assignments AS
SELECT DISTINCT ON (case_analysis_id)
    id, case_analysis_id, user_id, method, assigned_by, assigned_at
FROM analysis_assignments
ORDER BY case_analysis_id, id DESC;
ALTER TABLE case_analyses DROP COLUMN stage;
