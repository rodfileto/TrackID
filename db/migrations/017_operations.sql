-- +goose Up
-- Intelligence operations (docs/operations-plan.md): a named line of work. The slug is stable, for the
-- page's address and for what an organization attaches to the operation.
CREATE TABLE operations (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name TEXT NOT NULL CHECK (name <> ''),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CLOSED')),
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    CONSTRAINT operations_closed_check CHECK ((status = 'CLOSED') = (closed_at IS NOT NULL))
);

-- +goose Down
DROP TABLE operations;
