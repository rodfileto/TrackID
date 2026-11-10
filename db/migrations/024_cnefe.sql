-- +goose Up
-- IBGE's CNEFE (Censo 2022) as the first source for addresses (docs/geocoding-plan.md): a lookup in
-- TrackID's own database that sends nothing outside. Loaded by cmd/load-cnefe a state's file at a
-- time; loading a state again replaces its municipalities' rows. Only coordinates at the address's
-- own level are kept (NV_GEO_COORD 1 to 3); level is the best its addresses had, and addresses how
-- many were averaged. Public data: credit "Fonte: IBGE, Censo Demográfico 2022".

-- One row per number on a street in a locality. street_key is the street reduced (geocode.StreetKey);
-- street is how IBGE writes it, for display. The locality is part of the key: one municipality can
-- have streets of the same name in different localities (1% of the Distrito Federal's numbers are on
-- such streets, more than a kilometre apart), which averaging would put somewhere between them.
CREATE TABLE cnefe_addresses (
    municipality_code CHAR(7) NOT NULL,
    street_key TEXT NOT NULL,
    locality_key TEXT NOT NULL,
    number INTEGER NOT NULL,
    street TEXT NOT NULL,
    lat DOUBLE PRECISION NOT NULL,
    lon DOUBLE PRECISION NOT NULL,
    level SMALLINT NOT NULL,
    addresses INTEGER NOT NULL,
    locality TEXT NOT NULL,
    cep TEXT NOT NULL,
    geom GEOMETRY(Point, 4326) GENERATED ALWAYS AS (ST_SetSRID(ST_MakePoint(lon, lat), 4326)) STORED,
    PRIMARY KEY (municipality_code, street_key, locality_key, number)
);
-- The nearest address to a point (a click on the map).
CREATE INDEX cnefe_addresses_geom_idx ON cnefe_addresses USING GIST (geom);

-- Addresses with no number but a quadra, conjunto, lote, casa or bloco, as the Distrito Federal and
-- much of Goiás write them: one row per place, filed under its whole chain of locators and under each
-- shorter one (a quadra's row is its lotes' points averaged). The same key in two localities is two
-- places.
CREATE TABLE cnefe_places (
    municipality_code CHAR(7) NOT NULL,
    place_key TEXT NOT NULL,
    locality_key TEXT NOT NULL,
    lat DOUBLE PRECISION NOT NULL,
    lon DOUBLE PRECISION NOT NULL,
    level SMALLINT NOT NULL,
    addresses INTEGER NOT NULL,
    locality TEXT NOT NULL,
    cep TEXT NOT NULL,
    PRIMARY KEY (municipality_code, place_key, locality_key)
);

-- +goose Down
DROP TABLE cnefe_places;
DROP TABLE cnefe_addresses;
