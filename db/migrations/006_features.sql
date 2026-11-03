-- +goose Up
-- biometric_features holds the atomic thing the whole system compares, one row per
-- typed biometric sample, tagged with its provenance. It is sourced from exactly one place -- an
-- identity_files row (KNOWN, from enrollment) or a case_traces row (QUESTIONED, from a biometric
-- case's evidence) -- enforced by the exactly-one-source CHECK below. provenance is derivable
-- from that source but stored explicitly so a consumer never has to join to know which
-- side of the model a feature came from. See MODEL.md section 1.
--
-- feature_type:
--   FINGERPRINT_TEMPLATE -- an enrolled ten-print, KNOWN
--   FACE_RECORD          -- an enrolled face photo, KNOWN
--   FINGERPRINT_LIFT     -- a latent/questioned print from a case, QUESTIONED
--   FACE_CAPTURE         -- a face detected in case evidence, QUESTIONED
--
-- A NIST identity file yields one FINGERPRINT_TEMPLATE; a photo yields one FACE_RECORD.
-- The two unique constraints guarantee at most one feature per source, so the graph
-- feature id stays derivable from the source (bare identity_files.id, or
-- "TRACE:<case_trace_id>#feature") without a round-trip.
CREATE TABLE biometric_features (
    id BIGSERIAL PRIMARY KEY,
    feature_type TEXT NOT NULL CHECK (feature_type IN ('FINGERPRINT_TEMPLATE', 'FACE_RECORD', 'FINGERPRINT_LIFT', 'FACE_CAPTURE')),
    provenance TEXT NOT NULL CHECK (provenance IN ('KNOWN', 'QUESTIONED')),
    identity_file_id BIGINT REFERENCES identity_files(id) ON DELETE CASCADE,
    case_trace_id BIGINT REFERENCES case_traces(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT biometric_features_exactly_one_source_check CHECK (
        (identity_file_id IS NOT NULL)::int + (case_trace_id IS NOT NULL)::int = 1
    ),
    CONSTRAINT biometric_features_identity_file_id_key UNIQUE (identity_file_id),
    CONSTRAINT biometric_features_case_trace_id_key UNIQUE (case_trace_id)
);

CREATE INDEX biometric_features_feature_type_idx ON biometric_features (feature_type);
CREATE INDEX biometric_features_provenance_idx ON biometric_features (provenance);

-- feature_embeddings is a vector representation of a biometric feature -- e.g. a face
-- embedding computed from a FACE_RECORD/FACE_CAPTURE feature's underlying photo. It is
-- keyed on biometric_features.id rather than on identity_files/case_traces directly, which is
-- what makes it generic across KNOWN and QUESTIONED features: nothing here needs to know
-- which side of the enrollment/evidence hierarchy the vector came from.
--
-- embedding_type names the representation; vectors of different types live in different,
-- not-cosine-comparable spaces and are never compared:
--   FACE_AURAFACE_512 -- AuraFace-v1, the recognizer trackid-vision runs: what the face pipeline
--                        writes (embedding.EmbeddingType)
--   FACE_ARCFACE_512  -- InsightFace ArcFace; the pipeline does not write it, but embeddings
--                        loaded by other means can be stored and matched apart
-- A biometric feature has at most one row per embedding_type -- if a feature is re-embedded
-- (a model upgrade), the row is replaced, not duplicated.
--
-- matched_at marks when this embedding was last run through ANN matching (see the
-- biometricmatch package / cmd/match-embeddings), so unmatched embeddings can be found
-- without rescanning biometric_decisions.
CREATE TABLE feature_embeddings (
    id BIGSERIAL PRIMARY KEY,
    biometric_feature_id BIGINT NOT NULL REFERENCES biometric_features(id) ON DELETE CASCADE,
    embedding_type TEXT NOT NULL CHECK (embedding_type IN ('FACE_ARCFACE_512', 'FACE_AURAFACE_512')),
    embedding VECTOR(512) NOT NULL,
    model_version TEXT,
    matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT feature_embeddings_feature_type_key UNIQUE (biometric_feature_id, embedding_type)
);

CREATE INDEX feature_embeddings_embedding_idx ON feature_embeddings
    USING hnsw (embedding vector_cosine_ops);
CREATE INDEX feature_embeddings_unmatched_idx ON feature_embeddings (embedding_type)
    WHERE matched_at IS NULL;

-- codification_embeddings holds one embedding per codification of a face (case_codifications):
-- the original's and each rendered version's. The matcher never reads it; the face's embedding in
-- feature_embeddings is a copy of the one of the codification chosen for recognition
-- (case_traces.recognition_codification_id, the original while none is chosen).
CREATE TABLE codification_embeddings (
    id BIGSERIAL PRIMARY KEY,
    codification_id BIGINT NOT NULL REFERENCES case_codifications(id) ON DELETE CASCADE,
    embedding_type TEXT NOT NULL,
    embedding vector(512) NOT NULL,
    model_version TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT codification_embeddings_codification_type_key UNIQUE (codification_id, embedding_type)
);

-- biometric_templates holds a matcher's own serialized template for a biometric feature --
-- today a SourceAFIS fingerprint template, extracted by the sourceafis-sidecar service
-- (see the fingerprint package). It is the counterpart of feature_embeddings for
-- representations that are not a fixed-length vector: a template is an opaque blob that only
-- the matcher that wrote it can compare, so there is no pgvector column and no ANN index --
-- matching sends template bytes to the matcher (biometricmatch.RunTemplates).
--
-- Keyed like feature_embeddings: on biometric_features.id, so KNOWN (identity_files) and
-- QUESTIONED (case_traces) features are handled alike, and at most one row per
-- (biometric feature, template_type) -- re-extraction (a new image, a matcher upgrade)
-- replaces the row. template_type names the format; templates of different types are never
-- compared. matched_at marks when the template last went through matching, as in
-- feature_embeddings.
CREATE TABLE biometric_templates (
    id BIGSERIAL PRIMARY KEY,
    biometric_feature_id BIGINT NOT NULL REFERENCES biometric_features(id) ON DELETE CASCADE,
    template_type TEXT NOT NULL CHECK (template_type IN ('FINGERPRINT_SOURCEAFIS')),
    template BYTEA NOT NULL,
    model_version TEXT,
    matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT biometric_templates_feature_type_key UNIQUE (biometric_feature_id, template_type)
);

CREATE INDEX biometric_templates_unmatched_idx ON biometric_templates (template_type)
    WHERE matched_at IS NULL;

-- match_thresholds is the versioned configuration of the automatic matcher's cutoffs, one
-- append-only row per version; the current version of a representation is its latest row.
-- embedding_type names the representation the cutoffs apply to: a feature_embeddings
-- embedding_type or a biometric_templates template_type (e.g. FINGERPRINT_SOURCEAFIS).
-- Operational thresholds belong to each deploying organization's own validation (a ROC
-- analysis on its labelled data, say), so they live here as data with their provenance rather
-- than as a constant in code, and changing one takes effect on the next matching run without a
-- restart.
--
-- Both cutoffs are on the same scale as biometric_decisions.confidence: for a face embedding a
-- similarity, 1 - cosine distance, in (0, 1]; for a SourceAFIS template its score, unbounded
-- above, with 40 as its documented threshold. So the check keeps only what holds on every scale
-- -- a positive review cutoff at or below the confirm cutoff -- and the upper bound of a
-- similarity is checked in code (biometricmatch.Band.ValidateSimilarity). A pair at or above confirm_threshold gets a SYSTEM
-- POSITIVE, which clustering treats as CONFIRMED; a pair at or above review_threshold but
-- below confirm_threshold gets a SYSTEM INCONCLUSIVE, which is PENDING_REVIEW until an examiner
-- decides it; below review_threshold nothing is written. review_threshold = confirm_threshold
-- means no review band. Each decision cites the version it was classified under
-- (related_reference_kind 'match_threshold', related_reference the row id).
--
-- A new version applies to matching from then on; decisions already written stay, since
-- biometric_decisions is an append-only log.
CREATE TABLE match_thresholds (
    id BIGSERIAL PRIMARY KEY,
    embedding_type TEXT NOT NULL,
    review_threshold DOUBLE PRECISION NOT NULL,
    confirm_threshold DOUBLE PRECISION NOT NULL,
    source TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT match_thresholds_band_check CHECK (
        review_threshold > 0 AND review_threshold <= confirm_threshold
    )
);

CREATE INDEX match_thresholds_current_idx ON match_thresholds (embedding_type, id DESC);

-- +goose Down
DROP TABLE match_thresholds;
DROP TABLE biometric_templates;
DROP TABLE codification_embeddings;
DROP TABLE feature_embeddings;
DROP TABLE biometric_features;
