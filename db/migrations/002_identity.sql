-- +goose Up
-- The enrollment (KNOWN) hierarchy: person -< identity_document -< identity_register -<
-- identity_file. See MODEL.md section 2.1.
CREATE TABLE person (
    id BIGSERIAL PRIMARY KEY,
    person_id TEXT NOT NULL UNIQUE,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE identity_document (
    id BIGSERIAL PRIMARY KEY,
    person_id BIGINT REFERENCES person(id),
    document_number TEXT NOT NULL,
    document_type TEXT NOT NULL,
    -- Any government-issued taxpayer/fiscal identifier (e.g. Brazil's CPF).
    fiscal_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_document_type_number_key UNIQUE (document_type, document_number)
);

CREATE TABLE gender (
    code VARCHAR(1) PRIMARY KEY,
    label TEXT NOT NULL
);

INSERT INTO gender (code, label) VALUES
    ('M', 'Male'),
    ('F', 'Female'),
    ('O', 'Other'),
    ('U', 'Unknown');

CREATE TABLE identity_register (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES identity_document(id),
    register_number TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    parent_1_name TEXT NOT NULL,
    parent_1_gender VARCHAR(1) NOT NULL REFERENCES gender(code),
    parent_2_name TEXT NOT NULL,
    parent_2_gender VARCHAR(1) NOT NULL REFERENCES gender(code),
    birth_date TEXT,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The raw files produced by one enrollment event: a photo (-> a FACE_RECORD feature) and/or
-- a nist (-> a FINGERPRINT_TEMPLATE feature), distinguished by file_type + sequence.
CREATE TABLE identity_file (
    id BIGSERIAL PRIMARY KEY,
    register_id BIGINT NOT NULL REFERENCES identity_register(id),
    file_type TEXT NOT NULL,
    sequence SMALLINT NOT NULL DEFAULT 1,
    source_path TEXT NOT NULL,
    storage_ref TEXT NOT NULL,
    content_type TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_file_register_type_sequence_key UNIQUE (register_id, file_type, sequence)
);

CREATE INDEX identity_file_register_id_idx ON identity_file (register_id);

-- Person name search (person.SearchByName) runs substring matches (ILIKE '%term%') against a
-- population-scale table, which a btree index can't accelerate -- a trigram GIN index can.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX identity_register_name_trgm_idx ON identity_register
    USING GIN (name gin_trgm_ops);

-- +goose Down
DROP TABLE identity_file;
DROP TABLE identity_register;
DROP TABLE gender;
DROP TABLE identity_document;
DROP TABLE person;
