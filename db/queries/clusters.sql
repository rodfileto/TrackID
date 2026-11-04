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
SELECT p.person_id, r.name, r.register_number, d.document_type, d.document_number,
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
