-- +goose Up
-- A notification tells one user that something needs them (docs/realtime-plan.md). A row holds only a kind and a
-- reference: what the user reads is built when they read it, under the access they have then, so a notification can
-- never carry what they may no longer see.
--
-- kind    -- ANALYSIS_ASSIGNED (subject: case_analyses.id)
-- subject -- text, as in access_log, so each kind names its own reference
-- read_at -- NULL while unread
CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('ANALYSIS_ASSIGNED')),
    subject TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ
);
CREATE INDEX notifications_user_idx ON notifications (user_id, id DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications;
