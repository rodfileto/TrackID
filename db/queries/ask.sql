-- The AI mode's read-only queries (ask package, docs/ask-plan.md). The model never writes SQL: it
-- chooses a tool, and a tool's filters reach these parameterized queries. An empty array or a NULL
-- leaves a filter off. occurred_after is inclusive and occurred_before exclusive; a period counts
-- by occurred_from, the start of an occurrence's window. A location of precision JURISDICTION is the investigating
-- unit's municipality, not a place (migration 030): every join to locations leaves it out, so its occurrence counts as
-- one with no place.

-- name: AskOccurrenceSummary :one
-- How many occurrences match, and how many a period or a place filter left out only because the
-- occurrence has no date or no place: an answer has to be able to say so.
WITH base AS (
    SELECT
        ((sqlc.narg(occurred_after)::timestamptz IS NULL AND sqlc.narg(occurred_before)::timestamptz IS NULL)
         OR COALESCE((sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after)::timestamptz)
             AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before)::timestamptz), FALSE))::bool AS date_ok,
        COALESCE((sqlc.narg(uf)::text IS NULL OR l.uf = sqlc.narg(uf)::text)
            AND (sqlc.narg(municipality_code)::text IS NULL OR l.municipality_code = sqlc.narg(municipality_code)::text)
            AND (sqlc.narg(neighbourhood)::text IS NULL OR lower(l.neighbourhood) = lower(sqlc.narg(neighbourhood)::text))
            AND (cardinality(@place_types::text[]) = 0 OR l.place_type = ANY(@place_types::text[])), FALSE)::bool AS place_ok,
        (o.occurred_from IS NULL)::bool AS undated,
        (l.id IS NULL)::bool AS unplaced
    FROM occurrences o
    LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
    WHERE (cardinality(@types::text[]) = 0 OR EXISTS (
            SELECT 1 FROM occurrence_classifications c WHERE c.occurrence_id = o.id AND c.type_code = ANY(@types::text[])))
      AND (cardinality(@groups::text[]) = 0 OR EXISTS (
            SELECT 1 FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
            WHERE c.occurrence_id = o.id AND t.grp = ANY(@groups::text[])))
      AND (cardinality(@statuses::text[]) = 0 OR o.status = ANY(@statuses::text[]))
      AND (sqlc.narg(attempted)::bool IS NULL OR o.attempted = sqlc.narg(attempted)::bool)
      AND (sqlc.narg(agency)::text IS NULL OR lower(o.agency) = lower(sqlc.narg(agency)::text))
)
SELECT
    COUNT(*) FILTER (WHERE date_ok AND place_ok)::bigint AS matched,
    COUNT(*) FILTER (WHERE undated AND NOT date_ok AND place_ok)::bigint AS undated_excluded,
    COUNT(*) FILTER (WHERE unplaced AND NOT place_ok AND date_ok)::bigint AS unplaced_excluded
FROM base;

