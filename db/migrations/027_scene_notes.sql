-- +goose Up
-- The scene, beyond a collection's vestiges (docs/scene-examination-plan.md).

-- Notes an examiner writes at the scene, on a phone that may have no signal. They are kept on the
-- device until it syncs, then here, still a draft: a note is its author's, not a case, and holds
-- no vestige. It becomes a scene collection through the collection form, which sets collection_id.
-- client_id is made on the device, so a note synced twice is one row.
CREATE TABLE scene_notes (
    id BIGSERIAL PRIMARY KEY,
    client_id UUID NOT NULL UNIQUE,
    author_id BIGINT NOT NULL REFERENCES users(id),
    place_type TEXT CHECK (place_type IN ('PUBLIC_ROAD', 'RESIDENCE', 'COMMERCE', 'BANK',
        'GOVERNMENT_OFFICE', 'TRANSPORT', 'RURAL', 'OTHER')),
    place_name TEXT,
    -- The device's position, or one typed; both or neither.
    lon DOUBLE PRECISION CHECK (lon BETWEEN -180 AND 180),
    lat DOUBLE PRECISION CHECK (lat BETWEEN -90 AND 90),
    address TEXT,
    neighbourhood TEXT,
    -- Set when the address was found from the point at sync (geocode.Result's Source and Flags).
    geocode_source TEXT,
    geocode_flags TEXT[] NOT NULL DEFAULT '{}',
    called_at TIMESTAMPTZ,   -- when the team was called out
    examined_at TIMESTAMPTZ, -- when the scene was examined
    occurred_at TIMESTAMPTZ, -- when what is examined happened, as told at the scene
    team BIGINT[] NOT NULL DEFAULT '{}', -- users at the scene besides the author
    other_officers TEXT,
    notes TEXT,
    written_at TIMESTAMPTZ NOT NULL, -- on the device
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collection_id BIGINT REFERENCES case_collections(id) ON DELETE SET NULL,
    CHECK ((lon IS NULL) = (lat IS NULL))
);
CREATE INDEX scene_notes_author_idx ON scene_notes (author_id, written_at DESC);

-- Audio recorded with a note. sha256 is the file's; storage_ref where it is kept.
CREATE TABLE scene_note_audios (
    id BIGSERIAL PRIMARY KEY,
    scene_note_id BIGINT NOT NULL REFERENCES scene_notes(id) ON DELETE CASCADE,
    client_id UUID NOT NULL UNIQUE,
    title TEXT,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256 TEXT NOT NULL,
    storage_ref TEXT NOT NULL,
    recorded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX scene_note_audios_note_idx ON scene_note_audios (scene_note_id);

-- A collection says when the team was called out, who else was there, and its team.
ALTER TABLE case_collections
    ADD COLUMN called_at TIMESTAMPTZ,
    ADD COLUMN other_officers TEXT;

-- Users at the scene besides the collector.
CREATE TABLE case_collection_members (
    collection_id BIGINT NOT NULL REFERENCES case_collections(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    PRIMARY KEY (collection_id, user_id)
);

-- An examiner's audio notes of a scene are files of the case with the collection as origin: their
-- record of it, not evidence, so they have no vestige.
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'video', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image',
        'collection_note'));

-- +goose Down
DELETE FROM case_files WHERE category = 'collection_note';
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'video', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image'));
DROP TABLE case_collection_members;
ALTER TABLE case_collections DROP COLUMN other_officers, DROP COLUMN called_at;
DROP TABLE scene_note_audios;
DROP TABLE scene_notes;
