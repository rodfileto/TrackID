package graph

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// syncSource is the value stored on Event:Decision.source for decisions Sync
// materializes.
const syncSource = "CRIMINAL_CASES_SYNC"

// SyncStats reports what Sync materialized.
type SyncStats struct {
	Cases     int
	Decisions int
}

// caseRow is one criminal_cases row (an evidence item).
type caseRow struct {
	caseID      string
	caseType    string
	description string
}

// comparisonRow is one comparisons row (an evidence-to-evidence edge).
type comparisonRow struct {
	evidenceA       string
	evidenceB       string
	caseType        string
	comparisonType  string
	responsibleUser sql.NullString
}

// Sync materializes criminal_cases into the Neo4j forensic graph: an
// Object:Evidence node plus its Object:BiometricFeature for every case, and an
// Event:Decision linking two evidence items for every comparison. It reads only
// from Postgres.
func Sync(ctx context.Context, db *sql.DB, driver neo4j.DriverWithContext) (SyncStats, error) {
	evidenceRows, decisionRows, stats, err := buildSyncRows(ctx, db)
	if err != nil {
		return SyncStats{}, err
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if len(evidenceRows) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncEvidenceQuery, map[string]any{"rows": evidenceRows})
			return nil, err
		}); err != nil {
			return SyncStats{}, fmt.Errorf("materialize evidence: %w", err)
		}
	}
	if len(decisionRows) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncDecisionsQuery, map[string]any{"decisions": decisionRows})
			return nil, err
		}); err != nil {
			return SyncStats{}, fmt.Errorf("materialize decisions: %w", err)
		}
	}
	return stats, nil
}

// SyncPlan returns the counts Sync would materialize, without connecting to
// Neo4j.
func SyncPlan(ctx context.Context, db *sql.DB) (SyncStats, error) {
	_, _, stats, err := buildSyncRows(ctx, db)
	return stats, err
}

// buildSyncRows loads criminal_cases and comparisons and shapes them into the
// parameter rows for the evidence and decision write queries.
func buildSyncRows(ctx context.Context, db *sql.DB) ([]map[string]any, []map[string]any, SyncStats, error) {
	if db == nil {
		return nil, nil, SyncStats{}, fmt.Errorf("database is not configured")
	}
	cases, err := loadCaseRows(ctx, db)
	if err != nil {
		return nil, nil, SyncStats{}, err
	}
	comparisons, err := loadComparisonRows(ctx, db)
	if err != nil {
		return nil, nil, SyncStats{}, err
	}

	evidenceRows := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		mapping, ok := Modalities[c.caseType]
		if !ok {
			continue
		}
		evidenceRows = append(evidenceRows, map[string]any{
			"caseId":       c.caseID,
			"featureId":    FeatureID(c.caseID),
			"description":  c.description,
			"evidenceType": mapping.EvidenceType,
			"featureType":  mapping.FeatureType,
			"featureLabel": mapping.FeatureLabel,
		})
	}

	var decisionRows []map[string]any
	for _, c := range comparisons {
		mapping, ok := Modalities[c.caseType]
		if !ok {
			continue
		}
		var responsibleUser any
		if c.responsibleUser.Valid {
			responsibleUser = c.responsibleUser.String
		}
		decisionRows = append(decisionRows, map[string]any{
			"caseId":           c.evidenceA,
			"relatedReference": c.evidenceB,
			"relatedFeatureId": FeatureID(c.evidenceB),
			"decisionId":       DecisionID(c.evidenceA, c.evidenceB),
			"comparisonType":   c.comparisonType,
			"responsibleUser":  responsibleUser,
			"evidenceType":     mapping.EvidenceType,
			"featureType":      mapping.FeatureType,
			"featureLabel":     mapping.FeatureLabel,
			"source":           syncSource,
		})
	}

	return evidenceRows, decisionRows, SyncStats{Cases: len(evidenceRows), Decisions: len(decisionRows)}, nil
}

const caseRowsQuery = `
SELECT case_id, case_type, description
FROM criminal_cases
ORDER BY case_id
`

func loadCaseRows(ctx context.Context, db *sql.DB) ([]caseRow, error) {
	rows, err := db.QueryContext(ctx, caseRowsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []caseRow
	for rows.Next() {
		var r caseRow
		if err := rows.Scan(&r.caseID, &r.caseType, &r.description); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const comparisonRowsQuery = `
SELECT evidence_a, evidence_b, case_type, comparison_type, responsible_user
FROM comparisons
WHERE status = 'confirmed'
ORDER BY evidence_a, evidence_b
`

func loadComparisonRows(ctx context.Context, db *sql.DB) ([]comparisonRow, error) {
	rows, err := db.QueryContext(ctx, comparisonRowsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []comparisonRow
	for rows.Next() {
		var r comparisonRow
		if err := rows.Scan(&r.evidenceA, &r.evidenceB, &r.caseType, &r.comparisonType, &r.responsibleUser); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const syncEvidenceQuery = `
UNWIND $rows AS row
MERGE (e:Object:Evidence {evidenceId: row.caseId})
ON CREATE SET e.description = row.description, e.evidenceType = row.evidenceType
MERGE (f:Object:BiometricFeature {featureId: row.featureId})
ON CREATE SET f.featureType = row.featureType
FOREACH (_ IN CASE WHEN row.featureLabel = 'FingerprintLift' THEN [1] ELSE [] END | SET f:FingerprintLift)
FOREACH (_ IN CASE WHEN row.featureLabel = 'FaceCapture' THEN [1] ELSE [] END | SET f:FaceCapture)
MERGE (e)-[:HAS_FEATURE]->(f)
`

const syncDecisionsQuery = `
UNWIND $decisions AS d
MERGE (a:Object:Evidence {evidenceId: d.caseId})
MERGE (b:Object:Evidence {evidenceId: d.relatedReference})
ON CREATE SET b.evidenceType = d.evidenceType
MERGE (fb:Object:BiometricFeature {featureId: d.relatedFeatureId})
ON CREATE SET fb.featureType = d.featureType
FOREACH (_ IN CASE WHEN d.featureLabel = 'FingerprintLift' THEN [1] ELSE [] END | SET fb:FingerprintLift)
FOREACH (_ IN CASE WHEN d.featureLabel = 'FaceCapture' THEN [1] ELSE [] END | SET fb:FaceCapture)
MERGE (b)-[:HAS_FEATURE]->(fb)
MERGE (decision:Event:Decision {decisionId: d.decisionId})
ON CREATE SET decision.matchType = d.comparisonType, decision.source = d.source
MERGE (a)-[:COMPARED]->(decision)
MERGE (decision)-[:COMPARED]->(b)
FOREACH (_ IN CASE WHEN d.responsibleUser IS NOT NULL THEN [1] ELSE [] END |
    MERGE (u:User {username: d.responsibleUser})
    MERGE (u)-[:DECIDED]->(decision)
)
`