-- name: AskOccurrenceBreakdown :many
-- Distinct matching occurrences per value of a dimension, most first (month and year in time
-- order). By type or group an occurrence with several types counts once under each, so these rows
-- can add up to more than the total. An empty key means the occurrence has no such value.
WITH matched AS (
    SELECT o.id, o.occurred_from, o.status, o.agency, l.uf, l.municipality_code, l.neighbourhood, l.place_type
    FROM occurrences o
    LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
    WHERE (cardinality(@types::text[]) = 0 OR EXISTS (
            SELECT 1 FROM occurrence_classifications c WHERE c.occurrence_id = o.id AND c.type_code = ANY(@types::text[])))
      AND (cardinality(@groups::text[]) = 0 OR EXISTS (
            SELECT 1 FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
            WHERE c.occurrence_id = o.id AND t.grp = ANY(@groups::text[])))
      AND (cardinality(@statuses::text[]) = 0 OR o.status = ANY(@statuses::text[]))
      AND (sqlc.narg(attempted)::bool IS NULL OR o.attempted = sqlc.narg(attempted)::bool)
      AND (sqlc.narg(agency)::text IS NULL OR lower(o.agency) = lower(sqlc.narg(agency)::text))
      AND (sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after)::timestamptz)
      AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before)::timestamptz)
      AND (sqlc.narg(uf)::text IS NULL OR l.uf = sqlc.narg(uf)::text)
      AND (sqlc.narg(municipality_code)::text IS NULL OR l.municipality_code = sqlc.narg(municipality_code)::text)
      AND (sqlc.narg(neighbourhood)::text IS NULL OR lower(l.neighbourhood) = lower(sqlc.narg(neighbourhood)::text))
      AND (cardinality(@place_types::text[]) = 0 OR l.place_type = ANY(@place_types::text[]))
)
SELECT k.key::text AS key, COUNT(DISTINCT m.id)::bigint AS occurrences
FROM matched m
CROSS JOIN LATERAL (
    SELECT COALESCE(CASE @dimension::text
        WHEN 'month' THEN to_char(m.occurred_from AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM')
        WHEN 'year' THEN to_char(m.occurred_from AT TIME ZONE 'America/Sao_Paulo', 'YYYY')
        WHEN 'status' THEN m.status
        WHEN 'agency' THEN m.agency
        WHEN 'uf' THEN m.uf::text
        WHEN 'municipality' THEN COALESCE((SELECT mu.name || '/' || mu.uf FROM municipalities mu WHERE mu.code = m.municipality_code), m.municipality_code::text)
        WHEN 'neighbourhood' THEN m.neighbourhood
        WHEN 'place_type' THEN m.place_type
    END, '') AS key
    WHERE @dimension::text NOT IN ('type', 'group')
    UNION ALL
    SELECT c.type_code FROM occurrence_classifications c
    WHERE @dimension::text = 'type' AND c.occurrence_id = m.id
    UNION ALL
    SELECT DISTINCT t.grp FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
    WHERE @dimension::text = 'group' AND c.occurrence_id = m.id
) k
GROUP BY k.key
ORDER BY CASE WHEN @dimension::text IN ('month', 'year') THEN k.key END, occurrences DESC, k.key
LIMIT @max_rows;

-- name: AskListOccurrences :many
-- Matching occurrences, newest first, each with its types and linked cases.
SELECT o.id, o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number, o.inquiry_number,
    o.occurred_from, o.attempted, o.status, o.description,
    l.name AS place_name, l.neighbourhood, l.municipality_code, l.uf, l.place_type,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types,
    COALESCE((SELECT json_agg(b.case_id ORDER BY b.case_id) FROM biometric_cases b
              WHERE b.occurrence_id = o.id), '[]')::jsonb AS cases
FROM occurrences o
LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE (cardinality(@types::text[]) = 0 OR EXISTS (
        SELECT 1 FROM occurrence_classifications c WHERE c.occurrence_id = o.id AND c.type_code = ANY(@types::text[])))
  AND (cardinality(@groups::text[]) = 0 OR EXISTS (
        SELECT 1 FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
        WHERE c.occurrence_id = o.id AND t.grp = ANY(@groups::text[])))
  AND (cardinality(@statuses::text[]) = 0 OR o.status = ANY(@statuses::text[]))
  AND (sqlc.narg(attempted)::bool IS NULL OR o.attempted = sqlc.narg(attempted)::bool)
  AND (sqlc.narg(agency)::text IS NULL OR lower(o.agency) = lower(sqlc.narg(agency)::text))
  AND (sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after)::timestamptz)
  AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before)::timestamptz)
  AND (sqlc.narg(uf)::text IS NULL OR l.uf = sqlc.narg(uf)::text)
  AND (sqlc.narg(municipality_code)::text IS NULL OR l.municipality_code = sqlc.narg(municipality_code)::text)
  AND (sqlc.narg(neighbourhood)::text IS NULL OR lower(l.neighbourhood) = lower(sqlc.narg(neighbourhood)::text))
  AND (cardinality(@place_types::text[]) = 0 OR l.place_type = ANY(@place_types::text[]))
ORDER BY o.occurred_from DESC NULLS LAST, o.id DESC
LIMIT @max_rows;

