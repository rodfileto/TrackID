-- +goose Up
-- Crime de dano (Código Penal, art. 163): destroying, defacing or damaging property, as a bank agency's ATMs or
-- façade. ISP-RJ's classification, which the types follow (migration 003), has no category for it, so it is
-- grouped with the other types outside that classification. Before it, such an occurrence had to be UNDETERMINED,
-- or was forced into a theft or robbery.
INSERT INTO occurrence_types (code, case_type, grp) VALUES ('DAMAGE', 'CRIMINAL', 'OTHER');

-- +goose Down
-- Fails while an occurrence is of this type: reclassify it first.
DELETE FROM occurrence_types WHERE code = 'DAMAGE';
