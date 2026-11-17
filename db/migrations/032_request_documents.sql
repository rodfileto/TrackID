-- +goose Up
-- The request's own document -- the ePol task it arrived as, or an ofício -- kept with each case the
-- request opens or joins. It says what is asked; it is not material to examine, so it is not an evidence.
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'video', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image',
        'collection_note', 'request_document'));
ALTER TABLE case_files ADD CONSTRAINT case_files_request_document_check
    CHECK (category <> 'request_document' OR case_request_id IS NOT NULL);

-- +goose Down
DELETE FROM case_files WHERE category = 'request_document';
ALTER TABLE case_files DROP CONSTRAINT case_files_request_document_check;
ALTER TABLE case_files DROP CONSTRAINT case_files_category_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_category_check
    CHECK (category IN ('evidence', 'video', 'document', 'forensic_report', 'report_draft', 'face_crop', 'codification_image',
        'collection_note'));
