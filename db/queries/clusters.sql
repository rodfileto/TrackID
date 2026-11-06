-- Clusters come in two layers (clusters.basis, migrations/008_clusters.sql): AUTOMATIC, from every CONFIRMED
-- pair, and REVIEWED, from the pairs people confirmed. Every query resolving features to clusters
-- names its layer; cluster ids are unique across both, so queries by cluster id need none.

-- name: ListClusters :many
SELECT id, modality
FROM clusters
WHERE basis = $1
ORDER BY id;

-- name: ListClusterMembers :many
SELECT cluster_id, feature_id
FROM cluster_members
WHERE basis = $1;

-- name: CreateCluster :one
INSERT INTO clusters (modality, basis) VALUES ($1, $2) RETURNING id;

-- name: InsertClusterMember :exec
INSERT INTO cluster_members (cluster_id, feature_id, basis) VALUES ($1, $2, $3);

-- name: DeleteClusterMember :exec
DELETE FROM cluster_members WHERE basis = $1 AND feature_id = $2;

-- name: DeleteCluster :exec
DELETE FROM clusters WHERE id = $1;

-- name: InsertClusterMerge :exec
INSERT INTO cluster_merges (from_cluster_id, to_cluster_id, reason, basis) VALUES ($1, $2, $3, $4);

-- name: ListClusterIDsForFeatureIDs :many
-- Resolves a set of graph feature ids (graph.KnownFeatureID /
-- graph.QuestionedFeatureID) to the clusters they belong to in one layer.
-- cluster_members.feature_id is UNIQUE per basis, so each input feature id
-- contributes at most one row.
SELECT DISTINCT cluster_id
FROM cluster_members
WHERE basis = @basis AND feature_id = ANY(@feature_ids::text[])
ORDER BY cluster_id;

-- name: ListClusterMembershipsForFeatureIDs :many
-- Same lookup as ListClusterIDsForFeatureIDs but keeping feature_id on each
-- row, so a caller can map each of its own feature ids back to the cluster
-- it landed in (see cases.ListTraceClusters).
SELECT feature_id, cluster_id
FROM cluster_members
WHERE basis = @basis AND feature_id = ANY(@feature_ids::text[]);

-- name: ListBiometricDecisionsForFeatures :many
-- Every biometric_decisions row touching any of the given feature ids, seen
-- from that feature's side (biometric_decision_sides): the raw chain a caller
-- groups by counterpart_id to derive that pair's status (see
-- cluster.DeriveEdgeStatus) and who/what decided it (see
-- cases.ListCaseClusters). A decision between two of the given features
-- appears once from each side.
SELECT s.feature_id, s.counterpart_id, s.role, s.decision, s.system_source, s.username, s.confidence, s.case_analysis_id,
       ca.analysis_type, s.conclusion
FROM biometric_decision_sides s
LEFT JOIN case_analyses ca ON ca.id = s.case_analysis_id
WHERE s.feature_id = ANY(@feature_ids::text[])
ORDER BY s.feature_id, s.counterpart_id, s.decided_at, s.id;

-- name: ListClustersByIDs :many
SELECT id, modality, basis, created_at
FROM clusters
WHERE id = ANY(@cluster_ids::bigint[])
ORDER BY id;

-- name: ListClusterMembersByClusterIDs :many
SELECT cluster_id, feature_id
FROM cluster_members
WHERE cluster_id = ANY(@cluster_ids::bigint[])
ORDER BY cluster_id, feature_id;

-- name: ListCasesByCaseTraceIDs :many
-- Resolves a set of case_trace ids (parsed back out of QUESTIONED cluster
-- member feature ids -- see graph.QuestionedFeatureID) to their biometric
-- cases. One case can own several of the given traces (rows are not
-- de-duplicated by case) so a caller can re-attribute each case back to the
-- cluster that supplied the matching trace.
SELECT bc.case_id, bc.modality, bc.description, ct.id AS case_trace_id
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
WHERE ct.id = ANY(@case_trace_ids::bigint[])
ORDER BY bc.case_id, ct.id;

-- name: ListKnownClusterMembers :many
-- Resolves a set of identity_files ids (KNOWN cluster member feature ids --
-- see graph.KnownFeatureID) to the enrollment they belong to, for rendering
-- a cluster's membership (see person.buildClusters). Same join shape as
-- FindNearestPersonsByFaceEmbedding, minus the embedding distance.
SELECT p.id AS person_number, p.person_id, r.name, r.register_number, d.document_type, d.document_number,
       f.id AS identity_file_id, f.content_type
FROM identity_files f
JOIN identity_registers r ON r.id = f.register_id
JOIN identity_documents d ON d.id = r.document_id
JOIN persons p ON p.id = d.person_id
WHERE f.id = ANY(@identity_file_ids::bigint[]);

-- name: ListQuestionedClusterMembers :many
-- Resolves a set of case_trace ids (parsed back out of QUESTIONED cluster
-- member feature ids -- see graph.QuestionedFeatureID) to the case evidence
-- they belong to, for rendering a cluster's membership (see
-- person.buildClusters). Same join and thumbnail-source resolution as
-- FindNearestCasesByFaceEmbedding, minus the embedding distance.
SELECT bc.case_id, bc.modality, bc.description, ct.id AS case_trace_id,
       tcf.id AS trace_crop_file_id, ecf.id AS evidence_file_id,
       ct.box_x1, ct.box_y1, ct.box_x2, ct.box_y2
