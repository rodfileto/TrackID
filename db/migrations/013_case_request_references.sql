-- +goose Up
-- Other numbers a request's document goes by in the systems around it, labelled by the institute:
-- a task in its case-management system, a process in its document system. They differ from one
-- institute to the next, so they are labelled rows rather than columns, and they identify nothing
-- here: only document_reference is the request's own document. Indexed to find a request by number.
CREATE TABLE case_request_references (
    id BIGSERIAL PRIMARY KEY,
    case_request_id BIGINT NOT NULL REFERENCES case_requests(id) ON DELETE CASCADE,
    label TEXT NOT NULL CHECK (label <> ''),
    number TEXT NOT NULL CHECK (number <> ''),
    CONSTRAINT case_request_references_key UNIQUE (case_request_id, label, number)
);
CREATE INDEX case_request_references_number_idx ON case_request_references (number);

-- +goose Down
DROP TABLE case_request_references;
