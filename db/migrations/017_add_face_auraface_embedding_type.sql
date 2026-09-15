-- +goose Up
-- Legacy face embeddings (pre-refactor face_embeddings table) were produced by an
-- "auraface" model, not ArcFace -- a different, not-cosine-comparable vector space, so
-- they need their own embedding_type rather than being mislabeled FACE_ARCFACE_512.
-- See cmd/backfill-face-embeddings, which populates feature_embeddings/biometricfeature
-- from that legacy table.
ALTER TABLE feature_embeddings DROP CONSTRAINT feature_embeddings_embedding_type_check;
ALTER TABLE feature_embeddings ADD CONSTRAINT feature_embeddings_embedding_type_check
    CHECK (embedding_type IN ('FACE_ARCFACE_512', 'FACE_AURAFACE_512'));

-- +goose Down
ALTER TABLE feature_embeddings DROP CONSTRAINT feature_embeddings_embedding_type_check;
ALTER TABLE feature_embeddings ADD CONSTRAINT feature_embeddings_embedding_type_check
    CHECK (embedding_type IN ('FACE_ARCFACE_512'));
