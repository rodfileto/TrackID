// Package biometricmatch runs ANN similarity search over feature_embeddings
// and records SYSTEM biometric_decisions for pairs whose cosine distance
// clears a threshold. It is agnostic to what produced the embedding -- it
// only reads feature_embeddings and biometricfeature, and writes decisions
// exactly like a human examiner would, just with role SYSTEM.
package biometricmatch

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Stats reports what a run considered and decided.
type Stats struct {
	Embeddings int // unmatched embeddings (or templates, for RunTemplates) considered
	Decisions  int // SYSTEM biometric_decisions rows written (or that would be)
}

// candidatesPerEmbedding caps how many ANN neighbors are inspected per
// embedding before giving up on finding more within threshold.
const candidatesPerEmbedding = 20

// candidate is one pair found within threshold, expressed in graph feature
// ids (see graph.KnownFeatureID / graph.QuestionedFeatureID) with
// featureAID < featureBID, matching biometric_decisions' ordering constraint.
// confidence is the score stored on the decision: 1 - cosine distance for an
// embedding, the matcher's own score for a template.
type candidate struct {
	featureAID string
	featureBID string
	modality   string
	confidence float64
}

// Plan reports what Run would do for embeddingType, without writing anything.
func Plan(ctx context.Context, sqlDB *sql.DB, embeddingType string, threshold float64) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	candidates, _, considered, err := computeCandidates(ctx, sqlDB, embeddingType, threshold)
	if err != nil {
		return Stats{}, err
	}
	return Stats{Embeddings: considered, Decisions: len(candidates)}, nil
}

// Run finds ANN neighbors within threshold for every unmatched embedding of
// embeddingType, records a SYSTEM POSITIVE biometric_decisions row for each
// pair not already decided, and marks the embeddings considered as matched.
// systemSource identifies what produced the vectors (e.g. "FACE_ARCFACE_512")
// and is stored on each decision row.
func Run(ctx context.Context, sqlDB *sql.DB, embeddingType string, threshold float64, systemSource string) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	if systemSource == "" {
		return Stats{}, fmt.Errorf("systemSource is required")
	}

	candidates, embeddingIDs, considered, err := computeCandidates(ctx, sqlDB, embeddingType, threshold)
	if err != nil {
		return Stats{}, err
	}
	if err := recordDecisions(ctx, sqlDB, candidates, threshold, systemSource, embeddingIDs, (*db.Queries).MarkFeatureEmbeddingMatched); err != nil {
		return Stats{}, err
	}
	return Stats{Embeddings: considered, Decisions: len(candidates)}, nil
}

