-- +goose Up
-- Arrests on a warrant get a group of their own (ARREST), so the occurrence map shows them in its legend, apart from the
-- grey of the types outside any group: trackid-pf's Mitra operation places each at the camera that sighted the person.
ALTER TABLE occurrence_types DROP CONSTRAINT occurrence_types_grp_check;
ALTER TABLE occurrence_types ADD CONSTRAINT occurrence_types_grp_check
    CHECK (grp IN ('LETHAL', 'PERSON', 'ROBBERY', 'THEFT', 'FRAUD', 'DRUGS', 'CIVIL', 'OTHER', 'ARREST'));
UPDATE occurrence_types SET grp = 'ARREST' WHERE code = 'ARREST_WARRANT_SERVED';

-- +goose Down
UPDATE occurrence_types SET grp = 'OTHER' WHERE code = 'ARREST_WARRANT_SERVED';
ALTER TABLE occurrence_types DROP CONSTRAINT occurrence_types_grp_check;
ALTER TABLE occurrence_types ADD CONSTRAINT occurrence_types_grp_check
    CHECK (grp IN ('LETHAL', 'PERSON', 'ROBBERY', 'THEFT', 'FRAUD', 'DRUGS', 'CIVIL', 'OTHER'));
