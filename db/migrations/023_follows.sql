-- +goose Up
-- Following persons and clusters (docs/follow-plan.md): an intelligence analyst follows a person or an AUTOMATIC
-- cluster and is told when it changes, in the bell and, when they set a webhook, in Microsoft Teams.

-- subject_kind -- PERSON (persons.person_id) or CLUSTER (clusters.id, AUTOMATIC)
CREATE TABLE follows (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('PERSON', 'CLUSTER')),
    subject_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, subject_kind, subject_id)
);
CREATE INDEX follows_subject_idx ON follows (subject_kind, subject_id);

-- What happened to a person or a cluster: the change log and the fan-out's queue (follows.Fanout).
--
-- change -- DOCUMENT_ADDED (ref: identity_documents.id), MEMBER_ADDED and MEMBER_LEFT (ref: feature id),
--           MERGED (ref: the surviving cluster), CLUSTER_JOINED (ref: cluster id), CASE_LINKED (ref: case_id),
--           or a deployment's own (follows.RecordFeature). Free text, so deployments add theirs.
-- amount -- how many of the change one row stands for, such as camera hits found by one search
-- fanned_out_at -- NULL until the fan-out has handled the row. A flag, not a cursor: transactions commit out of
--           id order, and a cursor would skip a row that committed late.
CREATE TABLE subject_changes (
    id BIGSERIAL PRIMARY KEY,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('PERSON', 'CLUSTER')),
    subject_id TEXT NOT NULL,
    change TEXT NOT NULL,
    ref TEXT NOT NULL DEFAULT '',
    amount INTEGER NOT NULL DEFAULT 1 CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    fanned_out_at TIMESTAMPTZ
);
CREATE INDEX subject_changes_pending_idx ON subject_changes (id) WHERE fanned_out_at IS NULL;
CREATE INDEX subject_changes_subject_idx ON subject_changes (subject_kind, subject_id, id);

-- The triggers record what happened, whichever process writes it, and wake the fan-out with an empty payload, so a
-- transaction's many inserts make one wake-up. What a change means for persons is left to the fan-out.
-- +goose StatementBegin
CREATE FUNCTION record_subject_change(kind TEXT, subject TEXT, what TEXT, reference TEXT) RETURNS void AS $$
BEGIN
    INSERT INTO subject_changes (subject_kind, subject_id, change, ref) VALUES (kind, subject, what, reference);
    PERFORM pg_notify('trackid_subject_changes', '');
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION identity_document_changed() RETURNS trigger AS $$
BEGIN
    PERFORM record_subject_change('PERSON', p.person_id, 'DOCUMENT_ADDED', NEW.id::text)
    FROM persons p WHERE p.id = NEW.person_id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER identity_document_added AFTER INSERT ON identity_documents FOR EACH ROW
    WHEN (NEW.person_id IS NOT NULL) EXECUTE FUNCTION identity_document_changed();
CREATE TRIGGER identity_document_moved AFTER UPDATE OF person_id ON identity_documents FOR EACH ROW
    WHEN (NEW.person_id IS NOT NULL AND NEW.person_id IS DISTINCT FROM OLD.person_id)
    EXECUTE FUNCTION identity_document_changed();

-- cluster.apply deletes and inserts a member only when it moves, so these see real changes only.
-- +goose StatementBegin
CREATE FUNCTION cluster_member_changed() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM record_subject_change('CLUSTER', NEW.cluster_id::text, 'MEMBER_ADDED', NEW.feature_id);
    ELSE
        PERFORM record_subject_change('CLUSTER', OLD.cluster_id::text, 'MEMBER_LEFT', OLD.feature_id);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER cluster_member_added AFTER INSERT ON cluster_members FOR EACH ROW
    WHEN (NEW.basis = 'AUTOMATIC') EXECUTE FUNCTION cluster_member_changed();
CREATE TRIGGER cluster_member_left AFTER DELETE ON cluster_members FOR EACH ROW
    WHEN (OLD.basis = 'AUTOMATIC') EXECUTE FUNCTION cluster_member_changed();

-- +goose StatementBegin
CREATE FUNCTION cluster_merged() RETURNS trigger AS $$
BEGIN
    PERFORM record_subject_change('CLUSTER', NEW.from_cluster_id::text, 'MERGED', NEW.to_cluster_id::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER cluster_merge_recorded AFTER INSERT ON cluster_merges FOR EACH ROW
    WHEN (NEW.basis = 'AUTOMATIC') EXECUTE FUNCTION cluster_merged();

-- Kinds are no longer fixed here: PERSON_CHANGED and CLUSTER_CHANGED join ANALYSIS_ASSIGNED (notifications package).
-- A user keeps one unread notification per followed subject, which further changes update: first_change_id and
-- last_change_id bound the subject's changes it reports. teams_sent_at is when Teams was last told of it.
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN first_change_id BIGINT,
    ADD COLUMN last_change_id BIGINT,
    ADD COLUMN teams_sent_at TIMESTAMPTZ;
CREATE UNIQUE INDEX notifications_unread_subject_idx ON notifications (user_id, kind, subject)
    WHERE read_at IS NULL AND kind IN ('PERSON_CHANGED', 'CLUSTER_CHANGED');
DROP INDEX notifications_user_idx;
CREATE INDEX notifications_user_idx ON notifications (user_id, updated_at DESC, id DESC);

-- A Microsoft Teams webhook (a Power Automate flow's URL), encrypted with SECRETS_KEY: anyone holding the URL can post
-- into the user's chat. The last failure is kept for the user to see.
ALTER TABLE users
    ADD COLUMN teams_webhook BYTEA,
    ADD COLUMN teams_webhook_set_at TIMESTAMPTZ,
    ADD COLUMN teams_last_error TEXT,
    ADD COLUMN teams_failed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users
    DROP COLUMN teams_failed_at,
    DROP COLUMN teams_last_error,
    DROP COLUMN teams_webhook_set_at,
    DROP COLUMN teams_webhook;
DROP INDEX notifications_user_idx;
CREATE INDEX notifications_user_idx ON notifications (user_id, id DESC);
DROP INDEX notifications_unread_subject_idx;
DELETE FROM notifications WHERE kind <> 'ANALYSIS_ASSIGNED';
ALTER TABLE notifications
    DROP COLUMN teams_sent_at,
    DROP COLUMN last_change_id,
    DROP COLUMN first_change_id,
    DROP COLUMN updated_at;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('ANALYSIS_ASSIGNED'));
DROP TRIGGER cluster_merge_recorded ON cluster_merges;
DROP FUNCTION cluster_merged();
DROP TRIGGER cluster_member_left ON cluster_members;
DROP TRIGGER cluster_member_added ON cluster_members;
DROP FUNCTION cluster_member_changed();
DROP TRIGGER identity_document_moved ON identity_documents;
DROP TRIGGER identity_document_added ON identity_documents;
DROP FUNCTION identity_document_changed();
DROP FUNCTION record_subject_change(TEXT, TEXT, TEXT, TEXT);
DROP TABLE subject_changes;
DROP TABLE follows;
