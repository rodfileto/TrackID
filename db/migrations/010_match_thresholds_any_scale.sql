-- +goose Up
-- match_thresholds now also versions template matchers' cutoffs (biometric_templates, e.g.
-- FINGERPRINT_SOURCEAFIS), whose scores are not similarities in (0, 1]: SourceAFIS's are
-- unbounded above, with 40 as its documented threshold. So the check keeps only what holds on
-- every scale -- a positive review cutoff at or below the confirm cutoff -- and the upper bound
-- of a similarity scale is checked in code (biometricmatch.Band.ValidateSimilarity), where the
-- embedding/template distinction is known. embedding_type holds a template_type for those rows.
ALTER TABLE match_thresholds DROP CONSTRAINT match_thresholds_band_check;
ALTER TABLE match_thresholds ADD CONSTRAINT match_thresholds_band_check CHECK (
    review_threshold > 0 AND review_threshold <= confirm_threshold
);

-- +goose Down
DELETE FROM match_thresholds WHERE confirm_threshold > 1;
ALTER TABLE match_thresholds DROP CONSTRAINT match_thresholds_band_check;
ALTER TABLE match_thresholds ADD CONSTRAINT match_thresholds_band_check CHECK (
    review_threshold > 0 AND review_threshold <= confirm_threshold AND confirm_threshold <= 1
);
