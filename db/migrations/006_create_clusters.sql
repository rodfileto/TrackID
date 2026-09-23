-- +goose Up
-- Stable biometric clusters. The id is a sequential number assigned once and never
-- reused (BIGSERIAL), so reports can cite it. cluster_merges is an append-only audit
-- trail with no FK: a merged-away cluster's row is deleted, but its number is kept here
-- so old citations resolve to the surviving cluster. See MODEL.md section 4.
--
-- cluster_members groups biometric features (not raw evidence items): feature_id is the
-- feature's graph id (bare identity_file.id, or "TRACE:<case_trace_id>#feature").
-- modality uses biometric_cases.modality's vocabulary. A cluster spans cases, so it has no
-- case_type: one person can link a CRIMINAL case to a CIVIL one.
CREATE TABLE clusters (
    id BIGSERIAL PRIMARY KEY,
    modality TEXT NOT NULL CHECK (modality IN ('FACIAL', 'FINGERPRINT')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cluster_members (
    cluster_id BIGINT NOT NULL REFERENCES clusters(id),
    feature_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cluster_members_feature_key UNIQUE (feature_id)
);

CREATE INDEX cluster_members_cluster_id_idx ON cluster_members (cluster_id);

CREATE TABLE cluster_merges (
    id BIGSERIAL PRIMARY KEY,
    from_cluster_id BIGINT NOT NULL,
    to_cluster_id BIGINT NOT NULL,
    merged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    merged_by TEXT,
    reason TEXT
);

-- +goose Down
DROP TABLE cluster_merges;
DROP TABLE cluster_members;
DROP TABLE clusters;
