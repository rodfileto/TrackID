-- name: ListOrganizations :many
SELECT id, acronym, name, kind, uf, active,
    (SELECT COUNT(*) FROM units WHERE units.organization_id = organizations.id)::int AS unit_count
FROM organizations ORDER BY kind, uf NULLS FIRST, acronym;

-- name: CreateOrganization :one
INSERT INTO organizations (acronym, name, kind, uf) VALUES ($1, $2, $3, sqlc.narg(uf))
RETURNING id;

-- name: UpdateOrganization :execrows
UPDATE organizations SET acronym = $2, name = $3, kind = $4, uf = sqlc.narg(uf), active = $5, updated_at = NOW()
WHERE id = $1;

-- name: CountOrganizationUnits :one
SELECT COUNT(*)::int FROM units WHERE organization_id = $1;

-- name: DeleteOrganizationRoot :exec
DELETE FROM units WHERE organization_id = $1 AND parent_id IS NULL;

-- name: DeleteOrganization :execrows
DELETE FROM organizations WHERE id = $1;

-- name: ListUnits :many
SELECT u.id, u.organization_id, u.parent_id, u.acronym, u.name, u.uf, u.municipality_code, u.receives_new_cases,
    u.active, u.laudo_header, u.laudo_name,
    (SELECT COUNT(*) FROM unit_assignments a WHERE a.unit_id = u.id
        AND a.starts_on <= CURRENT_DATE AND (a.ends_on IS NULL OR a.ends_on >= CURRENT_DATE))::int AS current_members
FROM units u ORDER BY u.organization_id, u.acronym;

-- name: GetUnit :one
SELECT id, organization_id, parent_id, acronym, name, uf, municipality_code, receives_new_cases, active,
    laudo_header, laudo_name
FROM units WHERE id = $1;

-- name: CreateUnit :one
INSERT INTO units (organization_id, parent_id, acronym, name, uf, municipality_code, laudo_header, laudo_name)
VALUES ($1, $2, $3, $4, sqlc.narg(uf), sqlc.narg(municipality_code), @laudo_header::text[], $5)
RETURNING id;

-- name: CreateRootUnit :exec
INSERT INTO units (organization_id, acronym, name, uf) VALUES ($1, $2, $3, sqlc.narg(uf));

-- name: UpdateUnit :execrows
UPDATE units SET parent_id = sqlc.narg(parent_id), acronym = $2, name = $3, uf = sqlc.narg(uf),
    municipality_code = sqlc.narg(municipality_code), laudo_header = @laudo_header::text[], laudo_name = $4,
    active = $5, updated_at = NOW()
WHERE id = $1;

-- name: UnitAncestors :many
-- The unit and the units above it, nearest first.
WITH RECURSIVE line AS (
    SELECT u.id, u.parent_id, u.laudo_header, 0 AS depth FROM units u WHERE u.id = $1
    UNION ALL
    SELECT p.id, p.parent_id, p.laudo_header, line.depth + 1 FROM units p JOIN line ON p.id = line.parent_id
    WHERE line.depth < 64
)
SELECT line.id, line.laudo_header FROM line ORDER BY line.depth;

-- name: ClearCaseReceiver :exec
UPDATE units SET receives_new_cases = FALSE, updated_at = NOW() WHERE receives_new_cases AND id <> $1;

-- name: SetCaseReceiver :exec
UPDATE units SET receives_new_cases = TRUE, updated_at = NOW() WHERE id = $1;

-- name: DeleteUnit :execrows
DELETE FROM units WHERE id = $1 AND parent_id IS NOT NULL;

-- name: ListUnitAssignments :many
SELECT a.id, a.user_id, a.unit_id, a.kind, a.starts_on, a.ends_on, a.note, a.created_at
FROM unit_assignments a ORDER BY a.user_id, a.starts_on DESC, a.id DESC;

-- name: GetUnitAssignment :one
SELECT id, user_id, unit_id, kind, starts_on, ends_on, note FROM unit_assignments WHERE id = $1;

-- name: EndOpenPosting :exec
-- A new posting ends the open one it follows: a transfer.
UPDATE unit_assignments SET ends_on = sqlc.arg(starts_on)::date - 1, updated_at = NOW()
WHERE user_id = sqlc.arg(user_id) AND kind = 'POSTING' AND ends_on IS NULL AND starts_on < sqlc.arg(starts_on)::date;

-- name: CreateUnitAssignment :one
INSERT INTO unit_assignments (user_id, unit_id, kind, starts_on, ends_on, note, created_by)
VALUES ($1, $2, $3, $4, sqlc.narg(ends_on), $5, sqlc.narg(created_by))
RETURNING id;

-- name: UpdateUnitAssignment :execrows
UPDATE unit_assignments SET unit_id = $2, starts_on = $3, ends_on = sqlc.narg(ends_on), note = $4, updated_at = NOW()
WHERE id = $1;

-- name: DeleteUnitAssignment :execrows
DELETE FROM unit_assignments WHERE id = $1;

-- name: GetUnitOnDate :one
-- Where a user worked on a day: on mission there, or else posted.
SELECT a.kind, u.id AS unit_id, u.acronym, u.name, u.laudo_name, o.name AS organization_name
FROM unit_assignments a
JOIN units u ON u.id = a.unit_id
JOIN organizations o ON o.id = u.organization_id
WHERE a.user_id = $1 AND a.starts_on <= @day::date AND (a.ends_on IS NULL OR a.ends_on >= @day::date)
ORDER BY a.kind = 'MISSION' DESC
LIMIT 1;
