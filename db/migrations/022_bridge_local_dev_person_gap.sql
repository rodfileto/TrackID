-- +goose Up
-- Bridges another instance of the same local-dev-database gap
-- 016_bridge_local_dev_biometricfeature_gap.sql describes: some local
-- Postgres instances were seeded from a database that already had
-- goose_db_version rows for 002 (create_identity) and 011
-- (rename_identity_fields_to_english) marked applied, from an unrelated
-- project's schema that predates this repo's "person" table -- so
-- identity_document/identity_register came in with no person_id column and
-- the pre-rename cpf/data_nascimento names, and goose (which tracks by
-- version number, not by inspecting the schema) considered both migrations
-- already done and skipped them.
--
-- Every statement below is guarded so this is a harmless no-op wherever
-- 002/011 already ran for real.
CREATE TABLE IF NOT EXISTS person (
    id BIGSERIAL PRIMARY KEY,
    person_id TEXT NOT NULL UNIQUE,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE identity_document
    ADD COLUMN IF NOT EXISTS person_id BIGINT REFERENCES person(id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'identity_document' AND column_name = 'cpf'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'identity_document' AND column_name = 'fiscal_number'
    ) THEN
        ALTER TABLE identity_document RENAME COLUMN cpf TO fiscal_number;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'identity_register' AND column_name = 'data_nascimento'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'identity_register' AND column_name = 'birth_date'
    ) THEN
        ALTER TABLE identity_register RENAME COLUMN data_nascimento TO birth_date;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Not reversible: on a database where this migration was a real bridge (not
-- a no-op), going down would have to guess whether identity_document rows
-- had a person before this ran, which isn't recoverable from the schema
-- alone. Matches 016's own down migration in only reverting the pieces it
-- unconditionally owns.
ALTER TABLE identity_document DROP COLUMN IF EXISTS person_id;
