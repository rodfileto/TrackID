-- +goose Up
-- Modus operandi tags are dropped from the model for now: free tags nobody curated, and not
-- needed by the analyses the occurrence layer serves yet.
DROP INDEX occurrences_modus_operandi_idx;
ALTER TABLE occurrences DROP COLUMN modus_operandi;

-- A place type for public agencies and government offices (órgão público).
ALTER TABLE locations DROP CONSTRAINT locations_place_type_check;
ALTER TABLE locations ADD CONSTRAINT locations_place_type_check CHECK (place_type IN ('PUBLIC_ROAD',
    'RESIDENCE', 'COMMERCE', 'BANK', 'GOVERNMENT_OFFICE', 'TRANSPORT', 'RURAL', 'OTHER'));

-- +goose Down
UPDATE locations SET place_type = 'OTHER' WHERE place_type = 'GOVERNMENT_OFFICE';
ALTER TABLE locations DROP CONSTRAINT locations_place_type_check;
ALTER TABLE locations ADD CONSTRAINT locations_place_type_check CHECK (place_type IN ('PUBLIC_ROAD',
    'RESIDENCE', 'COMMERCE', 'BANK', 'TRANSPORT', 'RURAL', 'OTHER'));

ALTER TABLE occurrences ADD COLUMN modus_operandi TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX occurrences_modus_operandi_idx ON occurrences USING GIN (modus_operandi);
