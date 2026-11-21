-- +goose Up
-- Cumprimento de mandado de prisão: an arrest made on an open warrant, such as the BNMP's, when a face recognition
-- system sighted its target and the police found them. It isn't a crime of its own but the event an operation records,
-- so it is grouped with the types outside ISP-RJ's classification (migration 003). Asked for by trackid-pf's Mitra
-- operation, which records one when its operational team reports a warrant served.
INSERT INTO occurrence_types (code, case_type, grp) VALUES ('ARREST_WARRANT_SERVED', 'CRIMINAL', 'OTHER');

-- +goose Down
-- Fails while an occurrence is of this type: reclassify it first.
DELETE FROM occurrence_types WHERE code = 'ARREST_WARRANT_SERVED';
