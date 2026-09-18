-- +goose Up
-- Person name search (person.SearchByName) needs substring matches
-- (ILIKE '%term%'), which a plain btree index can't accelerate -- a
-- trigram GIN index can. Unlike criminal_cases_description_idx (a
-- to_tsvector index, which only speeds up @@ tsquery, not ILIKE), this is
-- the first search in the schema expected to run substring matches against
-- a population-scale table, so it gets pg_trgm rather than a sequential
-- scan on every keystroke.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX identity_register_name_trgm_idx ON identity_register
    USING GIN (name gin_trgm_ops);

-- +goose Down
DROP INDEX identity_register_name_trgm_idx;
DROP EXTENSION IF EXISTS pg_trgm;
