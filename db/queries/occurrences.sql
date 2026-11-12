-- name: UpsertLocation :one
-- geom never crosses into Go: sqlc has no type for geography, so a point goes in and out as
-- lon/lat (WGS 84).
INSERT INTO locations (location_id, name, geom, precision, municipality_code, uf, neighbourhood, address, place_type,
    geocode_source, geocode_flags)
VALUES (
    sqlc.arg(location_id),
    sqlc.narg(name),
    ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography,
    sqlc.arg(precision),
    sqlc.arg(municipality_code),
    sqlc.arg(uf),
    sqlc.narg(neighbourhood),
    sqlc.narg(address),
    sqlc.narg(place_type),
    sqlc.narg(geocode_source),
    COALESCE(sqlc.arg(geocode_flags)::text[], '{}')
)
ON CONFLICT (location_id) DO UPDATE SET
    name = EXCLUDED.name,
    geom = EXCLUDED.geom,
    precision = EXCLUDED.precision,
    municipality_code = EXCLUDED.municipality_code,
    uf = EXCLUDED.uf,
    neighbourhood = EXCLUDED.neighbourhood,
    address = EXCLUDED.address,
    place_type = EXCLUDED.place_type,
    geocode_source = EXCLUDED.geocode_source,
    geocode_flags = EXCLUDED.geocode_flags,
    updated_at = NOW()
RETURNING id, (xmax = 0) AS inserted;

-- name: GetLocationByLocationID :one
SELECT id, location_id, name,
    ST_X(geom::geometry)::float8 AS lon,
    ST_Y(geom::geometry)::float8 AS lat,
    precision, municipality_code, uf, neighbourhood, address, place_type, geocode_source, geocode_flags
FROM locations
WHERE location_id = $1;

-- name: UpsertOccurrence :one
INSERT INTO occurrences (
    agency, occurrence_number, inquiry_number, location_id, occurred_from, occurred_to, reported_at,
    registered_at, attempted, offender_count, victim_count, status, description
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (agency, occurrence_number) DO UPDATE SET
    inquiry_number = EXCLUDED.inquiry_number,
    location_id = EXCLUDED.location_id,
    occurred_from = EXCLUDED.occurred_from,
    occurred_to = EXCLUDED.occurred_to,
    reported_at = EXCLUDED.reported_at,
    registered_at = EXCLUDED.registered_at,
    attempted = EXCLUDED.attempted,
    offender_count = EXCLUDED.offender_count,
    victim_count = EXCLUDED.victim_count,
    status = EXCLUDED.status,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id, (xmax = 0) AS inserted;

-- name: UpsertOccurrenceByInquiry :one
-- An imported occurrence with no police report, registered by its inquiry (IPL): keyed by the
-- inquiry number (occurrences_inquiry_registered_key).
INSERT INTO occurrences (
    inquiry_number, location_id, occurred_from, occurred_to, reported_at,
    registered_at, attempted, offender_count, victim_count, status, description
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (inquiry_number) WHERE occurrence_number IS NULL AND occurrence_seq IS NULL DO UPDATE SET
    location_id = EXCLUDED.location_id,
    occurred_from = EXCLUDED.occurred_from,
    occurred_to = EXCLUDED.occurred_to,
    reported_at = EXCLUDED.reported_at,
    registered_at = EXCLUDED.registered_at,
    attempted = EXCLUDED.attempted,
    offender_count = EXCLUDED.offender_count,
    victim_count = EXCLUDED.victim_count,
    status = EXCLUDED.status,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING id, (xmax = 0) AS inserted;

-- name: UpdateOccurrenceDetails :execrows
UPDATE occurrences SET occurred_from = sqlc.narg('occurred_from'), inquiry_number = sqlc.narg('inquiry_number'),
    description = @description::text, updated_at = NOW()
WHERE id = @id;

-- name: ListOccurrenceLinkedCaseTypes :many
-- The natures of the cases linked to an occurrence: its types must keep agreeing with them.
SELECT DISTINCT case_type FROM biometric_cases WHERE occurrence_id = $1;

-- name: DeleteOccurrenceClassifications :exec
-- With AddOccurrenceClassification, replaces an occurrence's types inside one transaction.
DELETE FROM occurrence_classifications WHERE occurrence_id = $1;

-- name: AddOccurrenceClassification :exec
INSERT INTO occurrence_classifications (occurrence_id, type_code)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListOccurrenceTypes :many
SELECT code, case_type, grp FROM occurrence_types ORDER BY code;

-- name: SetBiometricCaseOccurrence :execrows
-- A NULL occurrence_id unlinks the case.
UPDATE biometric_cases
SET occurrence_id = sqlc.narg(occurrence_id), updated_at = NOW()
WHERE case_id = sqlc.arg(case_id);

-- name: ListOccurrencesWithinRadius :many
-- Occurrences whose location lies within radius_m metres of a point, nearest first. Locations
-- known only to the municipality are left out unless include_municipality_centroids: their point
-- is a centroid, not a place.
SELECT o.id, o.agency, o.occurrence_number, o.occurred_from, o.status,
    l.location_id, l.precision,
    ST_X(l.geom::geometry)::float8 AS lon,
    ST_Y(l.geom::geometry)::float8 AS lat,
    ST_Distance(l.geom, ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography)::float8 AS distance_m
FROM occurrences o
JOIN locations l ON l.id = o.location_id
WHERE ST_DWithin(l.geom, ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography, sqlc.arg(radius_m)::float8)
  AND (sqlc.arg(include_municipality_centroids)::bool OR l.precision <> 'MUNICIPALITY')
  AND l.precision <> 'JURISDICTION'
ORDER BY distance_m, o.id;

-- name: GetOccurrenceIDByNumber :one
SELECT id FROM occurrences WHERE agency = $1 AND occurrence_number = $2;

-- name: GetOccurrenceIDByInquiry :one
-- An imported occurrence registered by its inquiry alone (UpsertOccurrenceByInquiry).
SELECT id FROM occurrences
WHERE inquiry_number = $1 AND occurrence_number IS NULL AND occurrence_seq IS NULL;

-- name: ListOccurrencesForMap :many
-- Occurrences with their point, types and linked cases, for the occurrence map. NULL bounds are
-- open. types is a JSON array of occurrence_types codes; cases a JSON array of
-- {caseId, modality, caseType}. A JURISDICTION location is no place, so its occurrence is not on the map.
SELECT o.id, o.agency, o.occurrence_number, o.occurrence_year, o.occurrence_seq,
    o.occurred_from, o.occurred_to, o.registered_at,
    o.attempted, o.status, o.description,
    ST_X(l.geom::geometry)::float8 AS lon,
    ST_Y(l.geom::geometry)::float8 AS lat,
    l.location_id, l.precision, l.place_type, l.name AS place_name, l.neighbourhood, l.municipality_code,
    l.geocode_source, l.geocode_flags,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types,
    COALESCE((SELECT json_agg(json_build_object('caseId', b.case_id, 'modality', b.modality, 'caseType', b.case_type)
                              ORDER BY b.case_id)
              FROM biometric_cases b WHERE b.occurrence_id = o.id), '[]')::jsonb AS cases
FROM occurrences o
JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE (sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after))
  AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before))
