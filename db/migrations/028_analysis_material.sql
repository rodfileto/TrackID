-- +goose Up
-- Material an examiner brings into an analysis (docs/scene-examination-plan.md, decision 6): a
-- reference document fetched for the comparison (a fingerprint card, an identity document) came
-- from no request and no scene, but from the analysis that needed it. It is a third origin, and a
-- file still has at most one.
ALTER TABLE case_files ADD COLUMN case_analysis_id BIGINT REFERENCES case_analyses(id) ON DELETE RESTRICT;
CREATE INDEX case_files_case_analysis_id_idx ON case_files (case_analysis_id);
ALTER TABLE case_files DROP CONSTRAINT case_files_one_origin_check;
ALTER TABLE case_files ADD CONSTRAINT case_files_one_origin_check
    CHECK (num_nonnulls(case_request_id, collection_id, case_analysis_id) <= 1);

-- +goose Down
ALTER TABLE case_files DROP CONSTRAINT case_files_one_origin_check;
UPDATE case_files SET case_analysis_id = NULL;
ALTER TABLE case_files ADD CONSTRAINT case_files_one_origin_check
    CHECK (case_request_id IS NULL OR collection_id IS NULL);
DROP INDEX case_files_case_analysis_id_idx;
ALTER TABLE case_files DROP COLUMN case_analysis_id;
