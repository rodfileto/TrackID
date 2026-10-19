-- +goose Up
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