ORDER BY o.occurred_from NULLS LAST, o.id
LIMIT sqlc.arg(max_rows);

-- name: ListOccurrencesForMapByCases :many
-- ListOccurrencesForMap for the occurrences of some cases: where a person or a cluster appears, through the cases it has
-- traces in. An occurrence with no place (or only its unit's municipality) is not here: PresenceCounts counts it.
SELECT o.id, o.agency, o.occurrence_number, o.occurrence_year, o.occurrence_seq,
    o.occurred_from, o.occurred_to, o.registered_at,
    o.attempted, o.status, o.description,
    ST_X(l.geom::geometry)::float8 AS lon,
    ST_Y(l.geom::geometry)::float8 AS lat,
    l.location_id, l.precision, l.place_type, l.name AS place_name, l.neighbourhood, l.municipality_code,
    l.geocode_source, l.geocode_flags,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types,
    COALESCE((SELECT json_agg(json_build_object('caseId', b.case_id, 'modality', b.modality, 'caseType', b.case_type)
                              ORDER BY b.case_id)
              FROM biometric_cases b WHERE b.occurrence_id = o.id), '[]')::jsonb AS cases
FROM occurrences o
JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE o.id IN (SELECT b.occurrence_id FROM biometric_cases b WHERE b.case_id = ANY(@case_ids::text[]) AND b.occurrence_id IS NOT NULL)
ORDER BY o.occurred_from NULLS LAST, o.id;

-- name: PresenceCounts :one
-- What a set of cases gives a map: how many have an occurrence, how many distinct occurrences, and how many of those have
-- no place to draw (none, or only their unit's municipality).
SELECT COUNT(*)::bigint AS cases,
    COUNT(b.occurrence_id)::bigint AS with_occurrence,
    COUNT(DISTINCT b.occurrence_id)::bigint AS occurrences,
    COUNT(DISTINCT b.occurrence_id) FILTER (WHERE b.occurrence_id IS NOT NULL AND l.id IS NULL)::bigint AS unplaced
FROM biometric_cases b
LEFT JOIN occurrences o ON o.id = b.occurrence_id
LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE b.case_id = ANY(@case_ids::text[]);

-- name: ListOccurrenceYears :many
SELECT DISTINCT EXTRACT(YEAR FROM occurred_from AT TIME ZONE 'America/Sao_Paulo')::int4 AS year
FROM occurrences WHERE occurred_from IS NOT NULL ORDER BY year;

-- name: MaxOccurrenceSeqForYear :one
-- The highest TrackID sequence already used for a year, or 0.
SELECT COALESCE(MAX(occurrence_seq), 0)::int4 AS max_seq
FROM occurrences
WHERE occurrence_year = sqlc.arg(occurrence_year)::int4;

-- name: CreateOccurrence :one
-- An occurrence registered in TrackID: numbered by occurrences.Create, the police agency's own
-- number and the inquiry number optional.
INSERT INTO occurrences (
    occurrence_year, occurrence_seq, agency, occurrence_number, inquiry_number, location_id, occurred_from, occurred_to,
    reported_at, registered_at, attempted, offender_count, victim_count, status, description
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), $10, $11, $12, $13, $14)
RETURNING id, registered_at;

-- name: GetLocationByID :one
SELECT id, location_id FROM locations WHERE id = $1;

-- name: ListLocationsNear :many
-- Existing locations within radius_m metres of a point, nearest first, with how many occurrences
-- each holds: offered for reuse when registering an occurrence, so a repeat place stays one row.
SELECT l.id, l.location_id, l.name, l.address, l.neighbourhood, l.place_type, l.precision,
    ST_X(l.geom::geometry)::float8 AS lon,
    ST_Y(l.geom::geometry)::float8 AS lat,
    ST_Distance(l.geom, ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography)::float8 AS distance_m,
    (SELECT COUNT(*) FROM occurrences o WHERE o.location_id = l.id)::int4 AS occurrence_count
FROM locations l
WHERE ST_DWithin(l.geom, ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography, sqlc.arg(radius_m)::float8)
  AND l.precision <> 'JURISDICTION'
ORDER BY distance_m, l.id
LIMIT 10;

-- name: SearchOccurrences :many
-- Occurrences whose TrackID number ("2026.0001"), police number or inquiry number contains q,
-- newest first: the case page's link search.
SELECT o.id, o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number, o.inquiry_number, o.occurred_from, o.status,
    l.name AS place_name, l.neighbourhood,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types
FROM occurrences o
LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE (o.occurrence_year::text || '.' || lpad(o.occurrence_seq::text, 4, '0')) ILIKE '%' || sqlc.arg(q)::text || '%'
   OR o.occurrence_number ILIKE '%' || sqlc.arg(q)::text || '%'
   OR o.inquiry_number ILIKE '%' || sqlc.arg(q)::text || '%'
   OR o.agency ILIKE '%' || sqlc.arg(q)::text || '%'
ORDER BY o.occurred_from DESC NULLS LAST, o.id DESC
LIMIT 20;

-- name: GetOccurrenceSummary :one
-- One occurrence as a case page shows it, with its location's point for the card's map, when it
-- has a location (has_place).
SELECT o.id, o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number, o.inquiry_number, o.occurred_from, o.status,
    o.description,
    l.name AS place_name, l.neighbourhood, l.address, l.precision, l.geocode_source,
    COALESCE(l.geocode_flags, '{}')::text[] AS geocode_flags,
    (o.location_id IS NOT NULL)::bool AS has_place,
    COALESCE(ST_X(l.geom::geometry), 0)::float8 AS lon,
    COALESCE(ST_Y(l.geom::geometry), 0)::float8 AS lat,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types
FROM occurrences o
LEFT JOIN locations l ON l.id = o.location_id
WHERE o.id = $1;

-- name: ListOccurrenceCaseTypes :many
-- The case types (CRIMINAL, CIVIL) of an occurrence's types: a case links only to an occurrence
-- of its own nature.
SELECT DISTINCT t.case_type
FROM occurrence_classifications c
JOIN occurrence_types t ON t.code = c.type_code
WHERE c.occurrence_id = $1;

-- name: GetCaseOccurrenceID :one
SELECT id, case_type, occurrence_id FROM biometric_cases WHERE case_id = $1;

-- name: UpsertMunicipality :exec
-- geojson is the boundary as a GeoJSON MultiPolygon (or Polygon) in WGS 84.
INSERT INTO municipalities (code, name, uf, geom, source)
VALUES (
    sqlc.arg(code), sqlc.arg(name), sqlc.arg(uf),
    ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(geojson)::text), 4326))::geography,
    sqlc.arg(source)
)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name, uf = EXCLUDED.uf, geom = EXCLUDED.geom, source = EXCLUDED.source, loaded_at = NOW();

-- name: GetMunicipalityAt :one
-- The municipality whose boundary covers a point.
SELECT code, name, uf FROM municipalities
WHERE ST_Covers(geom, ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography)
LIMIT 1;

-- name: CountMunicipalities :one
SELECT COUNT(*) FROM municipalities;

-- name: SetOccurrenceLocation :execrows
UPDATE occurrences SET location_id = sqlc.narg('location_id')::bigint, updated_at = NOW() WHERE id = @id;
