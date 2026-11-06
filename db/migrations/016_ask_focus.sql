-- +goose Up
-- What the page told the model an AI mode conversation had already found (ask.Entity): people, cases,
-- occurrences and inquiries by reference. It shapes the answer, as the earlier turns do, so the record
-- keeps it.
ALTER TABLE ask_queries ADD COLUMN focus JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE ask_queries DROP COLUMN focus;
