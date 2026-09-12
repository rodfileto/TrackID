-- +goose Up
-- biometric_decisions is the append-only event log of every decision made about one pair of
-- biometric features (fingerprint or face), whether made automatically (an AFIS score, an ANN
-- similarity) or by a human examiner. This is the source of truth for a pair's confirmation state
-- -- cluster membership itself (Neo4j IN_CLUSTER, with a status derived from this chain) is a
-- derived view, never written authoritatively to the graph. See MODEL.md section 3.
--
-- Each pair progresses through up to four roles, enforced structurally below (one row per role per
-- pair) but sequenced by the application layer, not Postgres:
--   SYSTEM        -- always automatic, always first if present, always optional
--   VERIFICATOR   -- the first human decision (a pair can go straight here with no prior SYSTEM row)
--   REVIEWER      -- a second, independent human decision; agreeing with VERIFICATOR settles the pair
--   INCONSISTENCE -- only recorded when REVIEWER disagrees with VERIFICATOR; whatever this role
--                    decides is final
--
-- feature_a_id/feature_b_id are the graph feature ids ("TRACE:<case_trace_id>#feature" for a
-- QUESTIONED feature, the bare identity_file.id for a KNOWN one) -- kept as opaque TEXT here since
-- Postgres never needs to interpret them, only to store and index them. Always stored with
-- feature_a_id < feature_b_id (enforced below) so a pair rediscovered from either direction lands
-- under one consistent key.
--
-- One row per decision event -- never updated or deleted. A pair accumulates at most four rows over
-- its lifetime (one per role), and the full chain matters for audit, not just the outcome.
CREATE TABLE biometric_decisions (
    id BIGSERIAL PRIMARY KEY,

    feature_a_id TEXT NOT NULL,
    feature_b_id TEXT NOT NULL,
    CONSTRAINT biometric_decisions_feature_order_check CHECK (feature_a_id < feature_b_id),

    modality TEXT NOT NULL CHECK (modality IN ('FINGERPRINT', 'FACE')),

    role TEXT NOT NULL CHECK (role IN ('SYSTEM', 'VERIFICATOR', 'REVIEWER', 'INCONSISTENCE')),
    CONSTRAINT biometric_decisions_pair_role_key UNIQUE (feature_a_id, feature_b_id, role),

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

    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX biometric_decisions_pair_idx ON biometric_decisions (feature_a_id, feature_b_id, decided_at);
