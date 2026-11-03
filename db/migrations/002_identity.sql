-- +goose Up
-- The enrollment (KNOWN) hierarchy: persons -< identity_documents -< identity_registers -<
-- identity_files. See MODEL.md section 2.1.
CREATE TABLE persons (
    id BIGSERIAL PRIMARY KEY,
    person_id TEXT NOT NULL UNIQUE,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE identity_documents (
    id BIGSERIAL PRIMARY KEY,
    person_id BIGINT REFERENCES persons(id),
    document_number TEXT NOT NULL,
    document_type TEXT NOT NULL,
    -- Any government-issued taxpayer/fiscal identifier (e.g. Brazil's CPF).
    fiscal_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_documents_type_number_key UNIQUE (document_type, document_number)
);

CREATE TABLE genders (
    code VARCHAR(1) PRIMARY KEY,
    label TEXT NOT NULL
);

INSERT INTO genders (code, label) VALUES
    ('M', 'Male'),
    ('F', 'Female'),
    ('O', 'Other'),
    ('U', 'Unknown');

CREATE TABLE identity_registers (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES identity_documents(id),
    register_number TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    parent_1_name TEXT NOT NULL,
    parent_1_gender VARCHAR(1) NOT NULL REFERENCES genders(code),
    parent_2_name TEXT NOT NULL,
    parent_2_gender VARCHAR(1) NOT NULL REFERENCES genders(code),
    birth_date TEXT,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Person name search (person.SearchByName) runs substring matches (ILIKE '%term%') against a
-- population-scale table, which a btree index can't accelerate -- a trigram GIN index can.
CREATE INDEX identity_registers_name_trgm_idx ON identity_registers
    USING GIN (name gin_trgm_ops);

-- The raw files produced by one enrollment event: a photo (-> a FACE_RECORD feature) and/or
-- a nist (-> a FINGERPRINT_TEMPLATE feature), distinguished by file_type + sequence.
CREATE TABLE identity_files (
    id BIGSERIAL PRIMARY KEY,
    register_id BIGINT NOT NULL REFERENCES identity_registers(id),
    file_type TEXT NOT NULL,
    sequence SMALLINT NOT NULL DEFAULT 1,
    source_path TEXT NOT NULL,
    storage_ref TEXT NOT NULL,
    content_type TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_files_register_type_sequence_key UNIQUE (register_id, file_type, sequence)
);

CREATE INDEX identity_files_register_id_idx ON identity_files (register_id);

-- +goose Down
DROP TABLE identity_files;
DROP TABLE identity_registers;
DROP TABLE genders;
DROP TABLE identity_documents;
DROP TABLE persons;
