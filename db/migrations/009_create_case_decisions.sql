-- +goose Up
-- case_decisions is an append-only log of decisions/status events about a criminal case
-- itself (e.g. a case being approved, rejected, or otherwise progressed through an
-- organization's own workflow) -- distinct from biometric_decisions, which is about a
-- pair of biometric features, not a case. decision is free-form: the vocabulary for what a
-- case decision can be is entirely organization-defined, so it isn't constrained here.
--
-- Mirrors biometric_decisions' decider pattern: exactly one of system_source/username
-- identifies who or what recorded the decision.
CREATE TABLE case_decisions (
    id BIGSERIAL PRIMARY KEY,
    criminal_case_id BIGINT NOT NULL REFERENCES criminal_cases(id) ON DELETE CASCADE,
    decision TEXT NOT NULL,
    system_source TEXT,
    username TEXT,
    CONSTRAINT case_decisions_decider_check CHECK (
        (system_source IS NOT NULL AND username IS NULL)
        OR
        (system_source IS NULL AND username IS NOT NULL)
    ),
    notes TEXT,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX case_decisions_criminal_case_id_idx ON case_decisions (criminal_case_id, decided_at);
