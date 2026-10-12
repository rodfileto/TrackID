-- +goose Up
-- Occurrence types now follow the classification of ISP-RJ (Instituto de Segurança Pública do
-- Rio de Janeiro), whose open data counts occurrences per police-station area (CISP) and month:
-- one type per ISP crime or civil category, so counts by type compare with ISP's directly. Left
-- out: ISP's totals (total_roubos, cvli, ...) and police productivity (arrests, seizures, vehicles
-- recovered). Attempts stay a flag (occurrences.attempted), so ISP's tentat_hom is a HOMICIDE
-- with attempted = TRUE. Bank fraud, including the scam of "helping" someone at an ATM, is FRAUD
-- (estelionato) with a modus_operandi tag.
--
-- Not in ISP, kept because they open biometric cases: IDENTITY_FRAUD (false identity, use of a
-- forged document -- a duplicate or impostor enrollment), UNIDENTIFIED_LIVING_PERSON and
-- DISASTER_VICTIM.
--
-- Gone: RESIDENTIAL_BURGLARY and SHOP_THEFT (ISP counts them in outros_furtos: OTHER_THEFT),
-- ATM_ATTACK (now ATM_ROBBERY, ISP's roubo_cx_eletronico), UNIDENTIFIED_BODY (now BODY_FOUND,
-- ISP's encontro_cadaver: a body found, identified or not). Deleting a type still in use fails.
--
-- locations.precision gains NEIGHBOURHOOD: a geocoder match on a neighbourhood or district,
-- finer than a municipality centroid but not a street.
ALTER TABLE occurrence_types DROP CONSTRAINT occurrence_types_grp_check;

DELETE FROM occurrence_types WHERE code NOT IN ('HOMICIDE', 'FEMICIDE', 'ROBBERY_HOMICIDE', 'BODILY_INJURY_DEATH', 'POLICE_INTERVENTION_DEATH', 'NEGLIGENT_HOMICIDE', 'ASSAULT', 'NEGLIGENT_BODILY_INJURY', 'RAPE', 'KIDNAPPING', 'EXPRESS_KIDNAPPING', 'EXTORTION', 'THREAT', 'STREET_ROBBERY', 'PHONE_ROBBERY', 'PUBLIC_TRANSPORT_ROBBERY', 'VEHICLE_ROBBERY', 'CARGO_ROBBERY', 'COMMERCIAL_ROBBERY', 'HOME_ROBBERY', 'BANK_ROBBERY', 'ATM_ROBBERY', 'FORCED_WITHDRAWAL_ROBBERY', 'POST_WITHDRAWAL_ROBBERY', 'BICYCLE_ROBBERY', 'OTHER_ROBBERY', 'VEHICLE_THEFT', 'STREET_THEFT', 'PUBLIC_TRANSPORT_THEFT', 'PHONE_THEFT', 'BICYCLE_THEFT', 'OTHER_THEFT', 'FRAUD', 'IDENTITY_FRAUD', 'DRUG_POSSESSION', 'DRUG_TRAFFICKING', 'MISSING_PERSON', 'BODY_FOUND', 'SKELETAL_REMAINS_FOUND', 'UNIDENTIFIED_LIVING_PERSON', 'DISASTER_VICTIM');

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
    ('DISASTER_VICTIM', 'CIVIL', 'CIVIL')
ON CONFLICT (code) DO UPDATE SET case_type = EXCLUDED.case_type, grp = EXCLUDED.grp;

ALTER TABLE occurrence_types ADD CONSTRAINT occurrence_types_grp_check
    CHECK (grp IN ('LETHAL', 'PERSON', 'ROBBERY', 'THEFT', 'FRAUD', 'DRUGS', 'CIVIL'));

ALTER TABLE locations DROP CONSTRAINT locations_precision_check;
ALTER TABLE locations ADD CONSTRAINT locations_precision_check
    CHECK (precision IN ('EXACT', 'GEOCODED', 'STREET', 'NEIGHBOURHOOD', 'MUNICIPALITY'));

-- +goose Down
-- A NEIGHBOURHOOD point becomes MUNICIPALITY, the coarser claim, so analysis keeps treating it
-- as approximate.
UPDATE locations SET precision = 'MUNICIPALITY' WHERE precision = 'NEIGHBOURHOOD';
ALTER TABLE locations DROP CONSTRAINT locations_precision_check;
ALTER TABLE locations ADD CONSTRAINT locations_precision_check
    CHECK (precision IN ('EXACT', 'GEOCODED', 'STREET', 'MUNICIPALITY'));

ALTER TABLE occurrence_types DROP CONSTRAINT occurrence_types_grp_check;

DELETE FROM occurrence_types WHERE code NOT IN ('RESIDENTIAL_BURGLARY', 'SHOP_THEFT', 'VEHICLE_THEFT', 'STREET_ROBBERY', 'COMMERCIAL_ROBBERY', 'HOME_ROBBERY', 'VEHICLE_ROBBERY', 'CARGO_ROBBERY', 'BANK_ROBBERY', 'ATM_ATTACK', 'EXPRESS_KIDNAPPING', 'HOMICIDE', 'FEMICIDE', 'ROBBERY_HOMICIDE', 'RAPE', 'IDENTITY_FRAUD', 'MISSING_PERSON', 'UNIDENTIFIED_BODY', 'UNIDENTIFIED_LIVING_PERSON', 'DISASTER_VICTIM');

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
    ('DISASTER_VICTIM', 'CIVIL', 'CIVIL')
ON CONFLICT (code) DO UPDATE SET case_type = EXCLUDED.case_type, grp = EXCLUDED.grp;

ALTER TABLE occurrence_types ADD CONSTRAINT occurrence_types_grp_check
    CHECK (grp IN ('PROPERTY', 'PERSON', 'FRAUD', 'CIVIL'));
