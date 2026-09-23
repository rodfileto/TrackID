package biometricmatch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/rodfileto/trackid/db"
)

// Band is the automatic matcher's pair of cutoffs, as similarities on the scale of
// biometric_decisions.confidence (1 - cosine distance for an embedding). A pair at or above
// Confirm is a SYSTEM POSITIVE (CONFIRMED on its own); at or above Review but below Confirm, a
// SYSTEM INCONCLUSIVE (PENDING_REVIEW until an examiner decides); below Review, nothing.
// Review == Confirm is a single cutoff with no review band.
type Band struct {
	Review  float64
	Confirm float64
}

// Validate checks 0 < Review <= Confirm <= 1, the same rule as match_thresholds' check.
func (b Band) Validate() error {
	if !(b.Review > 0 && b.Review <= b.Confirm && b.Confirm <= 1) {
		return fmt.Errorf("biometricmatch: band needs 0 < review <= confirm <= 1, got review %v, confirm %v", b.Review, b.Confirm)
	}
	return nil
}

// classify returns the SYSTEM decision for a pair scoring confidence, and the cutoff it was
// classified against. Callers only pass pairs already at or above b.Review.
func (b Band) classify(confidence float64) (decision string, threshold float64) {
	if confidence >= b.Confirm {
		return "POSITIVE", b.Confirm
	}
	return "INCONCLUSIVE", b.Review
}

// Version is one match_thresholds row: a Band with its provenance. ID is 0 for the built-in
// default used when no version has been set.
type Version struct {
	ID            int64
	EmbeddingType string
	Band          Band
	Source        string
}

// CurrentBand returns embeddingType's current threshold version: the latest match_thresholds
// row, or fallback (ID 0, Source "default") when none has been set.
func CurrentBand(ctx context.Context, sqlDB *sql.DB, embeddingType string, fallback Band) (Version, error) {
	row, err := db.New(sqlDB).GetCurrentMatchThreshold(ctx, embeddingType)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{EmbeddingType: embeddingType, Band: fallback, Source: "default"}, nil
	}
	if err != nil {
		return Version{}, fmt.Errorf("biometricmatch: current threshold for %s: %w", embeddingType, err)
	}
	return Version{
		ID:            row.ID,
		EmbeddingType: row.EmbeddingType,
		Band:          Band{Review: row.ReviewThreshold, Confirm: row.ConfirmThreshold},
		Source:        row.Source,
	}, nil
}

// SetBand records a new threshold version for embeddingType, which becomes current. source
// says where the numbers came from (e.g. "ROC on validation set X, FMR 1e-3"); createdBy may be
// empty.
func SetBand(ctx context.Context, sqlDB *sql.DB, embeddingType string, band Band, source, createdBy string) (Version, error) {
	if err := band.Validate(); err != nil {
		return Version{}, err
	}
	if embeddingType == "" || source == "" {
		return Version{}, fmt.Errorf("biometricmatch: embedding type and source are required")
	}
	row, err := db.New(sqlDB).InsertMatchThreshold(ctx, db.InsertMatchThresholdParams{
		EmbeddingType:    embeddingType,
		ReviewThreshold:  band.Review,
		ConfirmThreshold: band.Confirm,
		Source:           source,
		CreatedBy:        sql.NullString{String: createdBy, Valid: createdBy != ""},
	})
	if err != nil {
		return Version{}, fmt.Errorf("biometricmatch: set threshold for %s: %w", embeddingType, err)
	}
	return Version{ID: row.ID, EmbeddingType: row.EmbeddingType, Band: band, Source: row.Source}, nil
}

// RunBand is Run with a review band: it finds ANN neighbors at or above v.Band.Review for every
// unmatched embedding of embeddingType and records, for each pair not already decided, a SYSTEM
// POSITIVE (at or above Confirm) or SYSTEM INCONCLUSIVE (below it), citing v as the decision's
// related reference when v.ID is set. Stats.Review counts the INCONCLUSIVE ones.
func RunBand(ctx context.Context, sqlDB *sql.DB, embeddingType string, v Version, systemSource string) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	if systemSource == "" {
		return Stats{}, fmt.Errorf("systemSource is required")
	}
	if err := v.Band.Validate(); err != nil {
		return Stats{}, err
	}

	candidates, embeddingIDs, considered, err := computeCandidates(ctx, sqlDB, embeddingType, 1-v.Band.Review)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Embeddings: considered, Decisions: len(candidates)}
	for i := range candidates {
		candidates[i].decision, candidates[i].threshold = v.Band.classify(candidates[i].confidence)
		if candidates[i].decision == "INCONCLUSIVE" {
			stats.Review++
		}
	}
	var ref string
	if v.ID != 0 {
		ref = strconv.FormatInt(v.ID, 10)
	}
	if err := recordDecisions(ctx, sqlDB, candidates, systemSource, ref, embeddingIDs, (*db.Queries).MarkFeatureEmbeddingMatched); err != nil {
		return Stats{}, err
	}
	return stats, nil
}
