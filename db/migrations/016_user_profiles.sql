-- +goose Up
-- User profiles: two independent things about a user, kept apart on purpose.
--
-- account_role -- what the user may do to the system's accounts:
--   SUPER_ADMIN -- manages admins and users; the first one is created by cmd/create-superuser
--   ADMIN       -- manages plain users (creates them, sets what they examine, deactivates them)
--   USER        -- no account management
--
-- examines_fingerprint / examines_facial -- the biometrics the user may sign human decisions on:
-- either, both, or neither (not an examiner). Within a modality they examine, a user may take
-- any of the three human steps on a pair (VERIFICATOR, REVIEWER, INCONSISTENCE; see
-- biometric_decisions), the same rules for both modalities; what limits them is independence
-- (never two steps on the same pair), checked where decisions are recorded. Being an admin does
-- not make a user an examiner, nor the reverse.
--
-- is_active -- users are deactivated, never deleted: biometric_decisions and case_decisions
-- name their decider by username, so the account has to stay. For the same reason a username
-- never changes once created.
ALTER TABLE users
    ADD COLUMN account_role TEXT NOT NULL DEFAULT 'USER'
        CHECK (account_role IN ('SUPER_ADMIN', 'ADMIN', 'USER')),
    ADD COLUMN examines_fingerprint BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN examines_facial BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE users
    DROP COLUMN is_active,
    DROP COLUMN examines_facial,
    DROP COLUMN examines_fingerprint,
    DROP COLUMN account_role;
