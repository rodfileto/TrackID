-- +goose Up
-- case_evidences.case_file_id was ON DELETE SET NULL, so deleting an evidence's
-- case_files row (cases.DeleteEvidence, guarded to only run when the evidence has
-- no traces) left an orphaned case_evidences row behind with a null case_file_id
-- and nothing pointing at it. Since DeleteEvidence already refuses to delete a
-- file that has traces, cascading here is safe -- it can never take case_traces
-- down with it.
ALTER TABLE case_evidences DROP CONSTRAINT case_evidences_case_file_id_fkey;
ALTER TABLE case_evidences ADD CONSTRAINT case_evidences_case_file_id_fkey
    FOREIGN KEY (case_file_id) REFERENCES case_files(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE case_evidences DROP CONSTRAINT case_evidences_case_file_id_fkey;
ALTER TABLE case_evidences ADD CONSTRAINT case_evidences_case_file_id_fkey
    FOREIGN KEY (case_file_id) REFERENCES case_files(id) ON DELETE SET NULL;
