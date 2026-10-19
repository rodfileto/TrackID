-- +goose Up
-- Two cluster layers (see MODEL.md section 4). AUTOMATIC clusters are built from every CONFIRMED
-- pair, a SYSTEM match included: fast and wide, for intelligence (the Clusters page, the overview
-- graph). REVIEWED clusters are built only from pairs people confirmed (cluster.Reviewed:
-- VERIFICATOR and REVIEWER agreeing, or INCONSISTENCE): the only layer forensic views (a case, a
-- person) read. Both are reconciled the same way, each with its own stable ids and merge history;
-- every REVIEWED cluster sits inside one AUTOMATIC cluster. A feature belongs to one cluster per
-- layer, so cluster_members carries the basis too, kept equal to its cluster's by a composite FK.
-- Existing clusters were built from every CONFIRMED pair, so they become AUTOMATIC; the REVIEWED
-- layer is filled by the next cluster.Run.
ALTER TABLE clusters
    ADD COLUMN basis TEXT NOT NULL DEFAULT 'AUTOMATIC' CHECK (basis IN ('AUTOMATIC', 'REVIEWED'));
ALTER TABLE clusters ALTER COLUMN basis DROP DEFAULT;
ALTER TABLE clusters ADD CONSTRAINT clusters_id_basis_key UNIQUE (id, basis);

ALTER TABLE cluster_members ADD COLUMN basis TEXT NOT NULL DEFAULT 'AUTOMATIC';
ALTER TABLE cluster_members ALTER COLUMN basis DROP DEFAULT;
ALTER TABLE cluster_members DROP CONSTRAINT cluster_members_feature_key;
ALTER TABLE cluster_members ADD CONSTRAINT cluster_members_basis_feature_key UNIQUE (basis, feature_id);
ALTER TABLE cluster_members DROP CONSTRAINT cluster_members_cluster_id_fkey;
ALTER TABLE cluster_members ADD CONSTRAINT cluster_members_cluster_basis_fkey
    FOREIGN KEY (cluster_id, basis) REFERENCES clusters (id, basis);

ALTER TABLE cluster_merges ADD COLUMN basis TEXT NOT NULL DEFAULT 'AUTOMATIC'
    CHECK (basis IN ('AUTOMATIC', 'REVIEWED'));
ALTER TABLE cluster_merges ALTER COLUMN basis DROP DEFAULT;

-- +goose Down
DELETE FROM cluster_members WHERE basis = 'REVIEWED';
DELETE FROM clusters WHERE basis = 'REVIEWED';
ALTER TABLE cluster_merges DROP COLUMN basis;
ALTER TABLE cluster_members DROP CONSTRAINT cluster_members_cluster_basis_fkey;
ALTER TABLE cluster_members ADD CONSTRAINT cluster_members_cluster_id_fkey
    FOREIGN KEY (cluster_id) REFERENCES clusters (id);
ALTER TABLE cluster_members DROP CONSTRAINT cluster_members_basis_feature_key;
ALTER TABLE cluster_members ADD CONSTRAINT cluster_members_feature_key UNIQUE (feature_id);
ALTER TABLE cluster_members DROP COLUMN basis;
ALTER TABLE clusters DROP CONSTRAINT clusters_id_basis_key;
ALTER TABLE clusters DROP COLUMN basis;
