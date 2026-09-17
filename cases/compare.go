package cases

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/storage"
)

// Embedding sources reported per face in a FaceComparison.
const (
	// EmbeddingSourceStored is the vector already in feature_embeddings --
	// the same one system matching (cmd/match-embeddings) uses.
	EmbeddingSourceStored = "stored"
	// EmbeddingSourceCodificationImage is computed now from the analyst's
	// saved (adjusted) codification image.
	EmbeddingSourceCodificationImage = "codification_image"
	// EmbeddingSourceEvidence is computed now from the evidence image around
	// the trace's box.
	EmbeddingSourceEvidence = "evidence"
)

// ErrNotFaceCodification is returned by CompareFaces for a codification that
// isn't a face embedding (e.g. a fingerprint's minutiae).
var ErrNotFaceCodification = errors.New("cases: codification is not a face codification")

// NoFaceError is returned by CompareFaces when no face could be found to
// embed for one of the codifications.
type NoFaceError struct {
	CodificationID int64
}

func (e NoFaceError) Error() string {
	return fmt.Sprintf("cases: no face detected for codification %d", e.CodificationID)
}

// ComparedFace is one side of a FaceComparison.
type ComparedFace struct {
	CodificationID int64  `json:"codificationId"`
	Source         string `json:"source"`
}

// FaceComparison is the machine similarity between two face codifications:
// the cosine similarity of their embeddings (1 = identical direction).
type FaceComparison struct {
	Similarity    float64         `json:"similarity"`
	EmbeddingType string          `json:"embeddingType"`
	ModelVersion  string          `json:"modelVersion"`
	Faces         [2]ComparedFace `json:"faces"`
}

// CompareFaces scores how alike two face codifications of a case are. Each
// side uses its stored embedding when there is one, and otherwise computes
// one now with vis (nothing computed here is stored). vis may be nil when
// both codifications already have embeddings.
func CompareFaces(ctx context.Context, sqlDB *sql.DB, store *storage.Client, vis FaceVision, caseID string, codificationIDs [2]int64) (FaceComparison, error) {
	if sqlDB == nil {
		return FaceComparison{}, fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FaceComparison{}, ErrNotFound
		}
		return FaceComparison{}, err
	}

	result := FaceComparison{
		EmbeddingType: embedding.EmbeddingType,
		ModelVersion:  embedding.ModelVersion,
	}
	var vectors [2][]float64
	for i, codificationID := range codificationIDs {
		vector, source, err := codificationEmbedding(ctx, q, store, vis, caseRow.ID, codificationID)
		if err != nil {
			return FaceComparison{}, err
		}
		vectors[i] = vector
		result.Faces[i] = ComparedFace{CodificationID: codificationID, Source: source}
	}

	result.Similarity, err = cosineSimilarity(vectors[0], vectors[1])
	if err != nil {
		return FaceComparison{}, err
	}
	return result, nil
}

func codificationEmbedding(ctx context.Context, q *db.Queries, store *storage.Client, vis FaceVision, criminalCaseID, codificationID int64) ([]float64, string, error) {
	row, err := q.GetCodificationForComparison(ctx, db.GetCodificationForComparisonParams{
		EmbeddingType:  embedding.EmbeddingType,
		CodificationID: codificationID,
		CriminalCaseID: criminalCaseID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if row.CodificationType != graph.CodificationTypeFaceEmbedding {
		return nil, "", ErrNotFaceCodification
	}

	if row.Embedding != "" {
		vector, err := parseVector(row.Embedding)
		if err != nil {
			return nil, "", fmt.Errorf("cases: parse stored embedding for codification %d: %w", codificationID, err)
		}
		return vector, EmbeddingSourceStored, nil
	}

	if vis == nil {
		return nil, "", ErrVisionUnavailable
	}
	if store == nil {
		return nil, "", fmt.Errorf("cases: object storage is not configured")
	}

	source, storageRef := EmbeddingSourceCodificationImage, row.CodificationStorageRef
	if !storageRef.Valid {
		source, storageRef = EmbeddingSourceEvidence, row.EvidenceStorageRef
	}
	if !storageRef.Valid {
		return nil, "", NoFaceError{CodificationID: codificationID}
	}

	data, err := store.Download(ctx, storageRef.String)
	if err != nil {
		return nil, "", fmt.Errorf("cases: download image for codification %d: %w", codificationID, err)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, "", ErrUnsupportedImage
	}

	// A codification image is already cropped to the face; an evidence image
	// is narrowed to the trace's box.
	box := img.Bounds()
	if source == EmbeddingSourceEvidence {
		box = image.Rect(
			int(math.Floor(row.BoxX1.Float64)),
			int(math.Floor(row.BoxY1.Float64)),
			int(math.Ceil(row.BoxX2.Float64)),
			int(math.Ceil(row.BoxY2.Float64)),
		).Add(img.Bounds().Min)
	}
	if box.Empty() {
		return nil, "", NoFaceError{CodificationID: codificationID}
	}

	// Same padded pipeline the worker stores embeddings with, so a vector
	// computed here matches the one that later lands in feature_embeddings.
	embedded, err := embedding.EmbedFaceInBox(vis, img, box)
	if err != nil {
		return nil, "", fmt.Errorf("cases: embed face for codification %d: %w", codificationID, err)
	}
	if embedded == nil {
		return nil, "", NoFaceError{CodificationID: codificationID}
	}

	vector := make([]float64, len(embedded))
	for i, v := range embedded {
		vector[i] = float64(v)
	}
	return vector, source, nil
}

// parseVector reads pgvector's text format, "[v1,v2,...]".
func parseVector(text string) ([]float64, error) {
	parts := strings.Split(strings.Trim(text, "[]"), ",")
	vector := make([]float64, len(parts))
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, err
		}
		vector[i] = v
	}
	return vector, nil
}

func cosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) || len(a) == 0 {
		return 0, fmt.Errorf("cases: embeddings have different sizes (%d, %d)", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0, fmt.Errorf("cases: embedding has zero length")
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}
