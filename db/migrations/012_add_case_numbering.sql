-- +goose Up
-- Cases created through the API get a TrackID-generated number "<year>.<sequence>"
-- (e.g. 2026.0001). case_year + case_number store the two parts; the numbering logic lives
-- in Go (the cases package reads the current max per year and increments), and the unique
-- index below is the only database-level enforcement against duplicate year+number.
-- Cases imported by an organization's own import command keep their own case_id and leave
-- these two columns NULL, so this numbering does not interfere with the import contract.
ALTER TABLE criminal_cases
    ADD COLUMN case_year INTEGER,
    ADD COLUMN case_number INTEGER;

CREATE UNIQUE INDEX criminal_cases_year_number_key ON criminal_cases (case_year, case_number);

-- +goose Down
DROP INDEX criminal_cases_year_number_key;
ALTER TABLE criminal_cases
    DROP COLUMN case_number,
    DROP COLUMN case_year;
