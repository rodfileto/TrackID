-- +goose Up
-- A request first (docs/facial-reports-plan.md, "Request first"): a request is registered on its
-- own and opens or joins one or more cases -- one ofício can ask for a facial and a fingerprint
-- exam, and a later one can ask more of a case already open. The request keeps its number per
-- case here (sequence), so a case still lists its requests 1, 2, ...
CREATE TABLE case_request_cases (
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    biometric_case_id BIGINT NOT NULL REFERENCES biometric_cases(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (case_request_id, biometric_case_id),
    CONSTRAINT case_request_cases_case_sequence_key UNIQUE (biometric_case_id, sequence)
);

INSERT INTO case_request_cases (case_request_id, biometric_case_id, sequence, created_at)
SELECT id, biometric_case_id, sequence, created_at FROM case_requests;

ALTER TABLE case_requests DROP CONSTRAINT case_requests_case_sequence_key,
    DROP COLUMN biometric_case_id, DROP COLUMN sequence;

-- +goose Down
-- A request on several cases keeps only its first.
ALTER TABLE case_requests ADD COLUMN biometric_case_id BIGINT REFERENCES biometric_cases(id) ON DELETE CASCADE,
    ADD COLUMN sequence SMALLINT;
UPDATE case_requests r SET biometric_case_id = l.biometric_case_id, sequence = l.sequence
FROM (SELECT DISTINCT ON (case_request_id) case_request_id, biometric_case_id, sequence
      FROM case_request_cases ORDER BY case_request_id, created_at, biometric_case_id) l
WHERE l.case_request_id = r.id;
DELETE FROM case_requests WHERE biometric_case_id IS NULL;
ALTER TABLE case_requests ALTER COLUMN biometric_case_id SET NOT NULL, ALTER COLUMN sequence SET NOT NULL,
    ADD CONSTRAINT case_requests_case_sequence_key UNIQUE (biometric_case_id, sequence);
DROP TABLE case_request_cases;
