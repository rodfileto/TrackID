-- +goose Up
-- Bridges a specific local-dev-database gap, not a normal schema step: some local
-- Postgres instances were seeded from a database that already had goose_db_version
-- rows for versions 13-15 applied by an unrelated project's own (differently
-- numbered) migrations against the same shared instance, from before biometricfeature/
-- clusters/feature_embeddings existed. goose tracks by version number only, so a file
-- named 013/014/015 here would be silently skipped as "already applied" on such a
-- database. This is numbered 016 -- the first number guaranteed unused -- specifically
-- to avoid that silent no-op; migrations 013-015 are intentionally absent from this
-- repo's history. A database that only ever ran this repo's own 001-012 already has
-- everything below and just gets a routine version bump.
--
-- Every statement is guarded (IF NOT EXISTS / a duplicate-column check) so this is a
-- harmless no-op wherever 004/006/007/008 already created the real thing.
CREATE TABLE IF NOT EXISTS biometricfeature (
    id BIGSERIAL PRIMARY KEY,
    feature_type TEXT NOT NULL CHECK (feature_type IN ('FINGERPRINT_TEMPLATE', 'FACE_RECORD', 'FINGERPRINT_LIFT', 'FACE_CAPTURE')),
    provenance TEXT NOT NULL CHECK (provenance IN ('KNOWN', 'QUESTIONED')),
    identity_file_id BIGINT REFERENCES identity_file(id) ON DELETE CASCADE,
    case_trace_id BIGINT REFERENCES case_traces(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT biometricfeature_exactly_one_source_check CHECK (
        (identity_file_id IS NOT NULL)::int + (case_trace_id IS NOT NULL)::int = 1
    ),
    CONSTRAINT biometricfeature_identity_file_id_key UNIQUE (identity_file_id),
    CONSTRAINT biometricfeature_case_trace_id_key UNIQUE (case_trace_id)
);
CREATE INDEX IF NOT EXISTS biometricfeature_feature_type_idx ON biometricfeature (feature_type);
CREATE INDEX IF NOT EXISTS biometricfeature_provenance_idx ON biometricfeature (provenance);

CREATE TABLE IF NOT EXISTS clusters (
    id BIGSERIAL PRIMARY KEY,
    case_type TEXT NOT NULL CHECK (case_type IN ('FACIAL', 'FINGERPRINT')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cluster_members (
    cluster_id BIGINT NOT NULL REFERENCES clusters(id),
    feature_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cluster_members_feature_key UNIQUE (feature_id)
);
CREATE INDEX IF NOT EXISTS cluster_members_cluster_id_idx ON cluster_members (cluster_id);

CREATE TABLE IF NOT EXISTS cluster_merges (
    id BIGSERIAL PRIMARY KEY,
    from_cluster_id BIGINT NOT NULL,
    to_cluster_id BIGINT NOT NULL,
    merged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    merged_by TEXT,
    reason TEXT
);

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS feature_embeddings (
    id BIGSERIAL PRIMARY KEY,
    biometricfeature_id BIGINT NOT NULL REFERENCES biometricfeature(id) ON DELETE CASCADE,
    embedding_type TEXT NOT NULL CHECK (embedding_type IN ('FACE_ARCFACE_512')),
    embedding VECTOR(512) NOT NULL,
    model_version TEXT,
    matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT feature_embeddings_feature_type_key UNIQUE (biometricfeature_id, embedding_type)
);
CREATE INDEX IF NOT EXISTS feature_embeddings_embedding_idx ON feature_embeddings
    USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS feature_embeddings_unmatched_idx ON feature_embeddings (embedding_type)
    WHERE matched_at IS NULL;

ALTER TABLE biometric_decisions
    ADD COLUMN IF NOT EXISTS comparison_type TEXT,
    ADD COLUMN IF NOT EXISTS related_reference TEXT,
    ADD COLUMN IF NOT EXISTS related_reference_kind TEXT,
    ADD COLUMN IF NOT EXISTS responsible_user TEXT;

-- +goose Down
ALTER TABLE biometric_decisions
    DROP COLUMN IF EXISTS comparison_type,
    DROP COLUMN IF EXISTS related_reference,
    DROP COLUMN IF EXISTS related_reference_kind,
    DROP COLUMN IF EXISTS responsible_user;

DROP TABLE IF EXISTS feature_embeddings;
DROP TABLE IF EXISTS cluster_merges;
DROP TABLE IF EXISTS cluster_members;
DROP TABLE IF EXISTS clusters;
DROP TABLE IF EXISTS biometricfeature;
