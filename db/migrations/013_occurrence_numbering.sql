-- +goose Up
-- Occurrences registered in TrackID get a TrackID number, "<year>.<sequence>" (2026.0001), as
-- cases do (biometric_cases.case_year/case_number): occurrence_year and occurrence_seq, generated
-- in Go (occurrences.Create), the unique index below the only database-level guard against a
-- duplicate. The police agency's own report number stays in agency/occurrence_number, now
-- optional for a registered occurrence: both or neither, unique per agency when present.
-- Occurrences an importer ingests keep their own (agency, number) and no TrackID number, as
-- imported cases keep their own case_id. Every occurrence has one of the two.
ALTER TABLE occurrences ALTER COLUMN agency DROP NOT NULL;
ALTER TABLE occurrences ALTER COLUMN occurrence_number DROP NOT NULL;
ALTER TABLE occurrences ADD COLUMN occurrence_year INTEGER;
ALTER TABLE occurrences ADD COLUMN occurrence_seq INTEGER;
ALTER TABLE occurrences ADD CONSTRAINT occurrences_agency_number_pair_check
    CHECK ((agency IS NULL) = (occurrence_number IS NULL));
ALTER TABLE occurrences ADD CONSTRAINT occurrences_identified_check
    CHECK (occurrence_number IS NOT NULL OR (occurrence_year IS NOT NULL AND occurrence_seq IS NOT NULL));
CREATE UNIQUE INDEX occurrences_year_seq_key ON occurrences (occurrence_year, occurrence_seq);

-- +goose Down
-- Occurrences registered in TrackID without a police number would have no key left: they go.
DELETE FROM occurrences WHERE occurrence_number IS NULL;
DROP INDEX occurrences_year_seq_key;
ALTER TABLE occurrences DROP CONSTRAINT occurrences_identified_check;
ALTER TABLE occurrences DROP CONSTRAINT occurrences_agency_number_pair_check;
ALTER TABLE occurrences DROP COLUMN occurrence_seq;
ALTER TABLE occurrences DROP COLUMN occurrence_year;
ALTER TABLE occurrences ALTER COLUMN occurrence_number SET NOT NULL;
ALTER TABLE occurrences ALTER COLUMN agency SET NOT NULL;
