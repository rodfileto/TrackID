-- +goose Up
-- Where material comes from (docs/scene-collection-plan.md). Material is either received, sent in by
-- a request (case_files.case_request_id, as before), or collected at a scene by an examiner who
-- went there on call (case_collections), before any request formalized it. A vestige is a physical
-- item, received or collected, with the seals it was kept under.

-- Whether the case's analyses are reported in one laudo or each in its own, chosen by whoever
-- opens the case. Recorded now; one laudo for several analyses comes with the scene laudo.
ALTER TABLE biometric_cases ADD COLUMN report_mode TEXT NOT NULL DEFAULT 'SEPARATE'
    CHECK (report_mode IN ('SEPARATE', 'TOGETHER'));

-- A scene examined on call. The scene is the case's occurrence's place, unless location_id names
-- another (a body found far from where the person was last seen). collected_by is the examiner who
-- collected, who may not be who registered it (registered_by, a coordinator). nothing_collected says
-- the scene was examined and gave nothing: such a scene is registered too, its laudo saying so.
-- case_request_id is the request that formalized the examination, NULL while it is awaited.
CREATE TABLE case_collections (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    location_id BIGINT REFERENCES locations(id),
    collected_from TIMESTAMPTZ NOT NULL,
    collected_to TIMESTAMPTZ,
    collected_by BIGINT NOT NULL REFERENCES users(id),
    nothing_collected BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT,
    case_request_id BIGINT REFERENCES case_requests(id) ON DELETE SET NULL,
    registered_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_collections_case_sequence_key UNIQUE (biometric_case_id, sequence),
    CONSTRAINT case_collections_period_check CHECK (collected_to IS NULL OR collected_to >= collected_from)
);
CREATE INDEX case_collections_awaiting_idx ON case_collections (created_at) WHERE case_request_id IS NULL;
CREATE INDEX case_collections_collected_by_idx ON case_collections (collected_by);

-- A physical item of the case (a lift card, an envelope of lifts, a seized object, a pen drive),
-- numbered V1, V2, ... in the case, with exactly one origin: the request that sent it in or the
-- collection that gathered it. Deleting that origin is refused rather than leaving the item without one.
CREATE TABLE case_vestiges (
    id BIGSERIAL PRIMARY KEY,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    description TEXT NOT NULL CHECK (description <> ''),
    case_request_id BIGINT REFERENCES case_requests(id) ON DELETE RESTRICT,
    collection_id BIGINT REFERENCES case_collections(id) ON DELETE RESTRICT,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_vestiges_case_sequence_key UNIQUE (biometric_case_id, sequence),
    CONSTRAINT case_vestiges_one_origin_check CHECK ((case_request_id IS NULL) <> (collection_id IS NULL))
);
CREATE INDEX case_vestiges_case_request_id_idx ON case_vestiges (case_request_id);
CREATE INDEX case_vestiges_collection_id_idx ON case_vestiges (collection_id);

-- The seals a vestige was kept under, in order. The lab breaks one to examine and seals again with a
-- new number, so at most one is unbroken. applied_by is NULL for a seal applied outside (the agency
-- that sent the item). Seal numbers are not unique: numbering differs between issuers.
CREATE TABLE case_vestige_seals (
    id BIGSERIAL PRIMARY KEY,
    vestige_id BIGINT NOT NULL REFERENCES case_vestiges(id) ON DELETE CASCADE,
    seal_number TEXT NOT NULL CHECK (seal_number <> ''),
    applied_at TIMESTAMPTZ NOT NULL,
    applied_by BIGINT REFERENCES users(id),
    broken_at TIMESTAMPTZ,
    broken_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT case_vestige_seals_broken_check CHECK ((broken_at IS NULL) = (broken_by IS NULL)),
    CONSTRAINT case_vestige_seals_order_check CHECK (broken_at IS NULL OR broken_at >= applied_at)
);
CREATE UNIQUE INDEX case_vestige_seals_one_unbroken ON case_vestige_seals (vestige_id) WHERE broken_at IS NULL;
CREATE INDEX case_vestige_seals_number_idx ON case_vestige_seals (seal_number);

-- The vestiges a request's questions are about, when it did not send them (a request formalizing a
-- scene asks about what was collected there).
CREATE TABLE case_request_vestiges (
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    vestige_id BIGINT NOT NULL REFERENCES case_vestiges(id) ON DELETE CASCADE,
    PRIMARY KEY (case_request_id, vestige_id)
);
CREATE INDEX case_request_vestiges_vestige_id_idx ON case_request_vestiges (vestige_id);

-- A file's origin: the request that sent it in or the collection that gathered it, never both;
-- neither for a file derived here (a crop, a frame, a report) or one whose origin was never
-- recorded (imports, files from before this migration). vestige_id is the item it was taken from.
ALTER TABLE case_files ADD COLUMN collection_id BIGINT REFERENCES case_collections(id) ON DELETE RESTRICT;
ALTER TABLE case_files ADD COLUMN vestige_id BIGINT REFERENCES case_vestiges(id) ON DELETE SET NULL;
ALTER TABLE case_files ADD CONSTRAINT case_files_one_origin_check
    CHECK (case_request_id IS NULL OR collection_id IS NULL);
CREATE INDEX case_files_collection_id_idx ON case_files (collection_id);
CREATE INDEX case_files_vestige_id_idx ON case_files (vestige_id);

-- +goose Down
ALTER TABLE case_files DROP CONSTRAINT case_files_one_origin_check;
ALTER TABLE case_files DROP COLUMN vestige_id;
ALTER TABLE case_files DROP COLUMN collection_id;
DROP TABLE case_request_vestiges;
DROP TABLE case_vestige_seals;
DROP TABLE case_vestiges;
DROP TABLE case_collections;
ALTER TABLE biometric_cases DROP COLUMN report_mode;
