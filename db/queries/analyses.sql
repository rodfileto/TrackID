-- name: InsertBiometricDecision :one
INSERT INTO biometric_decisions
    (feature_a_id, feature_b_id, modality, role, decision, system_source, username, confidence, threshold, notes,
     related_reference, related_reference_kind, case_analysis_id, conclusion)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, sqlc.narg('conclusion'))
RETURNING id, feature_a_id, feature_b_id, modality, role, decision, system_source, username, confidence, threshold, notes,
    related_reference, related_reference_kind, decided_at, created_at, case_analysis_id;

-- name: ListBiometricDecisions :many
-- analysis_type and conclusion say what a human round was: a review (a lead) or a comparison (a
-- laudo, with its level).
SELECT d.feature_a_id, d.feature_b_id, d.modality, d.role, d.decision, d.system_source, d.username, d.confidence,
       d.case_analysis_id, ca.analysis_type, d.conclusion
FROM biometric_decisions d
LEFT JOIN case_analyses ca ON ca.id = d.case_analysis_id
ORDER BY d.feature_a_id, d.feature_b_id, d.decided_at, d.id;

-- name: InsertCaseAnalysis :one
-- The case's next analysis, numbered after its last one (lock the case first: LockBiometricCase).
INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type, requested_by, case_request_id)
SELECT @biometric_case_id::bigint, COALESCE(MAX(sequence), 0) + 1, @analysis_type::text, sqlc.narg('requested_by')::bigint,
       sqlc.narg('case_request_id')::bigint
FROM case_analyses
WHERE biometric_case_id = @biometric_case_id::bigint
RETURNING id, sequence;

-- name: InsertCaseAnalysisPair :exec
INSERT INTO case_analysis_pairs (case_analysis_id, feature_a_id, feature_b_id) VALUES ($1, $2, $3);

-- name: GetCaseAnalysis :one
-- One analysis with its case and its current examiner for each step (NULL while unassigned).
SELECT ca.id, ca.sequence, ca.analysis_type, ca.status, ca.stage, bc.id AS biometric_case_id, bc.case_id, bc.modality,
       v.user_id AS verificator_id, r.user_id AS reviewer_id, i.user_id AS inconsistence_id
FROM case_analyses ca
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
LEFT JOIN current_analysis_assignments v ON v.case_analysis_id = ca.id AND v.step = 'VERIFICATOR'
LEFT JOIN current_analysis_assignments r ON r.case_analysis_id = ca.id AND r.step = 'REVIEWER'
LEFT JOIN current_analysis_assignments i ON i.case_analysis_id = ca.id AND i.step = 'INCONSISTENCE'
WHERE ca.id = $1;

-- name: SetCaseAnalysisStage :exec
UPDATE case_analyses SET stage = $2 WHERE id = $1;

-- name: ListCaseAnalyses :many
-- A case's analyses in order, with who requested each, its examiner per step and its report.
SELECT ca.id, ca.sequence, ca.analysis_type, ca.status, ca.stage, ca.created_at, ca.closed_at,
       rq.username AS requested_by,
       vu.id AS verificator_id, vu.username AS verificator,
       ru.id AS reviewer_id, ru.username AS reviewer,
       iu.id AS inconsistence_id, iu.username AS inconsistence,
       ar.report_type, ar.status AS report_status, ar.report_year, ar.report_number, ar.issued_at,
       ib.username AS issued_by, ca.case_request_id, ar.id AS report_id, ar.complements_report_id
FROM case_analyses ca
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
LEFT JOIN users rq ON rq.id = ca.requested_by
LEFT JOIN current_analysis_assignments v ON v.case_analysis_id = ca.id AND v.step = 'VERIFICATOR'
LEFT JOIN users vu ON vu.id = v.user_id
LEFT JOIN current_analysis_assignments r ON r.case_analysis_id = ca.id AND r.step = 'REVIEWER'
LEFT JOIN users ru ON ru.id = r.user_id
LEFT JOIN current_analysis_assignments i ON i.case_analysis_id = ca.id AND i.step = 'INCONSISTENCE'
LEFT JOIN users iu ON iu.id = i.user_id
LEFT JOIN analysis_reports ar ON ar.case_analysis_id = ca.id
LEFT JOIN users ib ON ib.id = ar.issued_by
WHERE bc.case_id = $1
ORDER BY ca.sequence;

-- name: ListCaseAnalysisPairs :many
SELECT cap.case_analysis_id, cap.feature_a_id, cap.feature_b_id
FROM case_analysis_pairs cap
JOIN case_analyses ca ON ca.id = cap.case_analysis_id
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
WHERE bc.case_id = $1
ORDER BY cap.case_analysis_id, cap.id;

