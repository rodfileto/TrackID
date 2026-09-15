-- +goose Up
-- codification_image is the rendered result of manually adjusting a trace's codification
-- image (crop to the trace's box, brightness/contrast/saturation, interpolation -- see
-- CodificationEditorModal) -- stored as its own case_files row, the same way face_crop
-- already stores a detector's cropped region. case_codifications.case_file_id points to it;
-- a codification with no saved image yet just leaves the column NULL.
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'documento', 'forensic_report', 'face_crop', 'codification_image'));

ALTER TABLE case_codifications
    ADD COLUMN case_file_id BIGINT REFERENCES case_files(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE case_codifications DROP COLUMN case_file_id;

ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'documento', 'forensic_report', 'face_crop'));