-- name: AskCountCases :many
-- Biometric cases per value of a dimension, filtered by the case itself and, for a case linked to
-- one, by its occurrence. Any occurrence filter leaves out cases with no occurrence.
WITH matched AS (
    SELECT b.id, b.case_type, b.modality, b.occurrence_id
    FROM biometric_cases b
    LEFT JOIN occurrences o ON o.id = b.occurrence_id
    WHERE (sqlc.narg(modality)::text IS NULL OR b.modality = sqlc.narg(modality)::text)
      AND (sqlc.narg(case_type)::text IS NULL OR b.case_type = sqlc.narg(case_type)::text)
      AND (cardinality(@types::text[]) = 0 OR EXISTS (
            SELECT 1 FROM occurrence_classifications c WHERE c.occurrence_id = o.id AND c.type_code = ANY(@types::text[])))
      AND (cardinality(@groups::text[]) = 0 OR EXISTS (
            SELECT 1 FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
            WHERE c.occurrence_id = o.id AND t.grp = ANY(@groups::text[])))
      AND (sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after)::timestamptz)
      AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before)::timestamptz)
)
SELECT k.key::text AS key, COUNT(DISTINCT m.id)::bigint AS cases
FROM matched m
CROSS JOIN LATERAL (
    SELECT CASE @dimension::text
        WHEN 'modality' THEN m.modality
        WHEN 'case_type' THEN m.case_type
        WHEN 'linked' THEN CASE WHEN m.occurrence_id IS NULL THEN 'NOT_LINKED' ELSE 'LINKED' END
        ELSE 'ALL'
    END AS key
    WHERE @dimension::text NOT IN ('type', 'group')
    UNION ALL
    SELECT c.type_code FROM occurrence_classifications c
    WHERE @dimension::text = 'type' AND c.occurrence_id = m.occurrence_id
    UNION ALL
    SELECT DISTINCT t.grp FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
    WHERE @dimension::text = 'group' AND c.occurrence_id = m.occurrence_id
) k
GROUP BY k.key
ORDER BY cases DESC, k.key
LIMIT @max_rows;

