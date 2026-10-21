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
--
-- Two cluster layers (basis). AUTOMATIC clusters are built from every CONFIRMED pair, a SYSTEM
-- match included: fast and wide, for intelligence (the Clusters page, the overview graph).
-- REVIEWED clusters are built only from pairs people confirmed (cluster.Reviewed: VERIFICATOR and
-- REVIEWER agreeing, or INCONSISTENCE): the only layer forensic views (a case, a person) read.
-- Both are reconciled the same way, each with its own stable ids and merge history; every
-- REVIEWED cluster sits inside one AUTOMATIC cluster. A feature belongs to one cluster per layer,
-- so cluster_members carries the basis too, kept equal to its cluster's by a composite FK.
CREATE TABLE clusters (
    id BIGSERIAL PRIMARY KEY,
    modality TEXT NOT NULL CHECK (modality IN ('FACIAL', 'FINGERPRINT')),
    basis TEXT NOT NULL CHECK (basis IN ('AUTOMATIC', 'REVIEWED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT clusters_id_basis_key UNIQUE (id, basis)
);

CREATE TABLE cluster_members (
    cluster_id BIGINT NOT NULL,
    basis TEXT NOT NULL,
    feature_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cluster_members_basis_feature_key UNIQUE (basis, feature_id),
    CONSTRAINT cluster_members_cluster_basis_fkey
        FOREIGN KEY (cluster_id, basis) REFERENCES clusters (id, basis)
);

CREATE INDEX cluster_members_cluster_id_idx ON cluster_members (cluster_id);

CREATE TABLE cluster_merges (
    id BIGSERIAL PRIMARY KEY,
    basis TEXT NOT NULL CHECK (basis IN ('AUTOMATIC', 'REVIEWED')),
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
