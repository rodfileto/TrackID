-- +goose Up
CREATE TABLE identity_document (
    id BIGSERIAL PRIMARY KEY,
    document_number TEXT NOT NULL,
    document_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
    document_id BIGINT NOT NULL,
    register_id TEXT NOT NULL,
    register_number TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name TEXT NOT NULL,
    parent_1_name TEXT NOT NULL,
    parent_1_gender VARCHAR(1) NOT NULL,
    parent_2_name TEXT NOT NULL,
    parent_2_gender VARCHAR(1) NOT NULL,
    CONSTRAINT identity_register_document_id_fkey FOREIGN KEY (document_id) REFERENCES identity_document(id),
    CONSTRAINT identity_register_parent_1_gender_fkey FOREIGN KEY (parent_1_gender) REFERENCES gender(code),
    CONSTRAINT identity_register_parent_2_gender_fkey FOREIGN KEY (parent_2_gender) REFERENCES gender(code)
);