-- name: AskDataCoverage :one
-- What the database holds: an answer about "all the data" has to know its limits.
SELECT
    (SELECT COUNT(*) FROM occurrences)::bigint AS occurrences,
    (SELECT COUNT(*) FROM occurrences WHERE occurred_from IS NULL)::bigint AS undated,
    (SELECT COUNT(*) FROM occurrences WHERE location_id IS NULL)::bigint AS unplaced,
    (SELECT COUNT(*) FROM persons)::bigint AS persons,
    (SELECT COUNT(*) FROM biometric_cases)::bigint AS cases,
    (SELECT COUNT(*) FROM biometric_cases WHERE occurrence_id IS NOT NULL)::bigint AS linked_cases,
    COALESCE((SELECT to_char(MIN(occurred_from) AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM-DD') FROM occurrences), '')::text AS first_occurred,
    COALESCE((SELECT to_char(MAX(occurred_from) AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM-DD') FROM occurrences), '')::text AS last_occurred,
    (SELECT COUNT(*) FROM municipalities)::bigint AS municipalities_loaded;

-- name: AskFindMunicipalities :many
-- Municipalities whose name contains q, ignoring case and accents, with how many occurrences each
-- holds. When the deployment has loaded no boundaries this finds nothing.
SELECT mu.code::text AS code, mu.name, mu.uf::text AS uf,
    (SELECT COUNT(*) FROM occurrences o JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
     WHERE l.municipality_code = mu.code)::bigint AS occurrences
FROM municipalities mu
WHERE translate(lower(mu.name), 'áàâãéêíóôõúüç', 'aaaaeeiooouuc')
    LIKE '%' || translate(lower(replace(replace(replace(@q::text, '\', '\\'), '%', '\%'), '_', '\_')), 'áàâãéêíóôõúüç', 'aaaaeeiooouuc') || '%'
ORDER BY occurrences DESC, mu.name
LIMIT 20;

-- name: AskHasNonSyntheticData :one
-- Whether anything a tool can return is not synthetic. trackid-sim starts every occurrence's and
-- case's description with SIMULATED and every person's id with SIM-; a model on an outside server
-- only ever reads such data.
SELECT (EXISTS (SELECT 1 FROM occurrences WHERE description NOT LIKE 'SIMULATED%')
     OR EXISTS (SELECT 1 FROM biometric_cases WHERE description NOT LIKE 'SIMULATED%')
     OR EXISTS (SELECT 1 FROM persons WHERE person_id NOT LIKE 'SIM-%'))::bool AS found;

-- name: InsertAskQuery :one
INSERT INTO ask_queries (user_id, username, endpoint, model, external, question, history, focus, steps, answer,
    unverified_numbers, error, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- The walk from a person to their cases, occurrences and inquiries, and back (ask/walk.go). A person
-- reaches a case through a cluster holding a KNOWN feature of theirs and a QUESTIONED trace of the case,
-- as in CountPersonCasesByModality. Both cluster layers count, and reviewed says which: true when people
-- confirmed the link, false when it is only the matcher's.

-- name: AskSearchPersons :many
-- A person with several documents or registers comes back once per matching register. q is matched
-- literally: a % in it is a percent sign.
SELECT p.id AS person_number, r.name, r.birth_date, r.register_number, d.document_type, d.document_number
FROM identity_registers r
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE r.name ILIKE '%' || replace(replace(replace(@q::text, '\', '\\'), '%', '\%'), '_', '\_') || '%'
ORDER BY r.name, p.id, r.id
LIMIT @max_rows;

-- name: AskPersonRegisters :many
SELECT p.person_id, p.id AS person_number, r.name, r.birth_date, r.register_number, d.document_type, d.document_number
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
WHERE p.id = @person_number
ORDER BY r.id;

-- name: AskPersonCaseCounts :many
-- For each person, how many cases any link reaches and how many a reviewed link does.
SELECT p.id AS person_number, COUNT(DISTINCT bc.id)::bigint AS cases,
    (COUNT(DISTINCT bc.id) FILTER (WHERE known_cm.basis = 'REVIEWED'))::bigint AS reviewed_cases
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_files f ON f.register_id = r.id
JOIN cluster_members known_cm ON known_cm.feature_id = f.id::text
JOIN cluster_members quest_cm ON quest_cm.cluster_id = known_cm.cluster_id
JOIN case_traces ct ON quest_cm.feature_id = 'TRACE:' || ct.id || '#feature'
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE p.id = ANY(@person_numbers::bigint[])
GROUP BY p.id;

-- name: AskPersonCaseLinks :many
SELECT bc.case_id, bc.case_type, bc.modality, bc.occurrence_id, BOOL_OR(known_cm.basis = 'REVIEWED')::bool AS reviewed,
    json_agg(DISTINCT known_cm.cluster_id)::jsonb AS cluster_ids
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_files f ON f.register_id = r.id
JOIN cluster_members known_cm ON known_cm.feature_id = f.id::text
JOIN cluster_members quest_cm ON quest_cm.cluster_id = known_cm.cluster_id
JOIN case_traces ct ON quest_cm.feature_id = 'TRACE:' || ct.id || '#feature'
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE p.id = @person_number
GROUP BY bc.id
ORDER BY reviewed DESC, bc.case_id
LIMIT @max_rows;

-- name: AskCasePersonLinks :many
SELECT bc.case_id, p.id AS person_number, BOOL_OR(quest_cm.basis = 'REVIEWED')::bool AS reviewed,
    json_agg(DISTINCT ct.id)::jsonb AS trace_ids,
    json_agg(DISTINCT quest_cm.cluster_id)::jsonb AS cluster_ids,
    COALESCE((SELECT json_agg(DISTINCT r2.name) FROM identity_registers r2 JOIN identity_documents d2 ON d2.id = r2.document_id
              WHERE d2.person_id = p.id), '[]')::jsonb AS names,
    COALESCE((SELECT MIN(r3.birth_date) FROM identity_registers r3 JOIN identity_documents d3 ON d3.id = r3.document_id
              WHERE d3.person_id = p.id), '')::text AS birth_date
FROM biometric_cases bc
JOIN case_evidences ce ON ce.biometric_case_id = bc.id
JOIN case_traces ct ON ct.evidence_id = ce.id
JOIN cluster_members quest_cm ON quest_cm.feature_id = 'TRACE:' || ct.id || '#feature'
JOIN cluster_members known_cm ON known_cm.cluster_id = quest_cm.cluster_id
JOIN identity_files f ON known_cm.feature_id = f.id::text
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE bc.case_id = ANY(@case_ids::text[])
GROUP BY bc.case_id, p.id
ORDER BY bc.case_id, reviewed DESC, p.id;

-- name: AskCaseDetail :one
SELECT id, case_id, case_type, modality, description, occurrence_id FROM biometric_cases WHERE case_id = $1;

-- name: AskCasesOfOccurrences :many
SELECT case_id, case_type, modality, occurrence_id FROM biometric_cases
WHERE occurrence_id = ANY(@occurrence_ids::bigint[])
ORDER BY case_id;

-- name: AskOccurrenceDetails :many
-- Occurrences with what is known of when and where. Without a place, has_place is false.
SELECT o.id, o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number, o.inquiry_number,
    o.occurred_from, o.occurred_to, o.reported_at, o.registered_at, o.attempted, o.offender_count, o.victim_count,
    o.status, o.description,
    (l.id IS NOT NULL)::bool AS has_place,
    l.name AS place_name, l.address, l.neighbourhood, l.municipality_code, l.uf, l.place_type, l.precision,
    COALESCE(ST_X(l.geom::geometry), 0)::float8 AS lon,
    COALESCE(ST_Y(l.geom::geometry), 0)::float8 AS lat,
    mu.name AS municipality,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types
FROM occurrences o
LEFT JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
LEFT JOIN municipalities mu ON mu.code = l.municipality_code
WHERE o.id = ANY(@ids::bigint[])
ORDER BY o.occurred_from DESC NULLS LAST, o.id DESC;

-- name: AskOccurrenceIDsByInquiry :many
SELECT id FROM occurrences WHERE lower(inquiry_number) = lower(@inquiry_number::text) ORDER BY id LIMIT @max_rows;

-- name: AskOccurrencesNear :many
-- Occurrences within radius_m of a point, nearest first. A point known only to the municipality is
-- that municipality's centre, not a place, so it is left out (see AskOccurrencesNearLeftOut).
SELECT o.id, o.occurrence_year, o.occurrence_seq, o.agency, o.occurrence_number, o.inquiry_number,
    o.occurred_from, o.attempted, o.status,
    l.name AS place_name, l.neighbourhood, l.place_type, l.precision,
    ST_X(l.geom::geometry)::float8 AS lon,
    ST_Y(l.geom::geometry)::float8 AS lat,
    ST_Distance(l.geom, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography)::float8 AS distance_m,
    COALESCE((SELECT json_agg(c.type_code ORDER BY c.type_code) FROM occurrence_classifications c
              WHERE c.occurrence_id = o.id), '[]')::jsonb AS types,
    (SELECT COUNT(*) FROM biometric_cases b WHERE b.occurrence_id = o.id)::bigint AS linked_cases
FROM occurrences o
JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE ST_DWithin(l.geom, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography, @radius_m::float8)
  AND l.precision <> 'MUNICIPALITY'
  AND o.id <> @except_id::bigint
  AND (cardinality(@types::text[]) = 0 OR EXISTS (
        SELECT 1 FROM occurrence_classifications c WHERE c.occurrence_id = o.id AND c.type_code = ANY(@types::text[])))
  AND (cardinality(@groups::text[]) = 0 OR EXISTS (
        SELECT 1 FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
        WHERE c.occurrence_id = o.id AND t.grp = ANY(@groups::text[])))
  AND (sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after)::timestamptz)
  AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before)::timestamptz)
ORDER BY distance_m, o.id
LIMIT @max_rows;

-- name: AskOccurrencesNearLeftOut :one
-- What AskOccurrencesNear left out with the same filters: points known only to the municipality whose
-- centre falls in the radius, and, with a period, occurrences in the radius with no date.
SELECT
    COUNT(*) FILTER (WHERE l.precision = 'MUNICIPALITY'
        AND (sqlc.narg(occurred_after)::timestamptz IS NULL OR o.occurred_from >= sqlc.narg(occurred_after)::timestamptz)
        AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR o.occurred_from < sqlc.narg(occurred_before)::timestamptz))::bigint AS municipality_only,
    COUNT(*) FILTER (WHERE l.precision <> 'MUNICIPALITY' AND o.occurred_from IS NULL
        AND (sqlc.narg(occurred_after)::timestamptz IS NOT NULL OR sqlc.narg(occurred_before)::timestamptz IS NOT NULL))::bigint AS undated
FROM occurrences o
JOIN locations l ON l.id = o.location_id AND l.precision <> 'JURISDICTION'
WHERE ST_DWithin(l.geom, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography, @radius_m::float8)
  AND o.id <> @except_id::bigint
  AND (cardinality(@types::text[]) = 0 OR EXISTS (
        SELECT 1 FROM occurrence_classifications c WHERE c.occurrence_id = o.id AND c.type_code = ANY(@types::text[])))
  AND (cardinality(@groups::text[]) = 0 OR EXISTS (
        SELECT 1 FROM occurrence_classifications c JOIN occurrence_types t ON t.code = c.type_code
        WHERE c.occurrence_id = o.id AND t.grp = ANY(@groups::text[])));

-- name: AskPersonIdentityClusters :many
-- Other people whose enrollment shares a cluster with this person's, by modality: the matcher, or
-- examiners, placed their biometrics together. The link can be indirect, through other members.
-- shared_traces counts the case traces in those clusters.
SELECT other.id AS person_number, c.modality, BOOL_OR(c.basis = 'REVIEWED')::bool AS reviewed,
    (SELECT COUNT(DISTINCT tm.feature_id) FROM cluster_members tm
     WHERE tm.feature_id LIKE 'TRACE:%' AND tm.cluster_id = ANY(array_agg(c.id)))::bigint AS shared_traces
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_files f ON f.register_id = r.id
JOIN cluster_members mine ON mine.feature_id = f.id::text
JOIN clusters c ON c.id = mine.cluster_id
JOIN cluster_members theirs ON theirs.cluster_id = mine.cluster_id AND theirs.feature_id <> mine.feature_id
JOIN identity_files f2 ON f2.id::text = theirs.feature_id
JOIN identity_registers r2 ON r2.id = f2.register_id
JOIN identity_documents d2 ON d2.id = r2.document_id
JOIN persons other ON other.id = d2.person_id
WHERE p.id = @person_number AND other.id <> p.id
GROUP BY other.id, c.modality;

-- name: AskPersonIdentityDecisions :many
-- Every decision comparing one of this person's enrollments directly with another person's, with
-- the registers on each side: which document of whom matched which.
SELECT other.id AS person_number, bd.modality, bd.role, bd.decision, bd.confidence, bd.threshold,
    r.name AS my_name, d.document_type AS my_document_type, d.document_number AS my_document_number,
    r2.name AS their_name, d2.document_type AS their_document_type, d2.document_number AS their_document_number
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_files f ON f.register_id = r.id
JOIN biometric_decisions bd ON bd.feature_a_id = f.id::text OR bd.feature_b_id = f.id::text
JOIN identity_files f2 ON f2.id::text = CASE WHEN bd.feature_a_id = f.id::text THEN bd.feature_b_id ELSE bd.feature_a_id END
JOIN identity_registers r2 ON r2.id = f2.register_id
JOIN identity_documents d2 ON d2.id = r2.document_id
JOIN persons other ON other.id = d2.person_id
WHERE p.id = @person_number AND other.id <> p.id
ORDER BY other.id, bd.modality, bd.decided_at, bd.id;

-- name: AskPersonsShareParent :many
-- Of the given people, those with a parent's name in common with this person, on any register. It
-- answers yes or no: the names themselves stay on the person's page.
SELECT DISTINCT d2.person_id AS person_number
FROM identity_documents d
JOIN identity_registers r ON r.document_id = d.id
JOIN identity_documents d2 ON d2.person_id = ANY(@others::bigint[])
JOIN identity_registers r2 ON r2.document_id = d2.id
WHERE d.person_id = @person_number
  AND (lower(r.parent_1_name) IN (lower(r2.parent_1_name), lower(r2.parent_2_name))
    OR lower(r.parent_2_name) IN (lower(r2.parent_1_name), lower(r2.parent_2_name)));

-- name: AskPersonClusters :many
-- The clusters holding a person's enrollment samples, of both layers, with what each holds.
SELECT c.id AS cluster_id, c.basis, c.modality,
    (SELECT COUNT(*) FROM cluster_members m WHERE m.cluster_id = c.id)::bigint AS members,
    (SELECT COUNT(*) FROM cluster_members m WHERE m.cluster_id = c.id AND m.feature_id LIKE 'TRACE:%')::bigint AS traces
FROM clusters c
WHERE c.id IN (
    SELECT cm.cluster_id FROM cluster_members cm
    JOIN identity_files f ON f.id::text = cm.feature_id
    JOIN identity_registers r ON r.id = f.register_id
    JOIN identity_documents d ON d.id = r.document_id
    WHERE d.person_id = @person_number
)
ORDER BY c.basis, c.modality, c.id;

-- name: AskClusterPersons :many
-- The enrolled people a cluster holds samples of, with the names on those samples.
SELECT p.id AS person_number, json_agg(DISTINCT r.name)::jsonb AS names,
    COALESCE(MIN(r.birth_date), '')::text AS birth_date, COUNT(DISTINCT m.feature_id)::bigint AS samples
FROM cluster_members m
JOIN identity_files f ON f.id::text = m.feature_id
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE m.cluster_id = @cluster_id
GROUP BY p.id
ORDER BY p.id;

-- name: AskClusterCases :many
-- The biometric cases a cluster holds traces of.
SELECT bc.case_id, bc.case_type, bc.modality, bc.occurrence_id, COUNT(DISTINCT ct.id)::bigint AS traces
FROM cluster_members m
JOIN case_traces ct ON m.feature_id = 'TRACE:' || ct.id || '#feature'
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE m.cluster_id = @cluster_id
GROUP BY bc.id
ORDER BY bc.case_id;
