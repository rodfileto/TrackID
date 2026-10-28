-- +goose Up
-- A user's own picture (PUT /api/me/avatar): a 256 x 256 JPEG the server made from the upload.
-- avatar_updated_at is NULL while there is none, and versions the image for caches.
ALTER TABLE users ADD COLUMN avatar BYTEA,
    ADD COLUMN avatar_updated_at TIMESTAMPTZ,
    ADD CONSTRAINT users_avatar_check CHECK ((avatar IS NULL) = (avatar_updated_at IS NULL));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_avatar_check, DROP COLUMN avatar_updated_at, DROP COLUMN avatar;
