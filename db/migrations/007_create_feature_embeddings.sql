-- +goose Up
-- feature_embeddings is a vector representation of a biometricfeature -- e.g. a face
-- embedding computed from a FACE_RECORD/FACE_CAPTURE feature's underlying photo. It is
-- keyed on biometricfeature.id rather than on identity_file/case_trace directly, which is
-- what makes it generic across KNOWN and QUESTIONED features: nothing here needs to know
-- which side of the enrollment/evidence hierarchy the vector came from.
--
-- embedding_type names the representation; vectors of different types live in different,
-- not-cosine-comparable spaces and are never compared:
--   FACE_ARCFACE_512  -- ArcFace
--   FACE_AURAFACE_512 -- legacy embeddings from an "auraface" model (see
--                        cmd/backfill-face-embeddings)
-- A biometricfeature has at most one row per embedding_type -- if a feature is re-embedded
-- (a model upgrade), the row is replaced, not duplicated.
--
-- matched_at marks when this embedding was last run through ANN matching (see the
-- biometricmatch package / cmd/match-embeddings), so unmatched embeddings can be found
-- without rescanning biometric_decisions.
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE feature_embeddings (
    id BIGSERIAL PRIMARY KEY,
    biometricfeature_id BIGINT NOT NULL REFERENCES biometricfeature(id) ON DELETE CASCADE,
    embedding_type TEXT NOT NULL CHECK (embedding_type IN ('FACE_ARCFACE_512', 'FACE_AURAFACE_512')),
    embedding VECTOR(512) NOT NULL,
    model_version TEXT,
    matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT feature_embeddings_feature_type_key UNIQUE (biometricfeature_id, embedding_type)
);

CREATE INDEX feature_embeddings_embedding_idx ON feature_embeddings
    USING hnsw (embedding vector_cosine_ops);
CREATE INDEX feature_embeddings_unmatched_idx ON feature_embeddings (embedding_type)
    WHERE matched_at IS NULL;

-- +goose Down
DROP TABLE feature_embeddings;
