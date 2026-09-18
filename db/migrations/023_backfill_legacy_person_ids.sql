-- +goose Up
-- One-off backfill: every identity_document left with a NULL person_id
-- after 022_bridge_local_dev_person_gap.sql added the column gets its own
-- new person. The legacy data has no field that reliably groups documents
-- belonging to the same real person (fiscal_number is empty on all of
-- them), so this assigns one person per document rather than guessing at a
-- merge -- a real cross-document match/dedup is left to the organization's
-- own import pipeline and cluster tooling, not something a backfill should
-- decide. Business key is "LEGACY-<identity_document.id>": stable, unique,
-- and traceable back to the source document.
WITH new_persons AS (
    INSERT INTO person (person_id)
    SELECT 'LEGACY-' || id
    FROM identity_document
    WHERE person_id IS NULL
    RETURNING id, person_id
)
UPDATE identity_document d
SET person_id = np.id, updated_at = NOW()
FROM new_persons np
WHERE d.person_id IS NULL
  AND np.person_id = 'LEGACY-' || d.id;

-- +goose Down
WITH legacy_persons AS (
    SELECT id, person_id FROM person WHERE person_id LIKE 'LEGACY-%'
)
UPDATE identity_document d
SET person_id = NULL, updated_at = NOW()
FROM legacy_persons lp
WHERE d.person_id = lp.id;

DELETE FROM person WHERE person_id LIKE 'LEGACY-%';
