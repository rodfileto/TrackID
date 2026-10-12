-- +goose Up
-- The occurrence layer (POLE: Event and Location): the registered occurrence -- a police report,
-- the boletim de ocorrência -- that a biometric case investigates, and the place it happened.
-- One occurrence can open many biometric cases (say, a FACIAL case for its CCTV and a
-- FINGERPRINT case for its latents), so the link lives on the case. An occurrence is not
-- necessarily a crime: as biometric_cases covers CIVIL identification, occurrences cover the
-- reports that start it (a missing person, an unidentified body). See MODEL.md section 2.3.
CREATE EXTENSION IF NOT EXISTS postgis;

-- A place where occurrences happen. location_id is the importer's stable business key, as
-- case_id is for cases: deciding that two reports share a place (the same bank branch hit
-- twice) is the importer's job, and trackid neither geocodes nor deduplicates addresses.
--
-- precision says how exact geom is; a MUNICIPALITY point is the municipality's centroid, which
-- hotspot analysis must be able to leave out:
--   EXACT        -- surveyed or GPS
--   GEOCODED     -- from a full address
--   STREET       -- the street, not the number
--   MUNICIPALITY -- only the municipality is known
CREATE TABLE locations (
    id BIGSERIAL PRIMARY KEY,
    location_id TEXT NOT NULL UNIQUE,
    name TEXT,
    geom GEOGRAPHY(Point, 4326) NOT NULL,
    precision TEXT NOT NULL CHECK (precision IN ('EXACT', 'GEOCODED', 'STREET', 'MUNICIPALITY')),
    municipality_code CHAR(7) NOT NULL CHECK (municipality_code ~ '^[0-9]{7}$'), -- IBGE
    uf CHAR(2) NOT NULL,
    neighbourhood TEXT,
    address TEXT,
    place_type TEXT CHECK (place_type IN ('PUBLIC_ROAD', 'RESIDENCE', 'COMMERCE', 'BANK',
        'TRANSPORT', 'RURAL', 'OTHER')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX locations_geom_idx ON locations USING GIST (geom);
CREATE INDEX locations_municipality_code_idx ON locations (municipality_code);

-- occurred_from/occurred_to is a window, not an instant: many crimes are found after the fact
-- (a burglary discovered on Monday). occurred_to is NULL when the moment is known.
--
-- attempted, offender_count and modus_operandi are crime-only: NULL/empty on a civil
-- occurrence, whose location is where the person was last seen or the body was found.
-- modus_operandi holds free tags (FIREARM, MOTORCYCLE, FORCED_ENTRY, ...).
--
-- status:
--   REGISTERED          -- the report exists, nothing more
--   UNDER_INVESTIGATION -- being worked
--   INQUIRY_OPENED      -- a formal inquiry (inquérito) was opened
--   CLOSED              -- closed, including a missing person located
CREATE TABLE occurrences (
    id BIGSERIAL PRIMARY KEY,
    agency TEXT NOT NULL,
    occurrence_number TEXT NOT NULL,
    location_id BIGINT NOT NULL REFERENCES locations(id),
    occurred_from TIMESTAMPTZ NOT NULL,
    occurred_to TIMESTAMPTZ CHECK (occurred_to >= occurred_from),
    reported_at TIMESTAMPTZ,
    registered_at TIMESTAMPTZ NOT NULL,
    attempted BOOLEAN,
    offender_count SMALLINT CHECK (offender_count >= 0),
    victim_count SMALLINT CHECK (victim_count >= 0),
    modus_operandi TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL CHECK (status IN ('REGISTERED', 'UNDER_INVESTIGATION', 'INQUIRY_OPENED', 'CLOSED')),
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT occurrences_agency_number_key UNIQUE (agency, occurrence_number)
);

CREATE INDEX occurrences_location_id_idx ON occurrences (location_id);
CREATE INDEX occurrences_occurred_from_idx ON occurrences (occurred_from);
CREATE INDEX occurrences_modus_operandi_idx ON occurrences USING GIN (modus_operandi);
CREATE INDEX occurrences_description_idx ON occurrences USING GIN (to_tsvector('simple', description));

-- The occurrence types, chosen for what produces questioned fingerprints or faces. Codes only:
-- display names are the frontend's translations. case_type is the nature of the biometric case
-- such an occurrence opens, as in biometric_cases.case_type.
CREATE TABLE occurrence_types (
    code TEXT PRIMARY KEY,
    case_type TEXT NOT NULL CHECK (case_type IN ('CRIMINAL', 'CIVIL')),
    grp TEXT NOT NULL CHECK (grp IN ('PROPERTY', 'PERSON', 'FRAUD', 'CIVIL'))
);

INSERT INTO occurrence_types (code, case_type, grp) VALUES
    ('RESIDENTIAL_BURGLARY', 'CRIMINAL', 'PROPERTY'),
    ('SHOP_THEFT', 'CRIMINAL', 'PROPERTY'),
    ('VEHICLE_THEFT', 'CRIMINAL', 'PROPERTY'),
    ('STREET_ROBBERY', 'CRIMINAL', 'PROPERTY'),
    ('COMMERCIAL_ROBBERY', 'CRIMINAL', 'PROPERTY'),
    ('HOME_ROBBERY', 'CRIMINAL', 'PROPERTY'),
    ('VEHICLE_ROBBERY', 'CRIMINAL', 'PROPERTY'),
    ('CARGO_ROBBERY', 'CRIMINAL', 'PROPERTY'),
    ('BANK_ROBBERY', 'CRIMINAL', 'PROPERTY'),
    ('ATM_ATTACK', 'CRIMINAL', 'PROPERTY'),
    ('EXPRESS_KIDNAPPING', 'CRIMINAL', 'PERSON'),
    ('HOMICIDE', 'CRIMINAL', 'PERSON'),
    ('FEMICIDE', 'CRIMINAL', 'PERSON'),
    ('ROBBERY_HOMICIDE', 'CRIMINAL', 'PERSON'),
    ('RAPE', 'CRIMINAL', 'PERSON'),
    ('IDENTITY_FRAUD', 'CRIMINAL', 'FRAUD'),
    ('MISSING_PERSON', 'CIVIL', 'CIVIL'),
    ('UNIDENTIFIED_BODY', 'CIVIL', 'CIVIL'),
    ('UNIDENTIFIED_LIVING_PERSON', 'CIVIL', 'CIVIL'),
    ('DISASTER_VICTIM', 'CIVIL', 'CIVIL');

-- A report can carry several types (a robbery that ended in a homicide).
CREATE TABLE occurrence_classifications (
    occurrence_id BIGINT NOT NULL REFERENCES occurrences(id) ON DELETE CASCADE,
    type_code TEXT NOT NULL REFERENCES occurrence_types(code),
    PRIMARY KEY (occurrence_id, type_code)
);

CREATE INDEX occurrence_classifications_type_code_idx ON occurrence_classifications (type_code);

-- NULL until the case is linked to its occurrence (and for a case with no report at all).
ALTER TABLE biometric_cases
    ADD COLUMN occurrence_id BIGINT REFERENCES occurrences(id) ON DELETE SET NULL;

CREATE INDEX biometric_cases_occurrence_id_idx ON biometric_cases (occurrence_id);

-- +goose Down
DROP INDEX biometric_cases_occurrence_id_idx;
ALTER TABLE biometric_cases DROP COLUMN occurrence_id;
DROP TABLE occurrence_classifications;
DROP TABLE occurrence_types;
DROP TABLE occurrences;
DROP TABLE locations;
-- The postgis extension stays: other objects may come to depend on it.
