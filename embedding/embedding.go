// Package embedding computes a codification's biometric embedding vector via
// trackid-vision and stores it in feature_embeddings, the same table
// cmd/backfill-face-embeddings and cmd/match-embeddings already read/write.
// It also defines the Asynq task that runs this asynchronously from
// cmd/worker, enqueued whenever a codification gets an image to embed --
// either a trace freshly marked and Codify'd (cases.CreateTraces) or an
// existing one whose image is (re)saved (cases.SaveCodificationImage).
package embedding

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/storage"
)

// EmbeddingType/ModelVersion identify the vectors this package produces --
// same embedding_type cmd/backfill-face-embeddings writes for the legacy
// AuraFace-v1 vectors (see migration 017), so both live in one comparable
// space and cmd/match-embeddings -embedding-type=FACE_AURAFACE_512 matches
// across them.
const (
	EmbeddingType = "FACE_AURAFACE_512"
	ModelVersion  = "auraface:v1"
)

// TaskTypeComputeCodification is the Asynq task type cmd/worker registers a
// handler for (see HandleComputeCodification) and producers enqueue (see
// NewComputeCodificationTask).
const TaskTypeComputeCodification = "embedding:compute_codification"

// ComputeCodificationPayload is TaskTypeComputeCodification's JSON payload.
type ComputeCodificationPayload struct {
	CodificationID int64 `json:"codificationId"`
}

// NewComputeCodificationTask builds the task enqueued for codificationID.
func NewComputeCodificationTask(codificationID int64) (*asynq.Task, error) {
	payload, err := json.Marshal(ComputeCodificationPayload{CodificationID: codificationID})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal task payload: %w", err)
	}
	return asynq.NewTask(TaskTypeComputeCodification, payload), nil
}

// Enqueuer is the one asynq.Client method producers need -- letting callers
// (e.g. cases.SaveCodificationImage) depend on this narrow interface instead
// of asynq.Client directly.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// HandleComputeCodification returns the asynq.Handler cmd/worker registers
// for TaskTypeComputeCodification, bound to the given dependencies.
func HandleComputeCodification(sqlDB *sql.DB, store *storage.Client, vis *vision.Service) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload ComputeCodificationPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: unmarshal task payload: %w", asynq.SkipRetry, err)
		}
		_, err := ComputeForCodification(ctx, sqlDB, store, vis, payload.CodificationID)
		return err
	}
}

// ComputeForCodification resolves codificationID's source image, in priority
// order (see the GetCodificationSource query): the manually adjusted
// codification_image, the trace's own face_crop (an automated import
// pipeline that already produced one), or the evidence image plus the
// trace's own box -- what a trace marked by hand and Codify'd gets, with
// neither of the above. Either way the face is presented to the detector
// padded with context (see EmbedFaceInBox: a tight crop, on its own, is
// close to undetectable), and the highest-scoring face's embedding is
// upserted into feature_embeddings for the codification's biometricfeature.
//
// ok is false with a nil error for every legitimate "nothing to do" case --
// a MINUTIAE (fingerprint) codification, one with no image to embed yet
// (neither a dedicated crop nor even an evidence image), or an image with no
// detectable face -- so callers (and Asynq's retry policy) don't treat those
// as failures. An image trackid-vision can't decode fails with
// asynq.SkipRetry: retrying won't change the file.
func ComputeForCodification(ctx context.Context, sqlDB *sql.DB, store *storage.Client, vis *vision.Service, codificationID int64) (ok bool, err error) {
	if sqlDB == nil {
		return false, fmt.Errorf("embedding: nil db")
	}
	if store == nil {
		return false, fmt.Errorf("embedding: object storage is not configured")
	}
	if vis == nil {
		return false, fmt.Errorf("embedding: vision service is not configured")
	}

	q := db.New(sqlDB)
	row, err := q.GetCodificationSource(ctx, codificationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("embedding: get codification %d source: %w", codificationID, err)
	}
	if row.CodificationType != graph.CodificationTypeFaceEmbedding {
		return false, nil
	}

	// A dedicated crop (codification image or trace face_crop) already is
	// just the face -- embed the whole thing. Falling back to the evidence
	// image, the box narrows the whole photo down to this one trace's face.
	storageRef, box := row.CodificationStorageRef, image.Rectangle{}
	if !storageRef.Valid {
		storageRef = row.TraceCropStorageRef
	}
	if !storageRef.Valid {
		storageRef = row.EvidenceStorageRef
		if !row.BoxX1.Valid {
			return false, nil
		}
		box = image.Rect(
			int(math.Floor(row.BoxX1.Float64)), int(math.Floor(row.BoxY1.Float64)),
			int(math.Ceil(row.BoxX2.Float64)), int(math.Ceil(row.BoxY2.Float64)),
		)
	}
	if !storageRef.Valid {
		return false, nil
	}

	data, err := store.Download(ctx, storageRef.String)
	if err != nil {
		return false, fmt.Errorf("embedding: download codification %d source image: %w", codificationID, err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("%w: embedding: decode codification %d source image: %w", asynq.SkipRetry, codificationID, err)
	}
	if box.Empty() {
		box = img.Bounds()
	} else {
		box = box.Add(img.Bounds().Min).Intersect(img.Bounds())
		if box.Empty() {
			return false, nil
		}
	}

	vector, err := EmbedFaceInBox(vis, img, box)
	if err != nil {
		return false, fmt.Errorf("embedding: detect+embed codification %d: %w", codificationID, err)
	}
	if vector == nil {
		return false, nil
	}

	if _, err := q.UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{
		BiometricfeatureID: row.BiometricfeatureID,
		EmbeddingType:      EmbeddingType,
		Embedding:          vectorText(vector),
		ModelVersion:       sql.NullString{String: ModelVersion, Valid: true},
	}); err != nil {
		return false, fmt.Errorf("embedding: store feature_embeddings for biometricfeature %d: %w", row.BiometricfeatureID, err)
	}
	return true, nil
}

// vectorText renders v in pgvector's text input format ("[v1,v2,...]"),
// matching UpsertFeatureEmbedding's convention (see its comment in
// db/queries.sql) of casting a plain string rather than using a
// pgvector-aware driver/codec.
func vectorText(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.FormatFloat(float64(x), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
