-- +goose Up
-- The evidence (QUESTIONED) hierarchy: biometric_cases -< case_evidences -< case_traces -<
-- case_codifications -< case_codification_points, plus case_files (raw files attached to a case),
-- case_videos (video sent in) and case_request_cases (the requests a case answers). See MODEL.md
-- section 2.2. A case is linked to the occurrence it investigates by occurrence_id.
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
    -- NULL until the case is linked to its occurrence (and for a case with no report at all).
    occurrence_id BIGINT REFERENCES occurrences(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX biometric_cases_year_number_key ON biometric_cases (case_year, case_number);
CREATE INDEX biometric_cases_case_type_idx ON biometric_cases (case_type);
CREATE INDEX biometric_cases_description_idx ON biometric_cases USING GIN (to_tsvector('simple', description));
CREATE INDEX biometric_cases_occurrence_id_idx ON biometric_cases (occurrence_id);

-- The requests a case answers. A request keeps its number per case here (sequence), so a case
-- lists its requests 1, 2, ... whichever other cases the same request opened or joined.
CREATE TABLE case_request_cases (
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (case_request_id, biometric_case_id),
    CONSTRAINT case_request_cases_case_sequence_key UNIQUE (biometric_case_id, sequence)
);

-- Raw files attached to a biometric case:
--   evidence           -- an evidence photo/scan (one case_evidences row points at it)
--   video              -- a video sent in (one case_videos row points at it)
--   document           -- a supporting document
--   forensic_report    -- an issued report's PDF
--   report_draft       -- a version of a report's draft (.docx; analysis_report_versions)
--   face_crop          -- the region a detector cropped from an evidence item for one trace
--                         (case_traces.case_file_id points at it)
--   codification_image -- a codification's own image: a version rendered with a recipe, or an
--                         adjusted image saved for it; case_codifications.case_file_id points at it
-- hash_id is the file's SHA-256. case_request_id is the request that sent the file in, when one
-- did: the material's receipt.
CREATE TABLE case_files (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    media_type TEXT,
    hash_id TEXT,
    filename TEXT,
    source_path TEXT,
    storage_ref TEXT,
    content_type TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    case_request_id BIGINT REFERENCES case_requests(id) ON DELETE SET NULL,
    CONSTRAINT case_files_category_check
        CHECK (category IN ('evidence', 'video', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image'))
);

CREATE INDEX case_files_biometric_case_id_idx ON case_files (biometric_case_id);
CREATE UNIQUE INDEX case_files_case_category_hash_key ON case_files (biometric_case_id, category, hash_id);
CREATE INDEX case_files_case_request_id_idx ON case_files (case_request_id);

-- A video sent in as material (docs/facial-reports-plan.md, decision 14): its processing state
-- and, once processed, the persons trackid-vision's videoproc followed through it (tracks: per
-- person the best face's time, box, blur score and crop, every face found, and their span). The
-- examiner captures frames from it, each becoming an evidence.
CREATE TABLE case_videos (
    case_file_id BIGINT PRIMARY KEY REFERENCES case_files(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSING', 'DONE', 'FAILED')),
    duration_seconds DOUBLE PRECISION,
    frame_rate DOUBLE PRECISION,
    tracks JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

-- case_file_id cascades: deleting an evidence's case_files row (cases.DeleteEvidence, which
-- refuses when the evidence still has traces) must not leave an orphaned evidence behind.
--
-- An evidence captured from a video records the video, the time of the frame and its frame number
-- (source_video_file_id, source_timestamp_ms, source_frame).
CREATE TABLE case_evidences (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE CASCADE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_video_file_id BIGINT REFERENCES case_files(id),
    source_timestamp_ms BIGINT,
    source_frame BIGINT,
    CONSTRAINT case_evidences_case_sequence_key UNIQUE (biometric_case_id, sequence),
    CONSTRAINT case_evidences_source_check CHECK (
        (source_video_file_id IS NULL AND source_timestamp_ms IS NULL AND source_frame IS NULL)
        OR (source_video_file_id IS NOT NULL AND source_timestamp_ms IS NOT NULL))
);
CREATE INDEX case_evidences_biometric_case_id_idx ON case_evidences (biometric_case_id);

-- trace_type is the feature type of the QUESTIONED biometric feature the trace yields:
-- FACE_CAPTURE (one face detected or marked in an image) or FINGERPRINT_LIFT (one lift on a card).
--
-- trace_role: QUESTIONED is material to identify; REFERENCE is known material sent in for
-- comparison (e.g. a suspect's photo). A reference is never an unidentified face of the case.
--
-- suitability is the examiner's value determination of a QUESTIONED trace (FISWG analysis phase):
--   SUITABLE   -- the image supports a forensic comparison
--   LEAD_ONLY  -- good enough for an investigative lead, not for a comparison
--   UNSUITABLE -- of no value
-- NULL until assessed; set together with who assessed it and when.
--
-- recognition_codification_id is the codification whose embedding the matcher and clustering use
-- for this face (see case_codifications); NULL means the original (sequence 1).
CREATE TABLE case_traces (
    id BIGSERIAL PRIMARY KEY,
    evidence_id BIGINT NOT NULL REFERENCES case_evidences(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    trace_type TEXT NOT NULL CHECK (trace_type IN ('FACE_CAPTURE', 'FINGERPRINT_LIFT')),
    box_x1 DOUBLE PRECISION,
    box_y1 DOUBLE PRECISION,
    box_x2 DOUBLE PRECISION,
    box_y2 DOUBLE PRECISION,
    detection_score DOUBLE PRECISION,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    trace_role TEXT NOT NULL DEFAULT 'QUESTIONED' CHECK (trace_role IN ('QUESTIONED', 'REFERENCE')),
    suitability TEXT CHECK (suitability IN ('SUITABLE', 'LEAD_ONLY', 'UNSUITABLE')),
    suitability_notes TEXT,
    suitability_by BIGINT REFERENCES users(id),
    suitability_at TIMESTAMPTZ,
    recognition_codification_id BIGINT,
    CONSTRAINT case_traces_evidence_sequence_key UNIQUE (evidence_id, sequence),
    CONSTRAINT case_traces_suitability_assessed_check CHECK (
        (suitability IS NULL AND suitability_by IS NULL AND suitability_at IS NULL)
        OR (suitability IS NOT NULL AND suitability_by IS NOT NULL AND suitability_at IS NOT NULL)),
    CONSTRAINT case_traces_reference_suitability_check CHECK (trace_role = 'QUESTIONED' OR suitability IS NULL)
);
CREATE INDEX case_traces_evidence_id_idx ON case_traces (evidence_id);

-- A processed encoding of a trace (e.g. a minutiae set, an embedding). The pairwise comparison
-- outcome lives in biometric_decisions, not here. case_file_id points at the codification's own
-- image, NULL until there is one.
--
-- A face can have several codifications: the original (sequence 1) and versions the examiner
-- renders from it (mirrored, rotated, at another scale, brighter, in grey...) to see how a
-- recognition system takes the face. recipe is how a version was rendered from the evidence image
-- (imageproc.FaceRecipe), so the record says exactly what was done and the image can be rebuilt;
-- NULL for the original. Each codification has its own embedding (codification_embeddings); the
-- matcher uses the one case_traces.recognition_codification_id names.
CREATE TABLE case_codifications (
    id BIGSERIAL PRIMARY KEY,
    trace_id BIGINT NOT NULL REFERENCES case_traces(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    codification_type TEXT NOT NULL,
    case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    label TEXT,
    recipe JSONB,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT case_codifications_trace_sequence_key UNIQUE (trace_id, sequence)
);
CREATE INDEX case_codifications_trace_id_idx ON case_codifications (trace_id);

ALTER TABLE case_traces ADD CONSTRAINT case_traces_recognition_codification_id_fkey
    FOREIGN KEY (recognition_codification_id) REFERENCES case_codifications(id) ON DELETE SET NULL;

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

-- +goose Down
DROP TABLE case_codification_points;
ALTER TABLE case_traces DROP CONSTRAINT case_traces_recognition_codification_id_fkey;
DROP TABLE case_codifications;
DROP TABLE case_traces;
DROP TABLE case_evidences;
DROP TABLE case_videos;
DROP TABLE case_files;
DROP TABLE case_request_cases;
DROP TABLE biometric_cases;
