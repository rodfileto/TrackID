-- +goose Up
-- A KNOWN feature can be withdrawn: taken out of matching while its identity stays. An enrolled photo that came from
-- an outside registry because someone confirmed it, and was later found to be someone else, must stop matching new
-- evidence and stop being found by face search, yet its rows stay so the decisions and clusters that name it, and
-- who looked at it, still read. Withdrawing deletes the feature's embeddings; nothing embeds a withdrawn feature
-- again; and the AUTOMATIC cluster layer drops the links only a SYSTEM decision made to it. Links people confirmed
-- stay until people undo them (cluster package). Restoring clears the mark, and the feature is embedded again.
ALTER TABLE biometric_features
    ADD COLUMN withdrawn_at TIMESTAMPTZ,
    ADD COLUMN withdrawn_by TEXT, -- a username, or a system's name
    ADD COLUMN withdrawn_reason TEXT,
    ADD CONSTRAINT biometric_features_withdrawal_check CHECK (
        (withdrawn_at IS NULL) = (withdrawn_by IS NULL) AND (withdrawn_at IS NULL) = (withdrawn_reason IS NULL));
CREATE INDEX biometric_features_withdrawn_idx ON biometric_features (id) WHERE withdrawn_at IS NOT NULL;

-- +goose Down
DROP INDEX biometric_features_withdrawn_idx;
ALTER TABLE biometric_features
    DROP CONSTRAINT biometric_features_withdrawal_check,
    DROP COLUMN withdrawn_reason,
    DROP COLUMN withdrawn_by,
    DROP COLUMN withdrawn_at;
