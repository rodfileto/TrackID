-- +goose Up
-- The inquiry (inquérito policial, IPL) number, apart from the police report (BO) number. An
-- inquiry is not always opened from a report: it can itself be the document that registers the
-- occurrence (an IPL opened by portaria, with no BO), so an occurrence carries its inquiry number
-- in a field of its own rather than in occurrence_number. Not unique: inquiries are numbered by
-- the unit that opens them, which need not be the report's agency, and one inquiry can gather
-- several occurrences.
ALTER TABLE occurrences ADD COLUMN inquiry_number TEXT;
CREATE INDEX occurrences_inquiry_number_idx ON occurrences (inquiry_number);

-- An occurrence is now identified one of three ways: the police report number, TrackID's number,
-- or -- for an imported occurrence with no report -- the inquiry that registers it. Among those,
-- the inquiry number is the importer's business key (occurrences.Ingest), as (agency,
-- occurrence_number) is for reported ones: an inquiry registers one such occurrence.
ALTER TABLE occurrences DROP CONSTRAINT occurrences_identified_check,
    ADD CONSTRAINT occurrences_identified_check CHECK (occurrence_number IS NOT NULL OR inquiry_number IS NOT NULL
        OR (occurrence_year IS NOT NULL AND occurrence_seq IS NOT NULL));
CREATE UNIQUE INDEX occurrences_inquiry_registered_key ON occurrences (inquiry_number)
    WHERE occurrence_number IS NULL AND occurrence_seq IS NULL;

-- A request cites the occurrence number and the inquiry number separately: cited_reference is the
-- occurrence number (TrackID's or the police report's), cited_inquiry_number the inquiry's.
ALTER TABLE case_requests ADD COLUMN cited_inquiry_number TEXT;

-- +goose Down
ALTER TABLE case_requests DROP COLUMN cited_inquiry_number;
-- Occurrences identified by their inquiry alone cannot be kept.
DELETE FROM occurrences WHERE occurrence_number IS NULL AND occurrence_seq IS NULL;
DROP INDEX occurrences_inquiry_registered_key;
ALTER TABLE occurrences DROP CONSTRAINT occurrences_identified_check,
    ADD CONSTRAINT occurrences_identified_check
        CHECK (occurrence_number IS NOT NULL OR (occurrence_year IS NOT NULL AND occurrence_seq IS NOT NULL));
DROP INDEX occurrences_inquiry_number_idx;
ALTER TABLE occurrences DROP COLUMN inquiry_number;
