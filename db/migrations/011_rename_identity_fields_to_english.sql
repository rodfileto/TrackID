-- +goose Up
-- cpf and data_nascimento were named after Brazil/PF's own vocabulary even though this table is
-- meant to be generic (see MODEL.md section 2.1, which already documents identity_register's
-- "data_nascimento" column as generic "birthdate"). Renamed to English so any extension of
-- trackid, not just PF's, can use them: fiscal_number is any government-issued taxpayer/fiscal
-- identifier, not just Brazil's CPF.
ALTER TABLE identity_document RENAME COLUMN cpf TO fiscal_number;
ALTER TABLE identity_register RENAME COLUMN data_nascimento TO birth_date;

-- +goose Down
ALTER TABLE identity_document RENAME COLUMN fiscal_number TO cpf;
ALTER TABLE identity_register RENAME COLUMN birth_date TO data_nascimento;
