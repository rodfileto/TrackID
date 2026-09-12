-- +goose Up
-- face_crop is a generic biometric processing artifact: the cropped region a face (or, later,
-- another modality's) detector produced from an evidence item, stored as its own case_files row
-- (case_traces.case_file_id points back to it) rather than embedded anywhere else.
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'documento', 'forensic_report', 'face_crop'));
