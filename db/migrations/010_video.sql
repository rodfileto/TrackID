-- +goose Up
-- Video sent in as material (docs/facial-reports-plan.md, decision 14). The video itself is a
-- case file of category 'video', with its SHA-256, and can be attached to the request that sent
-- it. trackid-vision's videoproc samples it, detects and embeds the faces and groups them into one
-- track per person (case_videos.tracks); the examiner picks the frames to examine, and each pick
-- becomes an evidence -- that full frame, extracted at its time -- recording where it came from.
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'video', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image'));

-- One row per video: its processing state and, once processed, its tracks as videoproc found them
-- (per track: the best face's time, box, blur score and a small crop, and how many detections).
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

-- An evidence extracted from a video: the video, the time of the frame and its frame number.
ALTER TABLE case_evidences
    ADD COLUMN source_video_file_id BIGINT REFERENCES case_files(id),
    ADD COLUMN source_timestamp_ms BIGINT,
    ADD COLUMN source_frame BIGINT,
    ADD CONSTRAINT case_evidences_source_check CHECK (
        (source_video_file_id IS NULL AND source_timestamp_ms IS NULL AND source_frame IS NULL)
        OR (source_video_file_id IS NOT NULL AND source_timestamp_ms IS NOT NULL));

-- +goose Down
ALTER TABLE case_evidences DROP CONSTRAINT case_evidences_source_check,
    DROP COLUMN source_frame, DROP COLUMN source_timestamp_ms, DROP COLUMN source_video_file_id;
DROP TABLE case_videos;
DELETE FROM case_files WHERE category = 'video';
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image'));
