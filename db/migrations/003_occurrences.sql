-- +goose Up
-- The occurrence layer (POLE: Event and Location): the registered occurrence -- a police report,
-- the boletim de ocorrência -- that a biometric case investigates, and the place it happened.
-- One occurrence can open many biometric cases (say, a FACIAL case for its CCTV and a
-- FINGERPRINT case for its latents), so the link lives on the case (biometric_cases.occurrence_id).
-- An occurrence is not necessarily a crime: as biometric_cases covers CIVIL identification,
-- occurrences cover the reports that start it (a missing person, an unidentified body). See
-- MODEL.md section 2.3.
CREATE EXTENSION IF NOT EXISTS postgis;

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

-- A place where occurrences happen. location_id is the importer's stable business key, as
-- case_id is for cases: deciding that two reports share a place (the same bank branch hit
-- twice) is the importer's job, and trackid neither geocodes nor deduplicates addresses.
--
-- precision says how exact geom is; a MUNICIPALITY point is the municipality's centroid, which
-- hotspot analysis must be able to leave out:
--   EXACT         -- surveyed or GPS
--   GEOCODED      -- from a full address
--   STREET        -- the street, not the number
--   NEIGHBOURHOOD -- a geocoder match on a neighbourhood or district
--   MUNICIPALITY  -- only the municipality is known
CREATE TABLE locations (
    id BIGSERIAL PRIMARY KEY,
    location_id TEXT NOT NULL UNIQUE,
    name TEXT,
    geom GEOGRAPHY(Point, 4326) NOT NULL,
    precision TEXT NOT NULL CHECK (precision IN ('EXACT', 'GEOCODED', 'STREET', 'NEIGHBOURHOOD', 'MUNICIPALITY')),
    municipality_code CHAR(7) NOT NULL CHECK (municipality_code ~ '^[0-9]{7}$'), -- IBGE
    uf CHAR(2) NOT NULL,
    neighbourhood TEXT,
    address TEXT,
    place_type TEXT CHECK (place_type IN ('PUBLIC_ROAD', 'RESIDENCE', 'COMMERCE', 'BANK',
        'GOVERNMENT_OFFICE', 'TRANSPORT', 'RURAL', 'OTHER')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX locations_geom_idx ON locations USING GIST (geom);
CREATE INDEX locations_municipality_code_idx ON locations (municipality_code);

-- An occurrence is identified one of two ways, and every occurrence has at least one:
--   - by the police agency's own report number (agency, occurrence_number): both or neither,
--     unique per agency. Occurrences an importer ingests keep theirs, as imported cases keep
--     their own case_id.
--   - by a TrackID number "<year>.<sequence>" (2026.0001), as cases have
--     (biometric_cases.case_year/case_number): occurrence_year and occurrence_seq, generated in
--     Go (occurrences.Create), the unique index below the only database-level guard against a
--     duplicate.
--
-- occurred_from/occurred_to is a window, not an instant: many crimes are found after the fact
-- (a burglary discovered on Monday). occurred_to is NULL when the moment is known.
--
-- attempted and offender_count are crime-only: NULL on a civil occurrence, whose location is
-- where the person was last seen or the body was found.
--
-- status:
--   REGISTERED          -- the report exists, nothing more
--   UNDER_INVESTIGATION -- being worked
--   INQUIRY_OPENED      -- a formal inquiry (inquérito) was opened
--   CLOSED              -- closed, including a missing person located
CREATE TABLE occurrences (
    id BIGSERIAL PRIMARY KEY,
    agency TEXT,
    occurrence_number TEXT,
    occurrence_year INTEGER,
    occurrence_seq INTEGER,
    location_id BIGINT NOT NULL REFERENCES locations(id),
    occurred_from TIMESTAMPTZ NOT NULL,
    occurred_to TIMESTAMPTZ CHECK (occurred_to >= occurred_from),
    reported_at TIMESTAMPTZ,
    registered_at TIMESTAMPTZ NOT NULL,
    attempted BOOLEAN,
    offender_count SMALLINT CHECK (offender_count >= 0),
    victim_count SMALLINT CHECK (victim_count >= 0),
    status TEXT NOT NULL CHECK (status IN ('REGISTERED', 'UNDER_INVESTIGATION', 'INQUIRY_OPENED', 'CLOSED')),
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT occurrences_agency_number_key UNIQUE (agency, occurrence_number),
    CONSTRAINT occurrences_agency_number_pair_check CHECK ((agency IS NULL) = (occurrence_number IS NULL)),
    CONSTRAINT occurrences_identified_check
        CHECK (occurrence_number IS NOT NULL OR (occurrence_year IS NOT NULL AND occurrence_seq IS NOT NULL))
);

CREATE UNIQUE INDEX occurrences_year_seq_key ON occurrences (occurrence_year, occurrence_seq);
CREATE INDEX occurrences_location_id_idx ON occurrences (location_id);
CREATE INDEX occurrences_occurred_from_idx ON occurrences (occurred_from);
CREATE INDEX occurrences_description_idx ON occurrences USING GIN (to_tsvector('simple', description));

-- The occurrence types follow the classification of ISP-RJ (Instituto de Segurança Pública do
-- Rio de Janeiro), whose open data counts occurrences per police-station area (CISP) and month:
-- one type per ISP crime or civil category, so counts by type compare with ISP's directly. Left
-- out: ISP's totals (total_roubos, cvli, ...) and police productivity (arrests, seizures, vehicles
-- recovered). Attempts are a flag (occurrences.attempted), so ISP's tentat_hom is a HOMICIDE with
-- attempted = TRUE. Bank fraud, including the scam of "helping" someone at an ATM, is FRAUD
-- (estelionato).
--
-- Not in ISP, kept because they open biometric cases: IDENTITY_FRAUD (false identity, use of a
-- forged document -- a duplicate or impostor enrollment), UNIDENTIFIED_LIVING_PERSON and
-- DISASTER_VICTIM.
--
-- Codes only: display names are the frontend's translations. case_type is the nature of the
-- biometric case such an occurrence opens, as in biometric_cases.case_type.
CREATE TABLE occurrence_types (
    code TEXT PRIMARY KEY,
    case_type TEXT NOT NULL CHECK (case_type IN ('CRIMINAL', 'CIVIL')),
    grp TEXT NOT NULL CHECK (grp IN ('LETHAL', 'PERSON', 'ROBBERY', 'THEFT', 'FRAUD', 'DRUGS', 'CIVIL'))
);

INSERT INTO occurrence_types (code, case_type, grp) VALUES
    ('HOMICIDE', 'CRIMINAL', 'LETHAL'),
    ('FEMICIDE', 'CRIMINAL', 'LETHAL'),
    ('ROBBERY_HOMICIDE', 'CRIMINAL', 'LETHAL'),
    ('BODILY_INJURY_DEATH', 'CRIMINAL', 'LETHAL'),
    ('POLICE_INTERVENTION_DEATH', 'CRIMINAL', 'LETHAL'),
    ('NEGLIGENT_HOMICIDE', 'CRIMINAL', 'LETHAL'),
    ('ASSAULT', 'CRIMINAL', 'PERSON'),
    ('NEGLIGENT_BODILY_INJURY', 'CRIMINAL', 'PERSON'),
    ('RAPE', 'CRIMINAL', 'PERSON'),
    ('KIDNAPPING', 'CRIMINAL', 'PERSON'),
    ('EXPRESS_KIDNAPPING', 'CRIMINAL', 'PERSON'),
    ('EXTORTION', 'CRIMINAL', 'PERSON'),
    ('THREAT', 'CRIMINAL', 'PERSON'),
    ('STREET_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('PHONE_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('PUBLIC_TRANSPORT_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('VEHICLE_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('CARGO_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('COMMERCIAL_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('HOME_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('BANK_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('ATM_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('FORCED_WITHDRAWAL_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('POST_WITHDRAWAL_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('BICYCLE_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('OTHER_ROBBERY', 'CRIMINAL', 'ROBBERY'),
    ('VEHICLE_THEFT', 'CRIMINAL', 'THEFT'),
    ('STREET_THEFT', 'CRIMINAL', 'THEFT'),
    ('PUBLIC_TRANSPORT_THEFT', 'CRIMINAL', 'THEFT'),
    ('PHONE_THEFT', 'CRIMINAL', 'THEFT'),
    ('BICYCLE_THEFT', 'CRIMINAL', 'THEFT'),
    ('OTHER_THEFT', 'CRIMINAL', 'THEFT'),
    ('FRAUD', 'CRIMINAL', 'FRAUD'),
    ('IDENTITY_FRAUD', 'CRIMINAL', 'FRAUD'),
    ('DRUG_POSSESSION', 'CRIMINAL', 'DRUGS'),
    ('DRUG_TRAFFICKING', 'CRIMINAL', 'DRUGS'),
    ('MISSING_PERSON', 'CIVIL', 'CIVIL'),
    ('BODY_FOUND', 'CIVIL', 'CIVIL'),
    ('SKELETAL_REMAINS_FOUND', 'CIVIL', 'CIVIL'),
    ('UNIDENTIFIED_LIVING_PERSON', 'CIVIL', 'CIVIL'),
    ('DISASTER_VICTIM', 'CIVIL', 'CIVIL');

-- A report can carry several types (a robbery that ended in a homicide).
CREATE TABLE occurrence_classifications (
    occurrence_id BIGINT NOT NULL REFERENCES occurrences(id) ON DELETE CASCADE,
    type_code TEXT NOT NULL REFERENCES occurrence_types(code),
    PRIMARY KEY (occurrence_id, type_code)
);

CREATE INDEX occurrence_classifications_type_code_idx ON occurrence_classifications (type_code);

-- +goose Down
DROP TABLE occurrence_classifications;
DROP TABLE occurrence_types;
DROP TABLE occurrences;
DROP TABLE locations;
DROP TABLE municipalities;
-- The postgis extension stays: other objects may come to depend on it.
