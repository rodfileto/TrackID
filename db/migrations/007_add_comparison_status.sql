-- +goose Up
ALTER TABLE comparisons ADD COLUMN status TEXT NOT NULL DEFAULT 'confirmed'
    CHECK (status IN ('confirmed', 'rejected'));
