-- +goose Up
-- Three fixes to biometric_decisions' pair keying (see 005):
--
-- 1. The feature order check compared under the database's collation. Under en_US that
--    disagrees with byte order for ids like "TRACE:1#feature" vs "TRACE:10#feature", so a
--    writer ordering the pair with a plain Go "<" (biometricmatch, import tools) hit the
--    check. It now compares bytes (COLLATE "C"), and any existing row stored in collation
--    order but not byte order is swapped. A swap cannot collide with
--    biometric_decisions_pair_role_key: a pair was only ever stored in one orientation.
--
-- 2. biometric_decisions_pair_idx leads with feature_a_id, so "every decision touching
--    feature X" had to scan it whole to find X on the b side. The reverse index covers that.
--
-- 3. biometric_decision_sides lists each decision once from each feature's point of view
--    (feature_id, counterpart_id), so a reader asking about a feature filters one column
--    instead of handling both orientations itself. A filter on feature_id is pushed into
--    both UNION ALL branches and uses the index for that side.
ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_feature_order_check;

UPDATE biometric_decisions
SET feature_a_id = feature_b_id, feature_b_id = feature_a_id
WHERE feature_a_id COLLATE "C" > feature_b_id COLLATE "C";

ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_feature_order_check
    CHECK (feature_a_id COLLATE "C" < feature_b_id COLLATE "C");

CREATE INDEX biometric_decisions_pair_reverse_idx ON biometric_decisions (feature_b_id, feature_a_id);

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

-- +goose Down
DROP VIEW biometric_decision_sides;
DROP INDEX biometric_decisions_pair_reverse_idx;

ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_feature_order_check;

UPDATE biometric_decisions
SET feature_a_id = feature_b_id, feature_b_id = feature_a_id
WHERE feature_a_id > feature_b_id;

ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_feature_order_check
    CHECK (feature_a_id < feature_b_id);
