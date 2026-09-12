-- +goose Up
ALTER TABLE criminal_cases ADD COLUMN case_type TEXT NOT NULL CHECK (case_type IN ('FACIAL', 'FINGERPRINT'));
