-- +goose Up
-- match_thresholds is the versioned configuration of the automatic matcher's cutoffs, one
-- append-only row per version; the current version of an embedding type is its latest row.
-- Operational thresholds belong to each deploying organization's own validation (a ROC
-- analysis on its labelled data, say), so they live here as data with their provenance rather
-- than as a constant in code, and changing one takes effect on the next matching run without a
-- restart.
--
-- Both cutoffs are similarities on the same scale as biometric_decisions.confidence (for a
-- face embedding, 1 - cosine distance). A pair at or above confirm_threshold gets a SYSTEM
-- POSITIVE, which clustering treats as CONFIRMED; a pair at or above review_threshold but
-- below confirm_threshold gets a SYSTEM INCONCLUSIVE, which is PENDING_REVIEW until an examiner
-- decides it; below review_threshold nothing is written. review_threshold = confirm_threshold
-- means no review band. Each decision cites the version it was classified under
-- (related_reference_kind 'match_threshold', related_reference the row id).
--
-- A new version applies to matching from then on; decisions already written stay, since
-- biometric_decisions is an append-only log.
CREATE TABLE match_thresholds (
    id BIGSERIAL PRIMARY KEY,
    embedding_type TEXT NOT NULL,
    review_threshold DOUBLE PRECISION NOT NULL,
    confirm_threshold DOUBLE PRECISION NOT NULL,
    source TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT match_thresholds_band_check CHECK (
        review_threshold > 0 AND review_threshold <= confirm_threshold AND confirm_threshold <= 1
    )
);

CREATE INDEX match_thresholds_current_idx ON match_thresholds (embedding_type, id DESC);

-- +goose Down
DROP TABLE match_thresholds;
