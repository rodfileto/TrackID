-- +goose Up
-- An occurrence can be registered before its place or its date are known, and old documents may
-- never give them: a laboratory report says nothing of a scene's address, and a scene report's date
-- is the examination's, not the crime's. Leaving them empty keeps an invented point off the map and
-- an invented date out of the statistics. The occurrence's identity is unaffected
-- (occurrences_identified_check).
ALTER TABLE occurrences ALTER COLUMN location_id DROP NOT NULL;
ALTER TABLE occurrences ALTER COLUMN occurred_from DROP NOT NULL;

-- A crime whose type a document doesn't say, so no specific type has to be guessed. It counts as
-- criminal only because that is the nature of the cases it opens.
ALTER TABLE occurrence_types DROP CONSTRAINT occurrence_types_grp_check;
ALTER TABLE occurrence_types ADD CONSTRAINT occurrence_types_grp_check
    CHECK (grp IN ('LETHAL', 'PERSON', 'ROBBERY', 'THEFT', 'FRAUD', 'DRUGS', 'CIVIL', 'OTHER'));
INSERT INTO occurrence_types (code, case_type, grp) VALUES ('UNDETERMINED', 'CRIMINAL', 'OTHER');

-- +goose Down
-- Fails while any occurrence has no place or no date: those need one before going back.
DELETE FROM occurrence_classifications WHERE type_code = 'UNDETERMINED';
DELETE FROM occurrence_types WHERE code = 'UNDETERMINED';
ALTER TABLE occurrence_types DROP CONSTRAINT occurrence_types_grp_check;
ALTER TABLE occurrence_types ADD CONSTRAINT occurrence_types_grp_check
    CHECK (grp IN ('LETHAL', 'PERSON', 'ROBBERY', 'THEFT', 'FRAUD', 'DRUGS', 'CIVIL'));
ALTER TABLE occurrences ALTER COLUMN occurred_from SET NOT NULL;
ALTER TABLE occurrences ALTER COLUMN location_id SET NOT NULL;
