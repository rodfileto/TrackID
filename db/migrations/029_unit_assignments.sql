-- +goose Up
-- Where each user works (docs/organizations-plan.md, phase 1), and what a laudo says of a unit.
--
-- A user is posted to one unit at a time (lotação, POSTING) and may be sent on mission to another for a
-- while (missão, MISSION). Both are dated, so a laudo names the unit its examiner worked in on its date,
-- and a change of posting never rewrites what was issued before it. A mission covering a day wins over the
-- posting. ends_on is the last day, inclusive; NULL is open-ended. Two postings, or two missions, of one
-- user never overlap.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE unit_assignments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    unit_id BIGINT NOT NULL REFERENCES units(id),
    kind TEXT NOT NULL CHECK (kind IN ('POSTING', 'MISSION')),
    starts_on DATE NOT NULL,
    ends_on DATE,
    note TEXT NOT NULL DEFAULT '', -- the act behind it, such as a portaria or an ordem de missão
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unit_assignments_dates_check CHECK (ends_on IS NULL OR ends_on >= starts_on),
    CONSTRAINT unit_assignments_no_overlap EXCLUDE USING gist (
        user_id WITH =, kind WITH =, daterange(starts_on, ends_on, '[]') WITH &&)
);
CREATE INDEX unit_assignments_unit_id_idx ON unit_assignments (unit_id);

-- A laudo's header lines, such as {POLÍCIA FEDERAL, Superintendência Regional em Minas Gerais}: a unit with
-- none takes its nearest ancestor's, and with none in its line the organization's name. laudo_name is how a
-- laudo's preâmbulo names the unit, such as "Núcleo de Identificação da Delegacia Regional Executiva no
-- estado de Minas Gerais"; empty, it is the unit's name.
ALTER TABLE units
    ADD COLUMN laudo_header TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN laudo_name TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE units DROP COLUMN laudo_name, DROP COLUMN laudo_header;
DROP TABLE unit_assignments;
