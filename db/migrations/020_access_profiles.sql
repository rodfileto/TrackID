-- +goose Up
-- Two more capability flags beside examines_fingerprint / examines_facial (migration 016), so the
-- account role goes back to what 016 said it was: account management. See
-- docs/access-profiles-plan.md.
--
-- intelligence_analyst -- reads every biometric case and person (full person page, clusters,
--                         face search, occurrences) and links occurrences to cases; edits a
--                         case's evidence only where they hold an analysis, as an examiner does.
-- coordinator          -- distributes analyses, reassigns their steps and opens comparisons on
--                         chosen pairs: the work admins did until now. Every current admin gets
--                         it, so distribution keeps working after the switch.
--
-- Whatever a user's flags, while they work an analysis blind (analyses.BlindWork) they see that
-- analysis's images and nothing else.
ALTER TABLE users
    ADD COLUMN intelligence_analyst BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN coordinator BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE users SET coordinator = TRUE WHERE account_role IN ('ADMIN', 'SUPER_ADMIN');

-- +goose Down
ALTER TABLE users
    DROP COLUMN coordinator,
    DROP COLUMN intelligence_analyst;
