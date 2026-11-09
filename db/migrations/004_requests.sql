-- +goose Up
-- Requests (docs/facial-reports-plan.md, "Request first"). A request is an authority asking the
-- institute to examine material: a police station's ofício, a prosecutor's or a court's request.
-- It is not the occurrence (the event): one occurrence can lead to several requests, and a civil
-- case has a request but no occurrence. A request is registered on its own and opens or joins one
-- or more cases (case_request_cases, 005_cases): one ofício can ask for a facial and a fingerprint
-- exam, and a later one can ask more of a case already open.
--
-- The request cites the occurrence and the inquiry separately: cited_reference is the occurrence
-- number as the request cites it (TrackID's or the police report's), cited_inquiry_number the
-- inquiry's; occurrence_id is the occurrence it was resolved to, if any.
CREATE TABLE case_requests (
    id BIGSERIAL PRIMARY KEY,
    authority TEXT NOT NULL,
    document_reference TEXT NOT NULL,
    document_date DATE,
    received_on DATE NOT NULL,
    due_on DATE,
    cited_reference TEXT,
    occurrence_id BIGINT REFERENCES occurrences(id) ON DELETE SET NULL,
    notes TEXT,
    registered_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cited_inquiry_number TEXT,
    CONSTRAINT case_requests_due_check CHECK (due_on IS NULL OR due_on >= received_on)
);
CREATE INDEX case_requests_due_idx ON case_requests (due_on);

-- The questions (quesitos) a request asks, in the order it asks them. A laudo answers each.
CREATE TABLE case_request_questions (
    id BIGSERIAL PRIMARY KEY,
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    sequence SMALLINT NOT NULL,
    question TEXT NOT NULL,
    CONSTRAINT case_request_questions_sequence_key UNIQUE (case_request_id, sequence)
);

-- +goose Down
DROP TABLE case_request_questions;
DROP TABLE case_requests;
