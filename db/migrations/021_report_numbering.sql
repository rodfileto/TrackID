-- +goose Up
-- A report's number is its issuing unit's, by discipline and year: each unit keeps one sequence per discipline, so a
-- review and a comparison share theirs, and two units can issue the same number. The issuing unit is stored on the
-- report because it can differ from the case's unit, and must not change if the case moves.
--
-- Reports already numbered take their case's unit. Numbers used to run per report type, so a unit's review and
-- comparison can both hold the same number: the first keeps its unit and the others stay without one, with the number
-- they were issued under, since an issued report is never renumbered.

-- +goose StatementBegin
CREATE FUNCTION report_discipline(report_type TEXT) RETURNS TEXT LANGUAGE SQL IMMUTABLE AS $$
    SELECT CASE
        WHEN report_type LIKE 'FACIAL_%' THEN 'FACIAL'
        WHEN report_type LIKE 'FINGERPRINT_%' THEN 'FINGERPRINT'
        ELSE 'CRIME_SCENE'
    END
$$;
-- +goose StatementEnd

ALTER TABLE analysis_reports
    ADD COLUMN issuing_unit_id BIGINT REFERENCES units(id),
    ADD COLUMN discipline TEXT GENERATED ALWAYS AS (report_discipline(report_type)) STORED;

WITH ranked AS (
    SELECT r.id, bc.unit_id, r.report_number,
           row_number() OVER (PARTITION BY bc.unit_id, r.discipline, r.report_year, r.report_number ORDER BY r.id) AS position
    FROM analysis_reports r
    JOIN case_analyses ca ON ca.id = r.case_analysis_id
    JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
    WHERE r.status = 'ISSUED'
)
UPDATE analysis_reports r SET issuing_unit_id = ranked.unit_id
FROM ranked
WHERE r.id = ranked.id AND (ranked.position = 1 OR ranked.report_number IS NULL);

DROP INDEX analysis_reports_number_key;
CREATE UNIQUE INDEX analysis_reports_number_key ON analysis_reports (issuing_unit_id, discipline, report_year, report_number);

-- +goose Down
DROP INDEX analysis_reports_number_key;
ALTER TABLE analysis_reports DROP COLUMN discipline, DROP COLUMN issuing_unit_id;
DROP FUNCTION report_discipline(TEXT);
CREATE UNIQUE INDEX analysis_reports_number_key ON analysis_reports (report_type, report_year, report_number);