-- name: ListAnalysisPairs :many
SELECT feature_a_id, feature_b_id FROM case_analysis_pairs WHERE case_analysis_id = $1 ORDER BY id;

-- name: InsertAnalysisAssignment :one
INSERT INTO analysis_assignments (case_analysis_id, step, user_id, method, assigned_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: LockCaseDistribution :exec
-- Serializes distributions and manual assignments (transaction-scoped), so two admins
-- distributing at once cannot hand the same pending work out twice. The key is arbitrary.
SELECT pg_advisory_xact_lock(7461001);

-- name: ListPendingWork :many
-- Open analyses whose current stage has no examiner yet, oldest first: the order
-- distribution.Distribute hands them out in, with who holds the other steps (an examiner never
-- takes two steps of one analysis). analysis_type filters when non-NULL.
SELECT ca.id, ca.sequence, ca.analysis_type, ca.stage, bc.case_id,
       v.user_id AS verificator_id, r.user_id AS reviewer_id
FROM case_analyses ca
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
LEFT JOIN current_analysis_assignments v ON v.case_analysis_id = ca.id AND v.step = 'VERIFICATOR'
LEFT JOIN current_analysis_assignments r ON r.case_analysis_id = ca.id AND r.step = 'REVIEWER'
LEFT JOIN current_analysis_assignments cur ON cur.case_analysis_id = ca.id AND cur.step = (CASE ca.stage WHEN 'VERIFICATION' THEN 'REVIEWER' WHEN 'INCONSISTENCE' THEN 'INCONSISTENCE' ELSE 'VERIFICATOR' END)
WHERE ca.status = 'OPEN' AND ca.stage <> 'REPORT' AND cur.user_id IS NULL
  AND (sqlc.narg('analysis_type')::text IS NULL OR ca.analysis_type = sqlc.narg('analysis_type'))
ORDER BY ca.created_at, ca.id;

-- name: ListPairDeciders :many
-- The users who have already worked on any of the given pairs: a human decision on one of them
-- (submitted in an analysis, or imported), or a draft of one in an analysis other than
-- analysis_id. distribution.Distribute gives them a step on these pairs only when nobody else is
-- eligible.
SELECT u.id
FROM users u
WHERE EXISTS (
        SELECT 1
        FROM biometric_decisions d
        WHERE d.role <> 'SYSTEM' AND d.username = u.username
          AND (d.feature_a_id, d.feature_b_id) IN (SELECT unnest(@feature_a_ids::text[]), unnest(@feature_b_ids::text[])))
   OR EXISTS (
        SELECT 1
        FROM analysis_decision_drafts dr
        WHERE dr.user_id = u.id AND dr.case_analysis_id <> @analysis_id::bigint
          AND (dr.feature_a_id, dr.feature_b_id) IN (SELECT unnest(@feature_a_ids::text[]), unnest(@feature_b_ids::text[])))
ORDER BY u.id;

-- name: ListExaminerLoads :many
-- Every active examiner with the number of open analyses whose current stage is theirs to work
-- (the report counting for the first examiner).
SELECT u.id, u.username, u.examines_fingerprint, u.examines_facial,
       COUNT(ca.id) AS open_work
FROM users u
LEFT JOIN current_analysis_assignments caa ON caa.user_id = u.id
LEFT JOIN case_analyses ca ON ca.id = caa.case_analysis_id AND ca.status = 'OPEN' AND caa.step = (CASE ca.stage WHEN 'VERIFICATION' THEN 'REVIEWER' WHEN 'INCONSISTENCE' THEN 'INCONSISTENCE' ELSE 'VERIFICATOR' END)
WHERE u.is_active AND (u.examines_fingerprint OR u.examines_facial)
GROUP BY u.id
ORDER BY u.id;

-- name: CountPendingWork :many
SELECT ca.analysis_type, ca.stage, COUNT(*) AS pending
FROM case_analyses ca
LEFT JOIN current_analysis_assignments cur ON cur.case_analysis_id = ca.id AND cur.step = (CASE ca.stage WHEN 'VERIFICATION' THEN 'REVIEWER' WHEN 'INCONSISTENCE' THEN 'INCONSISTENCE' ELSE 'VERIFICATOR' END)
WHERE ca.status = 'OPEN' AND ca.stage <> 'REPORT' AND cur.user_id IS NULL
GROUP BY ca.analysis_type, ca.stage
ORDER BY ca.analysis_type, ca.stage;

-- name: ListAnalysisAssignmentHistory :many
SELECT aa.id, aa.step, u.username, aa.method, ab.username AS assigned_by_username, aa.assigned_at
FROM analysis_assignments aa
LEFT JOIN users u ON u.id = aa.user_id
LEFT JOIN users ab ON ab.id = aa.assigned_by
WHERE aa.case_analysis_id = $1
ORDER BY aa.id;

-- name: ListAnalysesAssignedTo :many
-- Every step a user currently holds, open analyses first, newest assignment first.
SELECT bc.case_id, bc.case_type, bc.description, ca.id, ca.sequence, ca.analysis_type, ca.status, ca.stage,
       caa.step, caa.method, caa.assigned_at
FROM current_analysis_assignments caa
JOIN case_analyses ca ON ca.id = caa.case_analysis_id
JOIN biometric_cases bc ON bc.id = ca.biometric_case_id
WHERE caa.user_id = $1
ORDER BY ca.status DESC, caa.assigned_at DESC, ca.id DESC;

-- name: GetCaseAccess :one
-- Whether a user currently holds any step of any analysis of a case (open or closed).
SELECT bc.id, EXISTS (
    SELECT 1 FROM case_analyses ca
    JOIN current_analysis_assignments caa ON caa.case_analysis_id = ca.id
    WHERE ca.biometric_case_id = bc.id AND caa.user_id = @user_id::bigint
) AS holds_analysis
FROM biometric_cases bc
WHERE bc.case_id = @case_id::text;

-- name: UpsertDecisionDraft :exec
INSERT INTO analysis_decision_drafts (case_analysis_id, step, feature_a_id, feature_b_id, decision, notes, user_id, conclusion)
VALUES ($1, $2, $3, $4, $5, $6, $7, sqlc.narg('conclusion'))
ON CONFLICT (case_analysis_id, step, feature_a_id, feature_b_id) DO UPDATE SET
    decision = EXCLUDED.decision, notes = EXCLUDED.notes, user_id = EXCLUDED.user_id, conclusion = EXCLUDED.conclusion,
    updated_at = NOW();

-- name: ListDecisionDrafts :many
SELECT feature_a_id, feature_b_id, decision, notes, conclusion
FROM analysis_decision_drafts
WHERE case_analysis_id = $1 AND step = $2;

-- name: DeleteDecisionDrafts :exec
DELETE FROM analysis_decision_drafts WHERE case_analysis_id = $1 AND step = $2;

-- name: ListAnalysisDecisions :many
-- The human decisions submitted in one analysis.
SELECT feature_a_id, feature_b_id, role, decision, username, notes, decided_at, conclusion
FROM biometric_decisions
WHERE case_analysis_id = $1
ORDER BY feature_a_id, feature_b_id, id;

-- name: ListSystemDecisionsForFeatures :many
-- The SYSTEM decisions (the matcher's candidates) touching any of the given features: each pair's
-- latest, the one under the newest threshold version that decided it.
SELECT feature_a_id, feature_b_id, decision, confidence
FROM (
    SELECT DISTINCT ON (feature_a_id, feature_b_id) feature_a_id, feature_b_id, decision, confidence
    FROM biometric_decisions
    WHERE role = 'SYSTEM' AND (feature_a_id = ANY(@feature_ids::text[]) OR feature_b_id = ANY(@feature_ids::text[]))
    ORDER BY feature_a_id, feature_b_id, decided_at DESC, id DESC
) latest
ORDER BY confidence DESC NULLS LAST, feature_a_id, feature_b_id;

-- name: CloseCaseAnalysis :exec
UPDATE case_analyses SET status = 'CLOSED', closed_at = NOW() WHERE id = $1 AND status = 'OPEN';

-- name: LatestHumanDecisionID :one
-- The newest human decision (VERIFICATOR, REVIEWER, INCONSISTENCE): a sync's mark of the
-- examiners' decisions it has reconciled clusters with (cluster.Settle).
SELECT COALESCE(MAX(id), 0)::bigint AS id FROM biometric_decisions WHERE role <> 'SYSTEM';

-- name: CountHumanDecisionsOnFeature :one
SELECT COUNT(*) FROM biometric_decisions
WHERE role <> 'SYSTEM' AND (feature_a_id = @feature_id::text OR feature_b_id = @feature_id::text);

-- name: CurrentIdentificationLevel :one
SELECT minimum_conclusion FROM identification_levels ORDER BY id DESC LIMIT 1;

-- name: ListSystemSourcesForFeatures :many
-- Which matcher, at which cutoff, proposed candidates for the given features: a review report's
-- method.
SELECT DISTINCT system_source, threshold, related_reference
FROM biometric_decisions
WHERE role = 'SYSTEM' AND (feature_a_id = ANY(@feature_ids::text[]) OR feature_b_id = ANY(@feature_ids::text[]))
ORDER BY system_source, threshold;
