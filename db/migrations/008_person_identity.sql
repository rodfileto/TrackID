-- +goose Up
CREATE TABLE person (
    id BIGSERIAL PRIMARY KEY,
    person_id TEXT NOT NULL UNIQUE,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE identity_document ADD COLUMN person_id BIGINT REFERENCES person(id);

CREATE TABLE codifications (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL,
    format TEXT NOT NULL,
    version TEXT NOT NULL,
    payload_ref TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT codifications_trace_format_version_key UNIQUE (trace_id, format, version),
    CONSTRAINT codifications_trace_id_fkey FOREIGN KEY (trace_id) REFERENCES criminal_cases(case_id)
);

CREATE TABLE identifications (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL,
    identity_register_id BIGINT NOT NULL,
    trace_codification_id BIGINT,
    confidence DOUBLE PRECISION,
    responsible_user TEXT,
    source TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identifications_trace_register_key UNIQUE (trace_id, identity_register_id),
    CONSTRAINT identifications_trace_id_fkey FOREIGN KEY (trace_id) REFERENCES criminal_cases(case_id),
    CONSTRAINT identifications_register_id_fkey FOREIGN KEY (identity_register_id) REFERENCES identity_register(id),
    CONSTRAINT identifications_codification_id_fkey FOREIGN KEY (trace_codification_id) REFERENCES codifications(id)
);
