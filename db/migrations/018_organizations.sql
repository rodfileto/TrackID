-- +goose Up
-- Organizations and units (docs/organizations-plan.md, phase 1): who owns a case. Nothing here limits
-- access yet. An organization is the legal body; its units form a tree with one root each.
CREATE TABLE organizations (
    id BIGSERIAL PRIMARY KEY,
    acronym TEXT NOT NULL UNIQUE CHECK (acronym <> ''),
    name TEXT NOT NULL CHECK (name <> ''),
    kind TEXT NOT NULL CHECK (kind IN ('FEDERAL', 'STATE')),
    uf CHAR(2), -- the state of a state's organization
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT organizations_uf_check CHECK ((kind = 'STATE') = (uf IS NOT NULL))
);

-- A unit's place is optional: one without it is where its parent is, as a sector within a building.
-- municipality_code is IBGE's, with no foreign key for the same reason as locations': a deployment may
-- load only its own state's municipalities. receives_new_cases names the one unit a case goes to when
-- whoever creates it names none.
CREATE TABLE units (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    parent_id BIGINT,
    acronym TEXT NOT NULL CHECK (acronym <> ''),
    name TEXT NOT NULL CHECK (name <> ''),
    uf CHAR(2),
    municipality_code CHAR(7) CHECK (municipality_code ~ '^[0-9]{7}$'),
    receives_new_cases BOOLEAN NOT NULL DEFAULT FALSE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT units_organization_acronym_key UNIQUE (organization_id, acronym),
    CONSTRAINT units_organization_id_key UNIQUE (organization_id, id),
    -- A parent is always of the same organization.
    CONSTRAINT units_parent_fkey FOREIGN KEY (organization_id, parent_id) REFERENCES units(organization_id, id),
    CONSTRAINT units_not_own_parent CHECK (parent_id <> id),
    CONSTRAINT units_municipality_uf_check CHECK (municipality_code IS NULL OR uf IS NOT NULL)
);
CREATE UNIQUE INDEX units_one_root_idx ON units (organization_id) WHERE parent_id IS NULL;
CREATE UNIQUE INDEX units_one_receiver_idx ON units (receives_new_cases) WHERE receives_new_cases;
CREATE INDEX units_parent_idx ON units (parent_id);

-- The installation as it was: one organization with one unit, which a deployment renames.
INSERT INTO organizations (acronym, name, kind) VALUES ('DEFAULT', 'Instituição', 'FEDERAL');
INSERT INTO units (organization_id, acronym, name, receives_new_cases)
SELECT id, acronym, name, TRUE FROM organizations;

-- +goose StatementBegin
CREATE FUNCTION default_case_unit() RETURNS BIGINT LANGUAGE sql STABLE AS $$
    SELECT id FROM units WHERE receives_new_cases
$$;
-- +goose StatementEnd

-- The unit responsible for the case; its organization is the unit's.
ALTER TABLE biometric_cases
    ADD COLUMN unit_id BIGINT NOT NULL DEFAULT default_case_unit() REFERENCES units(id);
CREATE INDEX biometric_cases_unit_id_idx ON biometric_cases (unit_id);

-- The unit that asked, when it is one we know. It need not own the cases the request opens, and it can
-- be of another organization. authority stays the name as the document gives it.
ALTER TABLE case_requests ADD COLUMN requesting_unit_id BIGINT REFERENCES units(id);
CREATE INDEX case_requests_requesting_unit_id_idx ON case_requests (requesting_unit_id);

-- +goose Down
ALTER TABLE case_requests DROP COLUMN requesting_unit_id;
ALTER TABLE biometric_cases DROP COLUMN unit_id;
DROP FUNCTION default_case_unit();
DROP TABLE units;
DROP TABLE organizations;
