-- +goose Up
-- biometricfeature is the atomic thing the whole system compares: one typed biometric
-- sample, tagged with its provenance. It is sourced from exactly one place -- an
-- identity_file (KNOWN, from enrollment) or a case_trace (QUESTIONED, from a biometric
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
-- A NIST identity_file yields one FINGERPRINT_TEMPLATE; a photo yields one FACE_RECORD.
-- The two unique constraints guarantee at most one feature per source, so the graph
-- feature id stays derivable from the source (bare identity_file.id, or
-- "TRACE:<case_trace_id>#feature") without a round-trip.
CREATE TABLE biometricfeature (
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

CREATE INDEX biometricfeature_feature_type_idx ON biometricfeature (feature_type);
CREATE INDEX biometricfeature_provenance_idx ON biometricfeature (provenance);

-- +goose Down
DROP TABLE biometricfeature;
