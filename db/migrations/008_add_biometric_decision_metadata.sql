-- +goose Up
-- Generic annotation for what produced/asserted a biometric_decisions row, beyond the
-- decider itself (system_source/username, already on the table). comparison_type is a
-- free-form label for the kind of comparison (e.g. an AFIS lift-vs-ten-print comparison
-- family, or an organization's own vocabulary); related_reference/related_reference_kind
-- let a decision cite an external record it was derived from (e.g. a row in a
-- ground-truth dataset an organization imported), with related_reference_kind naming what
-- kind of external reference it is so consumers don't have to guess. responsible_user
-- records who is accountable for a decision that wasn't entered by that same user (e.g. a
-- SYSTEM decision seeded from a dataset a specific analyst is responsible for).
--
-- These are nullable, org-optional annotations, not part of the pair-role identity
-- (biometric_decisions_pair_role_key is unchanged): two organizations comparing the same
-- modality differ only in which biometricfeatures are on each side and what metadata (if
-- any) they attach here, never in table shape.
ALTER TABLE biometric_decisions
    ADD COLUMN comparison_type TEXT,
    ADD COLUMN related_reference TEXT,
    ADD COLUMN related_reference_kind TEXT,
    ADD COLUMN responsible_user TEXT;
