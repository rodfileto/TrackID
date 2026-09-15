package graph

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid/db"
)

// SyncStats reports what Sync materialized.
type SyncStats struct {
	Evidence int
	Features int
}

// caseRow is one criminal_cases row (a base case).
type caseRow struct {
	caseID      string
	caseType    string
	description string
}

// questionedFeatureRow is one QUESTIONED biometricfeature joined up to its
// criminal case (via case_traces/case_evidences).
type questionedFeatureRow struct {
	caseTraceID int64
	featureType string
	caseID      string
}

// Sync materializes criminal_cases into Evidence nodes and QUESTIONED
// biometricfeature rows into BiometricFeature nodes. KNOWN features are
// materialized by SyncIdentity (the identity chain), not here. Decisions are
// not materialized here either — they live in Postgres (biometric_decisions)
// and cluster status is derived from them. Sync reads only from Postgres.
func Sync(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext) (SyncStats, error) {
	cases, features, err := loadSyncRows(ctx, sqlDB)
	if err != nil {
		return SyncStats{}, err
	}

	evidenceParams := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		mapping, ok := Modalities[c.caseType]
		if !ok {
			continue
		}
		evidenceParams = append(evidenceParams, map[string]any{
			"caseId":       c.caseID,
			"description":  c.description,
			"evidenceType": mapping.EvidenceType,
		})
	}

	featureParams := make([]map[string]any, 0, len(features))
	for _, f := range features {
		featureParams = append(featureParams, map[string]any{
			"caseId":      f.caseID,
			"featureId":   QuestionedFeatureID(f.caseTraceID),
			"featureType": f.featureType,
		})
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	if len(evidenceParams) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncEvidenceQuery, map[string]any{"rows": evidenceParams})
			return nil, err
		}); err != nil {
			return SyncStats{}, fmt.Errorf("materialize evidence: %w", err)
		}
	}
	if len(featureParams) > 0 {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, syncFeaturesQuery, map[string]any{"rows": featureParams})
			return nil, err
		}); err != nil {
			return SyncStats{}, fmt.Errorf("materialize features: %w", err)
		}
	}
	return SyncStats{Evidence: len(evidenceParams), Features: len(featureParams)}, nil
}

// SyncPlan returns the counts Sync would materialize, without connecting to
// Neo4j.
func SyncPlan(ctx context.Context, sqlDB *sql.DB) (SyncStats, error) {
	cases, features, err := loadSyncRows(ctx, sqlDB)
	if err != nil {
		return SyncStats{}, err
	}
	evidence := 0
	for _, c := range cases {
		if _, ok := Modalities[c.caseType]; ok {
			evidence++
		}
	}
	return SyncStats{Evidence: evidence, Features: len(features)}, nil
}

func loadSyncRows(ctx context.Context, sqlDB *sql.DB) ([]caseRow, []questionedFeatureRow, error) {
	if sqlDB == nil {
		return nil, nil, fmt.Errorf("database is not configured")
	}
	cases, err := loadCaseRows(ctx, sqlDB)
	if err != nil {
		return nil, nil, err
	}
	features, err := loadQuestionedFeatureRows(ctx, sqlDB)
	if err != nil {
		return nil, nil, err
	}
	return cases, features, nil
}

func loadCaseRows(ctx context.Context, sqlDB *sql.DB) ([]caseRow, error) {
	rows, err := db.New(sqlDB).ListAllCriminalCases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]caseRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, caseRow{caseID: r.CaseID, caseType: r.CaseType, description: r.Description})
	}
	return out, nil
}

func loadQuestionedFeatureRows(ctx context.Context, sqlDB *sql.DB) ([]questionedFeatureRow, error) {
	rows, err := db.New(sqlDB).ListQuestionedFeatures(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]questionedFeatureRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, questionedFeatureRow{
			caseTraceID: r.CaseTraceID.Int64,
			featureType: r.FeatureType,
			caseID:      r.CaseID,
		})
	}
	return out, nil
}

const syncEvidenceQuery = `
UNWIND $rows AS row
MERGE (e:Object:Evidence {evidenceId: row.caseId})
ON CREATE SET e.description = row.description, e.evidenceType = row.evidenceType
`

const syncFeaturesQuery = `
UNWIND $rows AS row
MERGE (e:Object:Evidence {evidenceId: row.caseId})
MERGE (f:Object:BiometricFeature {featureId: row.featureId})
ON CREATE SET f.featureType = row.featureType, f.provenance = 'QUESTIONED'
MERGE (e)-[:HAS_FEATURE]->(f)
`
