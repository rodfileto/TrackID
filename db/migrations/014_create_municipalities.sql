-- +goose Up
-- IBGE municipalities with their boundaries, so a location's municipality_code comes from its
-- point (occurrences.Create) rather than being typed. Loaded by cmd/load-municipalities from the
-- GeoJSON tools/municipalities writes with geobr (IPEA, MIT; boundaries by IBGE). A deployment
-- loads the states it covers.
CREATE TABLE municipalities (
    code CHAR(7) PRIMARY KEY CHECK (code ~ '^[0-9]{7}$'),
    name TEXT NOT NULL,
    uf CHAR(2) NOT NULL,
    geom GEOGRAPHY(MultiPolygon, 4326) NOT NULL,
    source TEXT NOT NULL, -- e.g. "geobr read_municipality, year 2022, simplified"
    loaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX municipalities_geom_idx ON municipalities USING GIST (geom);

-- +goose Down
DROP TABLE municipalities;
