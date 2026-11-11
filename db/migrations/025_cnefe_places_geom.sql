-- +goose Up
-- A clicked point finds the nearest census address (geocode.CNEFE.Reverse, docs/geocoding-plan.md):
-- numbered addresses have their point and index since 024; places by quadra and lote get theirs here.
ALTER TABLE cnefe_places
    ADD COLUMN geom GEOMETRY(Point, 4326) GENERATED ALWAYS AS (ST_SetSRID(ST_MakePoint(lon, lat), 4326)) STORED;
CREATE INDEX cnefe_places_geom_idx ON cnefe_places USING GIST (geom);

-- +goose Down
DROP INDEX cnefe_places_geom_idx;
ALTER TABLE cnefe_places DROP COLUMN geom;
