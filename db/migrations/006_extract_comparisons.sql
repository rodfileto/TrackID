-- +goose Up
-- Extract comparisons out of criminal_cases. criminal_cases stays the evidence
-- identity (one row per case/evidence item); comparisons models the edge between
-- two evidence items. Person references (related_reference_kind <> 'CRIMINAL_CASE')
-- are deferred until identified-person linkage is in scope, so they are not migrated.

CREATE TABLE comparisons (
    id BIGSERIAL PRIMARY KEY,
    evidence_a TEXT NOT NULL,
    evidence_b TEXT NOT NULL,
    case_type TEXT NOT NULL CHECK (case_type IN ('FACIAL', 'FINGERPRINT')),
    comparison_type TEXT NOT NULL,
    responsible_user TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT comparisons_evidence_pair_key UNIQUE (evidence_a, evidence_b)
);

CREATE INDEX comparisons_evidence_a_idx ON comparisons (evidence_a);
CREATE INDEX comparisons_evidence_b_idx ON comparisons (evidence_b);
CREATE INDEX comparisons_case_type_idx ON comparisons (case_type);

INSERT INTO comparisons (evidence_a, evidence_b, case_type, comparison_type, responsible_user)
SELECT case_id, related_reference, case_type, comparison_type, responsible_user
FROM criminal_cases
WHERE related_reference IS NOT NULL
  AND related_reference_kind = 'CRIMINAL_CASE'
  AND comparison_type IS NOT NULL;

ALTER TABLE criminal_cases
    DROP COLUMN responsible_user,
    DROP COLUMN comparison_type,
    DROP COLUMN related_reference,
    DROP COLUMN related_reference_kind;
