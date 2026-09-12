-- +goose Up
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    nome TEXT NOT NULL,
    ultimo_nome TEXT NOT NULL,
    matricula TEXT NOT NULL UNIQUE,
    cargo TEXT NOT NULL,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE criminal_cases (
    id BIGSERIAL PRIMARY KEY,
    case_id TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    responsible_user TEXT,
    comparison_type TEXT,
    related_reference TEXT,
    related_reference_kind TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX criminal_cases_related_reference_idx ON criminal_cases (related_reference);
CREATE INDEX criminal_cases_comparison_type_idx ON criminal_cases (comparison_type);
CREATE INDEX criminal_cases_description_idx ON criminal_cases USING GIN (to_tsvector('simple', description));
