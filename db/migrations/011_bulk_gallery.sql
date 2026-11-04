-- +goose Up
-- Bulk enrollment and deduplication of a large gallery (docs/large-gallery-plan.md): the tables
-- cmd/bulk-enroll and cmd/bulk-match write.

-- match_path says which matcher owns an embedding until it is first matched. SYNC embeddings are
-- matched by the per-face sync (embedding.SyncFace -> biometricmatch.RunBand), as before. BULK
-- embeddings, written by cmd/bulk-enroll, wait for cmd/bulk-match, so a trace arriving during an
-- upload never pulls the whole pending gallery into the sync.
ALTER TABLE feature_embeddings ADD COLUMN match_path TEXT NOT NULL DEFAULT 'SYNC'
    CONSTRAINT feature_embeddings_match_path_check CHECK (match_path IN ('SYNC', 'BULK'));
DROP INDEX feature_embeddings_unmatched_idx;
CREATE INDEX feature_embeddings_unmatched_idx ON feature_embeddings (embedding_type, match_path, id)
    WHERE matched_at IS NULL;

-- bulk_enroll_outcomes records what vectorizing one KNOWN face photo gave, for one embedding
-- type: one row per feature, written with its embedding (if any) in the same transaction.
--   EMBEDDED       -- one face found and embedded
--   MULTIPLE_FACES -- several faces found; the largest was embedded, and the photo is worth a look
--   NO_FACE        -- no face found; nothing embedded
--   UNDECODABLE    -- the file is not an image the decoder reads
--   FETCH_FAILED   -- the file could not be read; the only outcome a later run retries
-- faces is the number of faces found; detector_score and relative_size (face box area over image
-- area) describe the embedded face, for calibration (false-match hubs are mostly poor photos).
-- crop_ref is the stored face crop, when one was written.
CREATE TABLE bulk_enroll_outcomes (
    biometric_feature_id BIGINT NOT NULL REFERENCES biometric_features(id) ON DELETE CASCADE,
    embedding_type TEXT NOT NULL,
    model_version TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('EMBEDDED', 'MULTIPLE_FACES', 'NO_FACE', 'UNDECODABLE', 'FETCH_FAILED')),
    faces INT NOT NULL DEFAULT 0,
    detector_score REAL,
    relative_size REAL,
    image_width INT,
    image_height INT,
    crop_ref TEXT,
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (biometric_feature_id, embedding_type)
);
CREATE INDEX bulk_enroll_outcomes_outcome_idx ON bulk_enroll_outcomes (embedding_type, outcome);

-- bulk_match_batches is one batch of BULK embeddings matched by cmd/bulk-match: compared, exactly,
-- against itself, every earlier MATCHED batch of the same embedding type and model version, and
-- every QUESTIONED embedding. score_floor and neighbours (k) are what the score store kept: per
-- face, the neighbours scoring at least the floor, at most k of them. selection says how the batch
-- was chosen. A SELECTED batch is chosen but not matched yet; a rerun resumes it.
CREATE TABLE bulk_match_batches (
    id BIGSERIAL PRIMARY KEY,
    embedding_type TEXT NOT NULL,
    model_version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'SELECTED' CHECK (status IN ('SELECTED', 'MATCHED')),
    size INT NOT NULL,
    selection TEXT NOT NULL,
    score_floor REAL NOT NULL,
    neighbours INT NOT NULL CHECK (neighbours > 0),
    comparisons BIGINT,
    scores BIGINT,
    hubs INT,
    match_seconds DOUBLE PRECISION,
    matcher TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    matched_at TIMESTAMPTZ,
    CONSTRAINT bulk_match_batches_matched_check CHECK ((status = 'MATCHED') = (matched_at IS NOT NULL))
);

-- Each BULK embedding belongs to at most one batch.
CREATE TABLE bulk_match_batch_members (
    feature_embedding_id BIGINT PRIMARY KEY REFERENCES feature_embeddings(id) ON DELETE CASCADE,
    batch_id BIGINT NOT NULL REFERENCES bulk_match_batches(id) ON DELETE CASCADE
);
CREATE INDEX bulk_match_batch_members_batch_idx ON bulk_match_batch_members (batch_id, feature_embedding_id);

-- bulk_match_scores is the score store: every pair a batch kept, keyed by feature_embeddings ids
-- (embedding_a_id < embedding_b_id), with its similarity (1 - cosine distance, the scale of
-- biometric_decisions.confidence) and the batch that compared it. Append-only: a new threshold
-- version re-bands these scores (cmd/bulk-match apply) without matching again; only a new model
-- needs a new match. No foreign keys, for insert speed at gallery size; the ids are
-- feature_embeddings ids and the batch a bulk_match_batches id.
CREATE TABLE bulk_match_scores (
    embedding_a_id BIGINT NOT NULL,
    embedding_b_id BIGINT NOT NULL,
    score REAL NOT NULL,
    batch_id BIGINT NOT NULL,
    PRIMARY KEY (embedding_a_id, embedding_b_id),
    CONSTRAINT bulk_match_scores_order_check CHECK (embedding_a_id < embedding_b_id)
);
CREATE INDEX bulk_match_scores_b_idx ON bulk_match_scores (embedding_b_id);

-- bulk_match_hubs lists the faces whose k-th neighbour still scored at least the floor in a batch:
-- they hit the cap, so their lists are cut, and FMR estimates count them apart.
CREATE TABLE bulk_match_hubs (
    batch_id BIGINT NOT NULL REFERENCES bulk_match_batches(id) ON DELETE CASCADE,
    feature_embedding_id BIGINT NOT NULL,
    kth_score REAL NOT NULL,
    PRIMARY KEY (batch_id, feature_embedding_id)
);

-- One SYSTEM decision per pair and threshold version (related_reference, the match_thresholds id;
-- NULL for the built-in default), so a new version can re-decide a pair already decided. A pair's
-- latest SYSTEM row is its SYSTEM decision (cluster.DecidingRound reads the chain in decided_at,
-- id order). Human rows keep one row per pair, role and analysis.
ALTER TABLE biometric_decisions DROP CONSTRAINT biometric_decisions_pair_role_analysis_key;
CREATE UNIQUE INDEX biometric_decisions_pair_role_analysis_key ON biometric_decisions
    (feature_a_id, feature_b_id, role, case_analysis_id) NULLS NOT DISTINCT
    WHERE role <> 'SYSTEM';
CREATE UNIQUE INDEX biometric_decisions_pair_system_version_key ON biometric_decisions
    (feature_a_id, feature_b_id, related_reference) NULLS NOT DISTINCT
    WHERE role = 'SYSTEM';

-- +goose Down
DROP INDEX biometric_decisions_pair_system_version_key;
DROP INDEX biometric_decisions_pair_role_analysis_key;
ALTER TABLE biometric_decisions ADD CONSTRAINT biometric_decisions_pair_role_analysis_key
    UNIQUE NULLS NOT DISTINCT (feature_a_id, feature_b_id, role, case_analysis_id);
DROP TABLE bulk_match_hubs;
DROP TABLE bulk_match_scores;
DROP TABLE bulk_match_batch_members;
DROP TABLE bulk_match_batches;
DROP TABLE bulk_enroll_outcomes;
DROP INDEX feature_embeddings_unmatched_idx;
CREATE INDEX feature_embeddings_unmatched_idx ON feature_embeddings (embedding_type)
    WHERE matched_at IS NULL;
ALTER TABLE feature_embeddings DROP COLUMN match_path;
