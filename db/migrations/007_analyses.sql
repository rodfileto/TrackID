-- +goose Up
-- A biometric case is worked through one or more analyses: rounds of examination, each of a
-- type with its own methodology, examiners and report. For faces (FISWG):
--   FACIAL_REVIEW     -- one-to-many: the examiner reviews the facial recognition system's
--                        candidates for the case's faces; the result is an investigative lead or a
--                        negative result, not a forensic examination. Every FACIAL case gets one
--                        automatically (requested_by NULL).
--   FACIAL_COMPARISON -- one-to-one forensic examination of specific pairs (case_analysis_pairs),
--                        opened by a coordinator (requested_by); it leads to a forensic report
--                        (laudo).
-- For fingerprints, FINGERPRINT_COMPARISON (confronto papiloscópico); the laboratory and scene
-- examinations (FINGERPRINT_LAB_EXAMINATION, CRIME_SCENE_EXAMINATION) are reserved, not offered
-- yet (docs/request-routing-plan.md). case_request_id is the request the analysis answers.
--
-- An analysis is OPEN while it is worked and CLOSED when its report is issued. While open it
-- goes through stages, each worked by one examiner in one step (the biometric_decisions role
-- their decisions take):
--   EXAMINATION   -- its examiner decides every pair (VERIFICATOR) and submits
--   VERIFICATION  -- a second examiner decides every pair again, blind to the first (REVIEWER)
--   INCONSISTENCE -- only when they disagreed on a pair: a third examiner decides those pairs
--   REPORT        -- every pair settled; the first examiner issues the report, closing it
-- An analysis with no pairs to decide goes from EXAMINATION straight to REPORT.
CREATE TABLE case_analyses (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    analysis_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED')),
    stage TEXT NOT NULL DEFAULT 'EXAMINATION'
        CHECK (stage IN ('EXAMINATION', 'VERIFICATION', 'INCONSISTENCE', 'REPORT')),
    requested_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    case_request_id BIGINT REFERENCES case_requests(id) ON DELETE SET NULL,
    CONSTRAINT case_analyses_analysis_type_check CHECK (analysis_type IN (
        'FACIAL_REVIEW', 'FACIAL_COMPARISON', 'FINGERPRINT_COMPARISON',
        'FINGERPRINT_LAB_EXAMINATION', 'CRIME_SCENE_EXAMINATION')),
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

-- analysis_assignments is the append-only history of who holds each step of each analysis. An
-- analysis has one current examiner per step, its latest row for that step
-- (current_analysis_assignments), each distributed when its stage comes; a step with no row, or
-- whose latest row has a NULL user_id (unassigned by a coordinator), is pending. One person never
-- holds two steps of one analysis.
--   AUTOMATIC -- by distribution.Distribute; assigned_by is NULL
--   MANUAL    -- by a coordinator, or by a user who created the case, who is assigned_by
CREATE TABLE analysis_assignments (
    id BIGSERIAL PRIMARY KEY,
    case_analysis_id BIGINT NOT NULL REFERENCES case_analyses(id) ON DELETE CASCADE,
    step TEXT NOT NULL CHECK (step IN ('VERIFICATOR', 'REVIEWER', 'INCONSISTENCE')),
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
CREATE INDEX analysis_assignments_analysis_idx ON analysis_assignments (case_analysis_id, step, id DESC);
CREATE INDEX analysis_assignments_user_idx ON analysis_assignments (user_id);

CREATE VIEW current_analysis_assignments AS
SELECT DISTINCT ON (case_analysis_id, step)
    id, case_analysis_id, step, user_id, method, assigned_by, assigned_at
FROM analysis_assignments
ORDER BY case_analysis_id, step, id DESC;

-- A laudo's conclusion on a pair is one of the five opinion categories of OSAC 2022-S-0001
-- (Standard Guide for Image Comparison Opinions), which FISWG's ACE-V guideline for one-to-one
-- examinations points to (docs/facial-comparison-tool-plan.md); NULL outside a FACIAL_COMPARISON.
-- Every category maps onto decision (analyses.ConclusionDecision), so the rounds logic reads
-- decision alone. conclusion_decision gives NULL for a value outside the scale, and a CHECK passes
-- on NULL, so the tables also name the scale's values outright.
-- +goose StatementBegin
CREATE FUNCTION conclusion_decision(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE
        WHEN conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON') THEN 'POSITIVE'
        WHEN conclusion = 'INCONCLUSIVE' THEN 'INCONCLUSIVE'
        WHEN conclusion IN ('SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT') THEN 'NEGATIVE'
    END
$$;
-- +goose StatementEnd

-- The lowest laudo conclusion that counts as an identification, as data: one append-only version
-- per row, the latest current. With none, STRONG_SUPPORT_COMMON (cluster.DefaultIdentificationLevel).
CREATE TABLE identification_levels (
    id BIGSERIAL PRIMARY KEY,
    minimum_conclusion TEXT NOT NULL CHECK (conclusion_decision(minimum_conclusion) = 'POSITIVE'),
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identification_levels_scale_check CHECK (
        minimum_conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON'))
);

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
    conclusion TEXT,
    PRIMARY KEY (case_analysis_id, step, feature_a_id, feature_b_id),
    CONSTRAINT analysis_decision_drafts_conclusion_check CHECK (
        conclusion IS NULL OR conclusion_decision(conclusion) = decision),
    CONSTRAINT analysis_decision_drafts_conclusion_scale_check CHECK (
        conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON', 'INCONCLUSIVE', 'SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT'))
);


-- biometric_decisions is the append-only event log of every decision made about one pair of
-- biometric features (fingerprint or face), whether made automatically (an AFIS score, an ANN
-- similarity) or by a human examiner. This is the source of truth for a pair's confirmation state
-- -- clusters (the clusters tables) are a view derived from it, never
-- written authoritatively. See MODEL.md section 3.
--
-- Within one round, a pair goes through up to four roles, enforced structurally below (one row
-- per pair, role and analysis) but sequenced by the application layer, not Postgres:
--   SYSTEM        -- always automatic, always first if present, always optional
--   VERIFICATOR   -- the first human decision (a pair can go straight here with no prior SYSTEM row)
--   REVIEWER      -- a second, independent human decision; agreeing with VERIFICATOR settles the pair
--   INCONSISTENCE -- only recorded when REVIEWER disagrees with VERIFICATOR; whatever this role
--                    decides is final
--
-- Human decisions belong to the analysis they were made in (case_analysis_id), so one pair can be
-- decided again in a later round (a review, then a comparison); SYSTEM decisions belong to none.
-- NULLS NOT DISTINCT keeps the one-row-per-role rule for SYSTEM rows too.
--
-- feature_a_id/feature_b_id are the graph feature ids ("TRACE:<case_trace_id>#feature" for a
-- QUESTIONED feature, the bare identity_files.id for a KNOWN one) -- kept as opaque TEXT here since
-- Postgres never needs to interpret them, only to store and index them. Always stored with
-- feature_a_id < feature_b_id in byte order (COLLATE "C", so it agrees with a plain Go "<" --
-- under e.g. en_US, "TRACE:1#feature" vs "TRACE:10#feature" would disagree) so a pair
-- rediscovered from either direction lands under one consistent key.
--
-- modality uses biometric_cases.modality's vocabulary (FACIAL, FINGERPRINT).
--
-- One row per decision event -- never updated or deleted: the full chain of every round matters
-- for audit, not just the outcome.
CREATE TABLE biometric_decisions (
    id BIGSERIAL PRIMARY KEY,

    feature_a_id TEXT NOT NULL,
    feature_b_id TEXT NOT NULL,
    CONSTRAINT biometric_decisions_feature_order_check CHECK (feature_a_id COLLATE "C" < feature_b_id COLLATE "C"),

    modality TEXT NOT NULL CHECK (modality IN ('FACIAL', 'FINGERPRINT')),

    role TEXT NOT NULL CHECK (role IN ('SYSTEM', 'VERIFICATOR', 'REVIEWER', 'INCONSISTENCE')),
    case_analysis_id BIGINT REFERENCES case_analyses(id),
    CONSTRAINT biometric_decisions_system_has_no_analysis_check
        CHECK (role != 'SYSTEM' OR case_analysis_id IS NULL),
    CONSTRAINT biometric_decisions_pair_role_analysis_key
        UNIQUE NULLS NOT DISTINCT (feature_a_id, feature_b_id, role, case_analysis_id),

    decision TEXT NOT NULL CHECK (decision IN ('POSITIVE', 'NEGATIVE', 'INCONCLUSIVE')),

    -- Exactly one of the two identifies who/what decided, determined by role: SYSTEM rows carry
    -- system_source and leave username NULL; VERIFICATOR/REVIEWER/INCONSISTENCE rows carry username
    -- and leave system_source NULL.
    system_source TEXT,
    username TEXT,
    CONSTRAINT biometric_decisions_decider_check CHECK (
        (role = 'SYSTEM' AND system_source IS NOT NULL AND username IS NULL)
        OR
        (role != 'SYSTEM' AND username IS NOT NULL AND system_source IS NULL)
    ),

    -- confidence is the raw score behind a SYSTEM decision; threshold is the cutoff it was
    -- classified against at decision time. Both NULL for human decisions.
    confidence DOUBLE PRECISION,
    threshold DOUBLE PRECISION,
    CONSTRAINT biometric_decisions_human_has_no_score_check CHECK (
        role = 'SYSTEM' OR (confidence IS NULL AND threshold IS NULL)
    ),

    -- Examiner rationale, most useful on an INCONSISTENCE row.
    notes TEXT,

    -- An external record the decision was derived from, with the kind naming what sort of
    -- reference it is: the match_thresholds version a SYSTEM decision was classified under
    -- (kind 'match_threshold', the row id), or a row in an imported dataset.
    related_reference TEXT,
    related_reference_kind TEXT,

    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- A laudo's conclusion on the pair (see conclusion_decision above); human rows only.
    conclusion TEXT,
    CONSTRAINT biometric_decisions_conclusion_check CHECK (
        conclusion IS NULL OR (role != 'SYSTEM' AND conclusion_decision(conclusion) = decision)),
    CONSTRAINT biometric_decisions_conclusion_scale_check CHECK (
        conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON', 'INCONCLUSIVE', 'SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT'))
);

CREATE INDEX biometric_decisions_pair_idx ON biometric_decisions (feature_a_id, feature_b_id, decided_at);
-- pair_idx leads with feature_a_id, so "every decision touching feature X" would scan it whole to
-- find X on the b side; this covers that side.
CREATE INDEX biometric_decisions_pair_reverse_idx ON biometric_decisions (feature_b_id, feature_a_id);
CREATE INDEX biometric_decisions_analysis_idx ON biometric_decisions (case_analysis_id);

-- Each decision once from each feature's point of view (feature_id, counterpart_id), so a reader
-- asking about a feature filters one column instead of handling both orientations itself. A
-- filter on feature_id is pushed into both UNION ALL branches and uses the index for that side.
CREATE VIEW biometric_decision_sides AS
SELECT id, feature_a_id AS feature_id, feature_b_id AS counterpart_id,
       modality, role, case_analysis_id, decision, system_source, username, confidence, threshold,
       notes, related_reference, related_reference_kind, decided_at, created_at, conclusion
FROM biometric_decisions
UNION ALL
SELECT id, feature_b_id AS feature_id, feature_a_id AS counterpart_id,
       modality, role, case_analysis_id, decision, system_source, username, confidence, threshold,
       notes, related_reference, related_reference_kind, decided_at, created_at, conclusion
FROM biometric_decisions;

-- +goose Down
DROP VIEW biometric_decision_sides;
DROP TABLE biometric_decisions;
DROP TABLE analysis_decision_drafts;
DROP TABLE identification_levels;
DROP FUNCTION conclusion_decision(TEXT);
DROP VIEW current_analysis_assignments;
DROP TABLE analysis_assignments;
DROP TABLE case_analysis_pairs;
DROP TABLE case_analyses;
