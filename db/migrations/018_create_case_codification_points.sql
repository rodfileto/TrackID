-- +goose Up
-- Manual minutiae marking for a trace's codification: one point (e.g. a ridge ending or
-- bifurcation, for a FINGERPRINT_LIFT trace's MINUTIAE codification) per row, in pixel
-- coordinates against the trace's evidence image. point_type/angle are free-form and
-- optional -- forensic minutiae conventions vary by tool/organization and aren't
-- enforced here, unlike case_traces.trace_type's fixed vocabulary.
CREATE TABLE case_codification_points (
    id BIGSERIAL PRIMARY KEY,
    codification_id BIGINT NOT NULL REFERENCES case_codifications(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    x DOUBLE PRECISION NOT NULL,
    y DOUBLE PRECISION NOT NULL,
    point_type TEXT,
    angle DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_codification_points_codification_sequence_key UNIQUE (codification_id, sequence)
);
CREATE INDEX case_codification_points_codification_id_idx ON case_codification_points (codification_id);

-- +goose Down
DROP TABLE case_codification_points;
