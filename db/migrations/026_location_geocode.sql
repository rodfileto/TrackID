-- +goose Up
-- Where a location's point or address came from, and what to check about it, as the geocoder said
-- (geocode.Result's Source and Flags, docs/geocoding-plan.md): "cnefe2022:<municipality>" for IBGE's
-- census list, "osm:way/…" for OpenStreetMap, "ibge:<code>" for a municipality's centre. Empty for a
-- point placed by hand with its address typed. Each source's credit is shown with it.
ALTER TABLE locations
    ADD COLUMN geocode_source TEXT,
    ADD COLUMN geocode_flags TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE locations DROP COLUMN geocode_flags, DROP COLUMN geocode_source;
