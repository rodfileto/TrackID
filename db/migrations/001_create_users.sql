-- +goose Up
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    nome TEXT NOT NULL,
    ultimo_nome TEXT NOT NULL,
    matricula TEXT NOT NULL UNIQUE,
    cargo TEXT NOT NULL,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
