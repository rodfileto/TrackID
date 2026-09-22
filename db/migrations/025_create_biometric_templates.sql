-- +goose Up
-- biometric_templates holds a matcher's own serialized template for a biometricfeature --
-- today a SourceAFIS fingerprint template, extracted by the sourceafis-sidecar service
-- (see the fingerprint package). It is the counterpart of feature_embeddings for
-- representations that are not a fixed-length vector: a template is an opaque blob that only
-- the matcher that wrote it can compare, so there is no pgvector column and no ANN index --
-- matching sends template bytes to the matcher (biometricmatch.RunTemplates).
--
-- Keyed like feature_embeddings: on biometricfeature.id, so KNOWN (identity_file) and
-- QUESTIONED (case_trace) features are handled alike, and at most one row per
-- (biometricfeature, template_type) -- re-extraction (a new image, a matcher upgrade)
-- replaces the row. template_type names the format; templates of different types are never
-- compared. matched_at marks when the template last went through matching, as in
-- feature_embeddings.
CREATE TABLE biometric_templates (
    id BIGSERIAL PRIMARY KEY,
    biometricfeature_id BIGINT NOT NULL REFERENCES biometricfeature(id) ON DELETE CASCADE,
    template_type TEXT NOT NULL CHECK (template_type IN ('FINGERPRINT_SOURCEAFIS')),
    template BYTEA NOT NULL,
    model_version TEXT,
    matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT biometric_templates_feature_type_key UNIQUE (biometricfeature_id, template_type)
);

CREATE INDEX biometric_templates_unmatched_idx ON biometric_templates (template_type)
    WHERE matched_at IS NULL;

-- +goose Down
DROP TABLE biometric_templates;
