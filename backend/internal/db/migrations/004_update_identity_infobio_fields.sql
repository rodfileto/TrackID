-- +goose Up
ALTER TABLE identity_document ADD COLUMN cpf TEXT;
ALTER TABLE identity_document ADD CONSTRAINT identity_document_type_number_key UNIQUE (document_type, document_number);

ALTER TABLE identity_register DROP COLUMN register_id;
ALTER TABLE identity_register ADD COLUMN numero_identificacao TEXT;
ALTER TABLE identity_register ADD COLUMN data_nascimento TEXT;
ALTER TABLE identity_register ADD COLUMN nist_path TEXT;
ALTER TABLE identity_register ADD COLUMN storage_ref TEXT;
ALTER TABLE identity_register ADD COLUMN meta JSONB;
ALTER TABLE identity_register ADD CONSTRAINT identity_register_register_number_key UNIQUE (register_number);