FROM case_traces ct
JOIN case_evidences ce ON ce.id = ct.evidence_id
JOIN biometric_cases bc ON bc.id = ce.biometric_case_id
LEFT JOIN case_files tcf ON tcf.id = ct.case_file_id
LEFT JOIN case_files ecf ON ecf.id = ce.case_file_id
WHERE ct.id = ANY(@case_trace_ids::bigint[]);

-- name: ListConflictCandidateMembers :many
-- The enrollment samples that can be part of an identity conflict (cluster.ListConflicts): those in a
-- cluster holding more than one name, and every sample whose identity -- by person, fiscal number, or
-- name with birth date -- has samples of one modality in different AUTOMATIC clusters. Spelling
-- variants and homonyms still come through here; Go tells them apart.
WITH known AS (
    SELECT m.basis, m.cluster_id, c.modality, m.feature_id, f.id AS identity_file_id, p.id AS person_number, p.person_id,
        r.name, r.birth_date, d.document_type, d.document_number, d.fiscal_number
    FROM cluster_members m
    JOIN clusters c ON c.id = m.cluster_id
    JOIN identity_files f ON f.id::text = m.feature_id
    JOIN identity_registers r ON r.id = f.register_id
    JOIN identity_documents d ON d.id = r.document_id
    JOIN persons p ON p.id = d.person_id
),
named AS (
    SELECT cluster_id FROM known GROUP BY cluster_id HAVING COUNT(DISTINCT lower(name)) > 1
),
split_person AS (
    SELECT person_number, modality FROM known WHERE basis = 'AUTOMATIC'
    GROUP BY person_number, modality HAVING COUNT(DISTINCT cluster_id) > 1
),
split_fiscal AS (
    SELECT fiscal_number, modality FROM known WHERE basis = 'AUTOMATIC' AND NULLIF(btrim(fiscal_number), '') IS NOT NULL
    GROUP BY fiscal_number, modality HAVING COUNT(DISTINCT cluster_id) > 1
),
split_name AS (
    SELECT lower(name) AS name, birth_date, modality FROM known WHERE basis = 'AUTOMATIC' AND NULLIF(btrim(birth_date), '') IS NOT NULL
    GROUP BY lower(name), birth_date, modality HAVING COUNT(DISTINCT cluster_id) > 1
)
SELECT k.basis, k.cluster_id, k.modality, k.feature_id, k.identity_file_id, k.person_number, k.person_id,
    k.name, k.birth_date, k.document_type, k.document_number, k.fiscal_number
FROM known k
WHERE k.cluster_id IN (SELECT cluster_id FROM named)
   OR (k.person_number, k.modality) IN (SELECT person_number, modality FROM split_person)
   OR (k.fiscal_number, k.modality) IN (SELECT fiscal_number, modality FROM split_fiscal)
   OR (lower(k.name), k.birth_date, k.modality) IN (SELECT name, birth_date, modality FROM split_name)
ORDER BY k.cluster_id, k.person_number, k.feature_id;

-- name: ListDecisionsBetweenFeatures :many
-- Every decision comparing two of the given features directly, oldest first.
SELECT bd.feature_a_id, bd.feature_b_id, bd.role, bd.decision, bd.confidence, bd.threshold,
    bd.case_analysis_id, ca.analysis_type, bd.conclusion
FROM biometric_decisions bd
LEFT JOIN case_analyses ca ON ca.id = bd.case_analysis_id
WHERE bd.feature_a_id = ANY(@feature_ids::text[]) AND bd.feature_b_id = ANY(@feature_ids::text[])
ORDER BY bd.feature_a_id, bd.feature_b_id, bd.decided_at, bd.id;

-- name: ListFaceScoresBetweenFiles :many
-- The cosine similarity between the enrollment faces of the given identity files, measured from their
-- embeddings. The matcher only stores what it found alike, so this is what shows two records differ.
SELECT fa.identity_file_id::bigint AS file_a, fb.identity_file_id::bigint AS file_b,
    (1 - (ea.embedding <=> eb.embedding))::float8 AS score
FROM biometric_features fa
JOIN feature_embeddings ea ON ea.biometric_feature_id = fa.id
JOIN biometric_features fb ON fb.identity_file_id > fa.identity_file_id
JOIN feature_embeddings eb ON eb.biometric_feature_id = fb.id AND eb.embedding_type = ea.embedding_type
WHERE fa.identity_file_id = ANY(@file_ids::bigint[]) AND fb.identity_file_id = ANY(@file_ids::bigint[]);

-- name: ListClusterTraceCounts :many
-- How many case traces each of the given clusters holds.
SELECT cluster_id, COUNT(*)::bigint AS traces
FROM cluster_members
WHERE cluster_id = ANY(@cluster_ids::bigint[]) AND feature_id LIKE 'TRACE:%'
GROUP BY cluster_id;

-- name: ListConflictRecords :many
-- The biographic data on the given people's registers, for comparing them side by side on a page.
SELECT p.id AS person_number, d.document_type, d.document_number, d.fiscal_number, r.register_number, r.name, r.birth_date,
    r.parent_1_name, r.parent_2_name
FROM persons p
JOIN identity_documents d ON d.person_id = p.id
JOIN identity_registers r ON r.document_id = d.id
WHERE p.id = ANY(@person_numbers::bigint[])
ORDER BY p.id, r.id;
