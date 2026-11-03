-- +goose Up
-- The extensions the schema needs, TrackID's user accounts, and the record of who read what.
--   postgis -- places and occurrences (003_occurrences)
--   vector  -- face embeddings (006_features)
--   pg_trgm -- person name search (002_identity)
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- A user has two independent things, kept apart on purpose:
--
-- account_role -- what the user may do to the system's accounts:
--   SUPER_ADMIN -- manages admins and users; the first one is created by cmd/create-superuser
--   ADMIN       -- manages plain users (creates them, sets their capabilities, deactivates them)
--   USER        -- no account management
--
-- capabilities -- what the user may do with the cases:
--   examines_fingerprint / examines_facial -- the biometrics the user may sign human decisions on:
--                           either, both, or neither (not an examiner). Within a modality they
--                           examine, a user may take any of the three human steps on a pair
--                           (VERIFICATOR, REVIEWER, INCONSISTENCE; see biometric_decisions); what
--                           limits them is independence (never two steps on the same pair),
--                           checked where decisions are recorded.
--   intelligence_analyst -- reads every biometric case and person (full person page, clusters,
--                           face search, occurrences) and links occurrences to cases; edits a
--                           case's evidence only where they hold an analysis, as an examiner does.
--   coordinator          -- distributes analyses, reassigns their steps and opens comparisons on
--                           chosen pairs.
-- Being an admin gives none of the capabilities, and no capability gives account management.
-- Whatever a user's capabilities, while they work an analysis blind (analyses.BlindWork) they see
-- that analysis's images and nothing else. See docs/access-profiles-plan.md.
--
-- staff_number is the user's number in their organization, used to sign in.
--
-- is_active -- users are deactivated, never deleted: biometric_decisions name their decider by
-- username, so the account has to stay. For the same reason a username never changes once created.
--
-- avatar is the user's own picture (PUT /api/me/avatar): a 256 x 256 JPEG the server made from the
-- upload. avatar_updated_at is NULL while there is none, and versions the image for caches.
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    staff_number TEXT NOT NULL UNIQUE,
    job_title TEXT NOT NULL,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    account_role TEXT NOT NULL DEFAULT 'USER' CHECK (account_role IN ('SUPER_ADMIN', 'ADMIN', 'USER')),
    examines_fingerprint BOOLEAN NOT NULL DEFAULT FALSE,
    examines_facial BOOLEAN NOT NULL DEFAULT FALSE,
    intelligence_analyst BOOLEAN NOT NULL DEFAULT FALSE,
    coordinator BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    avatar BYTEA,
    avatar_updated_at TIMESTAMPTZ,
    CONSTRAINT users_avatar_check CHECK ((avatar IS NULL) = (avatar_updated_at IS NULL))
);

-- access_log is the append-only record of who read a person's history or a case they do not
-- work on (docs/access-profiles-plan.md): every request an intelligence analyst makes to a full
-- person page, its clusters or cases, or a face search, and every request to a case by someone
-- who holds none of its analyses (an analyst or a coordinator). It answers "who looked at this
-- person, or this case, and when" (LGPD; the TrackID book, ch. 11). Never updated or deleted.
--
-- kind     -- PERSON (subject: person id), FACE_SEARCH (no subject), CASE (subject: case id)
-- route    -- the API route pattern, e.g. /api/persons/:personId/cases
-- username -- kept beside user_id, as decisions keep their decider's: usernames never change
CREATE TABLE access_log (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    username TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('PERSON', 'FACE_SEARCH', 'CASE')),
    subject TEXT,
    route TEXT NOT NULL,
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX access_log_subject_idx ON access_log (kind, subject, accessed_at DESC);
CREATE INDEX access_log_user_idx ON access_log (user_id, accessed_at DESC);

-- +goose Down
DROP TABLE access_log;
DROP TABLE users;
-- The extensions stay: other objects may come to depend on them.
