-- +goose Up
-- The evidence (QUESTIONED) hierarchy: criminal_cases -< case_evidences -< case_traces -<
-- case_codifications, plus case_files (raw files attached to a case). Shared by both
-- case_type values: for FACIAL, an evidence is a raw image, a trace is one detected face
-- within it, and a codification is a processed form (e.g. an embedding); for FINGERPRINT,
-- an evidence is a lift card/image, a trace is one fingerprint lift on it, and a
-- codification is a minutiae encoding of that lift. See MODEL.md section 2.2.
CREATE TABLE criminal_cases (
    id BIGSERIAL PRIMARY KEY,
    case_id TEXT NOT NULL UNIQUE,
    case_type TEXT NOT NULL CHECK (case_type IN ('FACIAL', 'FINGERPRINT')),
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX criminal_cases_description_idx ON criminal_cases USING GIN (to_tsvector('simple', description));

-- Raw files attached to a criminal case (evidence photos, documents, forensic reports).
CREATE TABLE case_files (
    id BIGSERIAL PRIMARY KEY,
    criminal_case_id BIGINT NOT NULL REFERENCES criminal_cases(id) ON DELETE CASCADE,
    category TEXT NOT NULL CHECK (category IN ('evidence', 'documento', 'forensic_report')),
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

CREATE INDEX case_files_criminal_case_id_idx ON case_files (criminal_case_id);
CREATE UNIQUE INDEX case_files_case_category_hash_key ON case_files (criminal_case_id, category, hash_id);

CREATE TABLE case_evidences (
    id BIGSERIAL PRIMARY KEY,
    criminal_case_id BIGINT NOT NULL REFERENCES criminal_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_evidences_case_sequence_key UNIQUE (criminal_case_id, sequence)
);
CREATE INDEX case_evidences_criminal_case_id_idx ON case_evidences (criminal_case_id);

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
-- comparison outcome lives in biometric_decisions, not here.
CREATE TABLE case_codifications (
    id BIGSERIAL PRIMARY KEY,
    trace_id BIGINT NOT NULL REFERENCES case_traces(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    codification_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_codifications_trace_sequence_key UNIQUE (trace_id, sequence)
);
CREATE INDEX case_codifications_trace_id_idx ON case_codifications (trace_id);

-- Reconstructs the "case_id-NNN-NN-NN" fragment id for a codification row.
CREATE VIEW case_fragment_codes AS
SELECT
    cod.id AS codification_id,
    tr.id AS trace_id,
    ev.id AS evidence_id,
    cc.id AS criminal_case_id,
    cc.case_id
        || '-' || lpad(ev.sequence::text, 3, '0')
        || '-' || lpad(tr.sequence::text, 2, '0')
        || '-' || lpad(cod.sequence::text, 2, '0') AS fragment_id
FROM case_codifications cod
JOIN case_traces tr ON tr.id = cod.trace_id
JOIN case_evidences ev ON ev.id = tr.evidence_id
JOIN criminal_cases cc ON cc.id = ev.criminal_case_id;
