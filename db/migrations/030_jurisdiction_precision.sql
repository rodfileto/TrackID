-- +goose Up
-- A location of precision JURISDICTION is not where the occurrence happened: it is the municipality of the unit
-- investigating it, given to an occurrence imported with no place so it still has a municipality and a state. Nothing
-- geographic reads it as a place: the occurrence map, the searches by distance, the nearby locations offered for
-- reuse, the AI mode's place filters and breakdowns, and the laudo leave it out, as they leave out an occurrence with
-- no place. Setting the occurrence's real place (occurrences.SetPlace) replaces it.
ALTER TABLE locations DROP CONSTRAINT locations_precision_check;
ALTER TABLE locations ADD CONSTRAINT locations_precision_check
    CHECK (precision IN ('EXACT', 'GEOCODED', 'STREET', 'NEIGHBOURHOOD', 'MUNICIPALITY', 'JURISDICTION'));

-- +goose Down
UPDATE occurrences SET location_id = NULL
WHERE location_id IN (SELECT id FROM locations WHERE precision = 'JURISDICTION');
DELETE FROM locations WHERE precision = 'JURISDICTION';
ALTER TABLE locations DROP CONSTRAINT locations_precision_check;
ALTER TABLE locations ADD CONSTRAINT locations_precision_check
    CHECK (precision IN ('EXACT', 'GEOCODED', 'STREET', 'NEIGHBOURHOOD', 'MUNICIPALITY'));