// recordDecisions writes a SYSTEM POSITIVE decision for each candidate and
// marks every considered id matched, in one transaction -- the write half
// shared by Run (embeddings) and RunTemplates (templates).
func recordDecisions(ctx context.Context, sqlDB *sql.DB, candidates []candidate, threshold float64, systemSource string,
	consideredIDs []int64, markMatched func(*db.Queries, context.Context, int64) error) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)

	for _, c := range candidates {
		if _, err := q.InsertBiometricDecision(ctx, db.InsertBiometricDecisionParams{
			FeatureAID:   c.featureAID,
			FeatureBID:   c.featureBID,
			Modality:     c.modality,
			Role:         "SYSTEM",
			Decision:     "POSITIVE",
			SystemSource: sql.NullString{String: systemSource, Valid: true},
			Confidence:   sql.NullFloat64{Float64: c.confidence, Valid: true},
			Threshold:    sql.NullFloat64{Float64: threshold, Valid: true},
		}); err != nil {
			return fmt.Errorf("biometricmatch: insert decision (%s, %s): %w", c.featureAID, c.featureBID, err)
		}
	}

	for _, id := range consideredIDs {
		if err := markMatched(q, ctx, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// featureInfo is a biometricfeature's graph feature id and biometric_decisions
// modality (FACE/FINGERPRINT, not to be confused with biometric_cases.modality
// which spells the face modality "FACIAL").
type featureInfo struct {
	graphID  string
	modality string
}

func modalityForFeatureType(featureType string) (string, bool) {
	switch featureType {
	case graph.FeatureTypeFaceRecord, graph.FeatureTypeFaceCapture:
		return "FACE", true
	case graph.FeatureTypeFingerprintTemplate, graph.FeatureTypeFingerprintLift:
		return "FINGERPRINT", true
	default:
		return "", false
	}
}

func loadFeatureInfos(ctx context.Context, sqlDB *sql.DB) (map[int64]featureInfo, error) {
	q := db.New(sqlDB)
	rows, err := q.ListBiometricFeatures(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]featureInfo, len(rows))
	for _, r := range rows {
		modality, ok := modalityForFeatureType(r.FeatureType)
		if !ok {
			continue
		}
		var graphID string
		switch {
		case r.IdentityFileID.Valid:
			graphID = graph.KnownFeatureID(r.IdentityFileID.Int64)
		case r.CaseTraceID.Valid:
			graphID = graph.QuestionedFeatureID(r.CaseTraceID.Int64)
		default:
			continue
		}
		out[r.ID] = featureInfo{graphID: graphID, modality: modality}
	}
	return out, nil
}

// existingSystemPairs loads the set of feature pairs that already have a
// SYSTEM decision, keyed "a|b" with a < b, so a rediscovered pair (found from
// either side, or on a later run) is never redecided.
func existingSystemPairs(ctx context.Context, sqlDB *sql.DB) (map[string]bool, error) {
	q := db.New(sqlDB)
	rows, err := q.ListBiometricDecisions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.Role != "SYSTEM" {
			continue
		}
		out[r.FeatureAID+"|"+r.FeatureBID] = true
	}
	return out, nil
}

// computeCandidates is the read-only core shared by Plan and Run: it loads
// every unmatched embedding of embeddingType, searches its ANN neighbors, and
// returns the not-yet-decided pairs within threshold plus the full set of
// embedding ids considered (so Run can mark them matched even when they
// produced no candidate).
func computeCandidates(ctx context.Context, sqlDB *sql.DB, embeddingType string, threshold float64) ([]candidate, []int64, int, error) {
	q := db.New(sqlDB)

	unmatched, err := q.ListUnmatchedFeatureEmbeddings(ctx, embeddingType)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(unmatched) == 0 {
		return nil, nil, 0, nil
	}

	features, err := loadFeatureInfos(ctx, sqlDB)
	if err != nil {
		return nil, nil, 0, err
	}
	existing, err := existingSystemPairs(ctx, sqlDB)
	if err != nil {
		return nil, nil, 0, err
	}

	var candidates []candidate
	seen := map[string]bool{}
	embeddingIDs := make([]int64, 0, len(unmatched))

	for _, u := range unmatched {
		embeddingIDs = append(embeddingIDs, u.ID)
		sourceInfo, ok := features[u.BiometricfeatureID]
		if !ok {
			continue
		}

		neighbors, err := q.FindNearestFeatureEmbeddings(ctx, db.FindNearestFeatureEmbeddingsParams{
			Embedding:     u.Embedding,
			EmbeddingType: embeddingType,
			ExcludeID:     u.ID,
			ResultLimit:   candidatesPerEmbedding,
		})
		if err != nil {
			return nil, nil, 0, err
		}

		for _, n := range neighbors {
			distance, ok := n.Distance.(float64)
			if !ok {
				continue
			}
			if distance > threshold {
				break // ORDER BY distance ASC, so nothing further qualifies
			}
			neighborInfo, ok := features[n.BiometricfeatureID]
			if !ok {
				continue
			}
			a, b := sourceInfo.graphID, neighborInfo.graphID
			if a == b {
				continue
			}
			if a > b {
				a, b = b, a
			}
			key := a + "|" + b
			if existing[key] || seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, candidate{
				featureAID: a,
				featureBID: b,
				modality:   sourceInfo.modality,
				confidence: 1 - distance,
			})
		}
	}

	return candidates, embeddingIDs, len(unmatched), nil
}
