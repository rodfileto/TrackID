-- +goose Up
-- The evidence (QUESTIONED) hierarchy: biometric_cases -< case_evidences -< case_traces -<
-- case_codifications -< case_codification_points, plus case_files (raw files attached to a
-- case) and case_decisions (the case's own workflow log). See MODEL.md section 2.2.
--
-- A biometric case is any case in which questioned biometric material has to be compared
-- against the known (enrollment) side. Two independent axes classify it:
--
-- case_type -- the legal nature of the case:
--   CRIMINAL -- a criminal investigation (e.g. latent prints or faces from a crime scene)
--   CIVIL    -- a non-criminal identification: disaster victim identification (DVI),
--               identification of unidentified dead bodies, missing persons, ...
--
-- modality -- the biometric the case works with, which fixes what the hierarchy means:
--   FACIAL      -- an evidence is a raw image, a trace is one detected face within it, and a
--                  codification is a processed form (e.g. an embedding)
--   FINGERPRINT -- an evidence is a lift card/image, a trace is one fingerprint lift on it,
--                  and a codification is a minutiae encoding of that lift
CREATE TABLE biometric_cases (
    id BIGSERIAL PRIMARY KEY,
    case_id TEXT NOT NULL UNIQUE,
    case_type TEXT NOT NULL CHECK (case_type IN ('CRIMINAL', 'CIVIL')),
    modality TEXT NOT NULL CHECK (modality IN ('FACIAL', 'FINGERPRINT')),
    description TEXT NOT NULL,
    -- Cases created through the API get a TrackID-generated number "<year>.<sequence>"
    -- (e.g. 2026.0001). The numbering logic lives in Go (the cases package reads the current
    -- max per year and increments); the unique index below is the only database-level
    -- enforcement against duplicates. Cases imported by an organization's own import command
    -- keep their own case_id and leave both NULL.
    case_year INTEGER,
    case_number INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX biometric_cases_year_number_key ON biometric_cases (case_year, case_number);
CREATE INDEX biometric_cases_case_type_idx ON biometric_cases (case_type);
CREATE INDEX biometric_cases_description_idx ON biometric_cases USING GIN (to_tsvector('simple', description));

-- Raw files attached to a biometric case:
--   evidence           -- an evidence photo/scan (one case_evidences row points at it)
--   documento          -- a supporting document
--   forensic_report    -- a forensic report
--   face_crop          -- the region a detector cropped from an evidence item for one trace
--                         (case_traces.case_file_id points at it)
--   codification_image -- the rendered result of manually adjusting a trace's codification
--                         image (crop, brightness/contrast, interpolation -- see
--                         CodificationEditorModal); case_codifications.case_file_id points at it
CREATE TABLE case_files (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    category TEXT NOT NULL CHECK (category IN ('evidence', 'documento', 'forensic_report', 'face_crop', 'codification_image')),
    media_type TEXT,
    hash_id TEXT,
    filename TEXT,
    source_path TEXT,
    storage_ref TEXT,
    content_type TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX case_files_biometric_case_id_idx ON case_files (biometric_case_id);
CREATE UNIQUE INDEX case_files_case_category_hash_key ON case_files (biometric_case_id, category, hash_id);

-- case_file_id cascades: deleting an evidence's case_files row (cases.DeleteEvidence, which
-- refuses when the evidence still has traces) must not leave an orphaned evidence behind.
CREATE TABLE case_evidences (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE CASCADE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_evidences_case_sequence_key UNIQUE (biometric_case_id, sequence)
);
CREATE INDEX case_evidences_biometric_case_id_idx ON case_evidences (biometric_case_id);

CREATE TABLE case_traces (
    id BIGSERIAL PRIMARY KEY,
    evidence_id BIGINT NOT NULL REFERENCES case_evidences(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    trace_type TEXT NOT NULL CHECK (trace_type IN ('FACE_RECORD', 'FINGERPRINT_LIFT')),
    box_x1 DOUBLE PRECISION,
    box_y1 DOUBLE PRECISION,
    box_x2 DOUBLE PRECISION,
    box_y2 DOUBLE PRECISION,
    detection_score DOUBLE PRECISION,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_traces_evidence_sequence_key UNIQUE (evidence_id, sequence)
);
CREATE INDEX case_traces_evidence_id_idx ON case_traces (evidence_id);

-- A processed encoding of a trace (e.g. a minutiae set, an embedding). The pairwise
-- comparison outcome lives in biometric_decisions, not here. case_file_id points at the
-- codification_image, NULL until one is saved.
CREATE TABLE case_codifications (
    id BIGSERIAL PRIMARY KEY,
    trace_id BIGINT NOT NULL REFERENCES case_traces(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    codification_type TEXT NOT NULL,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_codifications_trace_sequence_key UNIQUE (trace_id, sequence)
);
CREATE INDEX case_codifications_trace_id_idx ON case_codifications (trace_id);

-- Manual minutiae marking for a codification: one point (e.g. a ridge ending or bifurcation)
-- per row, in pixel coordinates against the trace's evidence image. point_type/angle are
-- free-form and optional -- forensic minutiae conventions vary by tool/organization.
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

-- Reconstructs the "case_id-NNN-NN-NN" fragment id for a codification row.
CREATE VIEW case_fragment_codes AS
SELECT
    cod.id AS codification_id,
    tr.id AS trace_id,
    ev.id AS evidence_id,
    bc.id AS biometric_case_id,
    bc.case_id
        || '-' || lpad(ev.sequence::text, 3, '0')
        || '-' || lpad(tr.sequence::text, 2, '0')
        || '-' || lpad(cod.sequence::text, 2, '0') AS fragment_id
FROM case_codifications cod
JOIN case_traces tr ON tr.id = cod.trace_id
JOIN case_evidences ev ON ev.id = tr.evidence_id
JOIN biometric_cases bc ON bc.id = ev.biometric_case_id;

-- case_decisions is an append-only log of decisions/status events about a case itself (e.g. a
-- case being approved, rejected, or otherwise progressed through an organization's own
-- workflow) -- distinct from biometric_decisions, which is about a pair of biometric features.
-- decision is free-form: its vocabulary is organization-defined.
--
-- Mirrors biometric_decisions' decider pattern: exactly one of system_source/username
-- identifies who or what recorded the decision.
CREATE TABLE case_decisions (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    decision TEXT NOT NULL,
    system_source TEXT,
    username TEXT,
    CONSTRAINT case_decisions_decider_check CHECK (
        (system_source IS NOT NULL AND username IS NULL)
        OR
        (system_source IS NULL AND username IS NOT NULL)
    ),
    notes TEXT,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX case_decisions_biometric_case_id_idx ON case_decisions (biometric_case_id, decided_at);

-- +goose Down
DROP TABLE case_decisions;
DROP VIEW case_fragment_codes;
DROP TABLE case_codification_points;
DROP TABLE case_codifications;
DROP TABLE case_traces;
DROP TABLE case_evidences;
DROP TABLE case_files;
DROP TABLE biometric_cases;
