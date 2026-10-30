-- +goose Up
-- The facial comparison's conclusion scale becomes the five opinion categories of OSAC
-- 2022-S-0001 (Standard Guide for Image Comparison Opinions), which FISWG's ACE-V guideline for
-- one-to-one examinations points to (docs/facial-comparison-tool-plan.md). The earlier working
-- scale had seven levels, two of them worded as identification and exclusion, which OSAC and
-- FISWG rule out: those become the strong categories, and the support levels between them the
-- plain ones.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION conclusion_decision(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE
        WHEN conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON') THEN 'POSITIVE'
        WHEN conclusion = 'INCONCLUSIVE' THEN 'INCONCLUSIVE'
        WHEN conclusion IN ('SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT') THEN 'NEGATIVE'
        -- The earlier scale, so the rows below still pass their CHECKs while they are rewritten.
        WHEN conclusion IN ('IDENTIFICATION', 'STRONG_SUPPORT', 'MODERATE_SUPPORT', 'LIMITED_SUPPORT') THEN 'POSITIVE'
        WHEN conclusion IN ('LIMITED_EXCLUSION', 'EXCLUSION') THEN 'NEGATIVE'
    END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION opinion_scale_up(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE conclusion
        WHEN 'IDENTIFICATION' THEN 'STRONG_SUPPORT_COMMON'
        WHEN 'STRONG_SUPPORT' THEN 'STRONG_SUPPORT_COMMON'
        WHEN 'MODERATE_SUPPORT' THEN 'SUPPORT_COMMON'
        WHEN 'LIMITED_SUPPORT' THEN 'SUPPORT_COMMON'
        WHEN 'LIMITED_EXCLUSION' THEN 'SUPPORT_DIFFERENT'
        WHEN 'EXCLUSION' THEN 'STRONG_SUPPORT_DIFFERENT'
        ELSE conclusion
    END
$$;
-- +goose StatementEnd

UPDATE biometric_decisions SET conclusion = opinion_scale_up(conclusion) WHERE conclusion IS NOT NULL;
UPDATE analysis_decision_drafts SET conclusion = opinion_scale_up(conclusion) WHERE conclusion IS NOT NULL;
UPDATE identification_levels SET minimum_conclusion = opinion_scale_up(minimum_conclusion);
DROP FUNCTION opinion_scale_up(TEXT);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION conclusion_decision(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE
        WHEN conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON') THEN 'POSITIVE'
        WHEN conclusion = 'INCONCLUSIVE' THEN 'INCONCLUSIVE'
        WHEN conclusion IN ('SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT') THEN 'NEGATIVE'
    END
$$;
-- +goose StatementEnd

-- conclusion_decision gives NULL for a value outside the scale, and a CHECK passes on NULL: name
-- the scale's values outright.
ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_conclusion_scale_check CHECK (
    conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON', 'INCONCLUSIVE', 'SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT'));
ALTER TABLE analysis_decision_drafts ADD CONSTRAINT analysis_decision_drafts_conclusion_scale_check CHECK (
    conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON', 'INCONCLUSIVE', 'SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT'));
ALTER TABLE identification_levels ADD CONSTRAINT identification_levels_scale_check CHECK (
    minimum_conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON'));

-- +goose Down
-- Back to the seven-level working scale. The strong categories return as STRONG_SUPPORT and
-- EXCLUSION, the plain ones as MODERATE_SUPPORT and LIMITED_EXCLUSION.
ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_conclusion_scale_check;
ALTER TABLE analysis_decision_drafts DROP CONSTRAINT analysis_decision_drafts_conclusion_scale_check;
ALTER TABLE identification_levels DROP CONSTRAINT identification_levels_scale_check;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION conclusion_decision(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE
        WHEN conclusion IN ('IDENTIFICATION', 'STRONG_SUPPORT', 'MODERATE_SUPPORT', 'LIMITED_SUPPORT') THEN 'POSITIVE'
        WHEN conclusion = 'INCONCLUSIVE' THEN 'INCONCLUSIVE'
        WHEN conclusion IN ('LIMITED_EXCLUSION', 'EXCLUSION') THEN 'NEGATIVE'
        WHEN conclusion IN ('STRONG_SUPPORT_COMMON', 'SUPPORT_COMMON') THEN 'POSITIVE'
        WHEN conclusion IN ('SUPPORT_DIFFERENT', 'STRONG_SUPPORT_DIFFERENT') THEN 'NEGATIVE'
    END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION opinion_scale_down(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE conclusion
        WHEN 'STRONG_SUPPORT_COMMON' THEN 'STRONG_SUPPORT'
        WHEN 'SUPPORT_COMMON' THEN 'MODERATE_SUPPORT'
        WHEN 'SUPPORT_DIFFERENT' THEN 'LIMITED_EXCLUSION'
        WHEN 'STRONG_SUPPORT_DIFFERENT' THEN 'EXCLUSION'
        ELSE conclusion
    END
$$;
-- +goose StatementEnd

UPDATE biometric_decisions SET conclusion = opinion_scale_down(conclusion) WHERE conclusion IS NOT NULL;
UPDATE analysis_decision_drafts SET conclusion = opinion_scale_down(conclusion) WHERE conclusion IS NOT NULL;
UPDATE identification_levels SET minimum_conclusion = opinion_scale_down(minimum_conclusion);
DROP FUNCTION opinion_scale_down(TEXT);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION conclusion_decision(conclusion TEXT) RETURNS TEXT IMMUTABLE LANGUAGE SQL AS $$
    SELECT CASE
        WHEN conclusion IN ('IDENTIFICATION', 'STRONG_SUPPORT', 'MODERATE_SUPPORT', 'LIMITED_SUPPORT') THEN 'POSITIVE'
        WHEN conclusion = 'INCONCLUSIVE' THEN 'INCONCLUSIVE'
        WHEN conclusion IN ('LIMITED_EXCLUSION', 'EXCLUSION') THEN 'NEGATIVE'
    END
$$;
-- +goose StatementEnd
