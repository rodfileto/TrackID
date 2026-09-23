// Package embedding computes a codification's biometric embedding vector via
// trackid-vision and stores it in feature_embeddings, the same table
// cmd/backfill-face-embeddings and cmd/match-embeddings already read/write.
// It also defines the Asynq task that runs this asynchronously from
// cmd/worker, enqueued whenever a codification gets an image to embed --
// either a trace freshly marked and Codify'd (cases.CreateTraces) or an
// existing one whose image is (re)saved (cases.SaveCodificationImage).
//
// It also drives what happens after a FACE embedding lands: SyncFace runs
// biometricmatch.Run and cluster.Run incrementally, so a newly computed
// embedding gets matched and clustered without an operator running
// cmd/match-embeddings/cmd/cluster-biometrics by hand. See
// ComputeForCodification/ComputeForIdentityFeature, the two enqueuers.
// Fingerprint has no such pipeline yet.
package embedding

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cluster"
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
// for TaskTypeComputeCodification, bound to the given dependencies. queue may
// be nil (no follow-up sync-face is enqueued then); cmd/worker always passes
// one since it's already a queue producer for this reason.
func HandleComputeCodification(sqlDB *sql.DB, store *storage.Client, vis *vision.Service, queue Enqueuer) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload ComputeCodificationPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: unmarshal task payload: %w", asynq.SkipRetry, err)
		}
		ok, err := ComputeForCodification(ctx, sqlDB, store, vis, payload.CodificationID)
		if err != nil {
			return err
		}
		if ok {
			enqueueSyncFace(queue)
		}
		return nil
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
		Embedding:          VectorText(vector),
		ModelVersion:       sql.NullString{String: ModelVersion, Valid: true},
	}); err != nil {
		return false, fmt.Errorf("embedding: store feature_embeddings for biometricfeature %d: %w", row.BiometricfeatureID, err)
	}
	return true, nil
}

// TaskTypeComputeIdentityFeature is the Asynq task type cmd/worker registers
// a handler for (see HandleComputeIdentityFeature) and producers enqueue
// (see NewComputeIdentityFeatureTask) -- the KNOWN-side counterpart to
// TaskTypeComputeCodification, enqueued whenever an enrollment brings in a
// new FACE_RECORD identity_file (identity.Ingest).
const TaskTypeComputeIdentityFeature = "embedding:compute_identity_feature"

// ComputeIdentityFeaturePayload is TaskTypeComputeIdentityFeature's JSON
// payload.
type ComputeIdentityFeaturePayload struct {
	BiometricFeatureID int64 `json:"biometricFeatureId"`
}

// NewComputeIdentityFeatureTask builds the task enqueued for
// biometricFeatureID.
func NewComputeIdentityFeatureTask(biometricFeatureID int64) (*asynq.Task, error) {
	payload, err := json.Marshal(ComputeIdentityFeaturePayload{BiometricFeatureID: biometricFeatureID})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal task payload: %w", err)
	}
	return asynq.NewTask(TaskTypeComputeIdentityFeature, payload), nil
}

// HandleComputeIdentityFeature returns the asynq.Handler cmd/worker
// registers for TaskTypeComputeIdentityFeature, bound to the given
// dependencies. queue may be nil (no follow-up sync-face is enqueued then).
func HandleComputeIdentityFeature(sqlDB *sql.DB, store *storage.Client, vis *vision.Service, queue Enqueuer) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload ComputeIdentityFeaturePayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: unmarshal task payload: %w", asynq.SkipRetry, err)
		}
		ok, err := ComputeForIdentityFeature(ctx, sqlDB, store, vis, payload.BiometricFeatureID)
		if err != nil {
			return err
		}
		if ok {
			enqueueSyncFace(queue)
		}
		return nil
	}
}

// ComputeForIdentityFeature resolves biometricFeatureID's identity_file
// photo (see GetIdentityFeatureSource) and embeds it, the KNOWN-side
// counterpart to ComputeForCodification. Unlike a codification, an
// identity_file has no crop/box indirection -- the enrolled photo itself is
// the face record -- so it's presented to the detector whole (see
// EmbedFaceInBox: "pass img.Bounds() as box when the whole image is the
// face"), same as a codification's own dedicated crop is.
//
// ok is false with a nil error for every legitimate "nothing to do" case --
// a non-FACE_RECORD feature (e.g. a fingerprint), or an image with no
// detectable face -- so callers (and Asynq's retry policy) don't treat those
// as failures. An image trackid-vision can't decode fails with
// asynq.SkipRetry: retrying won't change the file.
func ComputeForIdentityFeature(ctx context.Context, sqlDB *sql.DB, store *storage.Client, vis *vision.Service, biometricFeatureID int64) (ok bool, err error) {
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
	row, err := q.GetIdentityFeatureSource(ctx, biometricFeatureID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("embedding: get identity feature %d source: %w", biometricFeatureID, err)
	}
	if row.FeatureType != graph.FeatureTypeFaceRecord {
		return false, nil
	}

	data, err := store.Download(ctx, row.StorageRef)
	if err != nil {
		return false, fmt.Errorf("embedding: download identity feature %d source image: %w", biometricFeatureID, err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("%w: embedding: decode identity feature %d source image: %w", asynq.SkipRetry, biometricFeatureID, err)
	}

	vector, err := EmbedFaceInBox(vis, img, img.Bounds())
	if err != nil {
		return false, fmt.Errorf("embedding: detect+embed identity feature %d: %w", biometricFeatureID, err)
	}
	if vector == nil {
		return false, nil
	}

	if _, err := q.UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{
		BiometricfeatureID: row.BiometricfeatureID,
		EmbeddingType:      EmbeddingType,
		Embedding:          VectorText(vector),
		ModelVersion:       sql.NullString{String: ModelVersion, Valid: true},
	}); err != nil {
		return false, fmt.Errorf("embedding: store feature_embeddings for biometricfeature %d: %w", biometricFeatureID, err)
	}
	return true, nil
}

// TaskTypeSyncFace is the Asynq task type cmd/worker registers a handler for
// (see HandleSyncFace). ComputeForCodification/ComputeForIdentityFeature's
// handlers enqueue it (via enqueueSyncFace) whenever they land a new FACE
// embedding -- the automatic, incremental counterpart to an operator running
// cmd/match-embeddings then cmd/cluster-biometrics by hand.
const TaskTypeSyncFace = "embedding:sync_face"

// syncFaceDebounce is the asynq.Unique TTL enqueueSyncFace uses.
// biometricmatch.Run only ever looks at feature_embeddings rows not yet
// matched, so a SyncFace run triggered moments after the last one just finds
// the same still-unmatched backlog; cluster.Run also rescans every
// biometricfeature and confirmed decision on each call, so collapsing a
// burst of embeddings (e.g. a bulk import) into one run matters more as the
// dataset grows.
const syncFaceDebounce = 30 * time.Second

// DefaultFaceBand is what this automatic pipeline applies while no face
// threshold version has been set in match_thresholds: similarity 0.6 (cosine
// distance 0.4), no review band -- the cutoff it has always used. A deploying
// organization sets its own, from its own validation, with cmd/match-threshold
// (biometricmatch.SetBand); each version is recorded with its provenance, so
// what actually runs continuously is never an unexplained value. Deliberately
// separate from cmd/match-embeddings' own -threshold flag, which stays for an
// operator's ad-hoc runs.
var DefaultFaceBand = biometricmatch.Band{Review: 0.6, Confirm: 0.6}

// NewSyncFaceTask builds the task that triggers SyncFace. It carries no
// payload -- SyncFace always processes whatever is currently unmatched or
// unclustered, never a specific feature -- so every enqueue is identical,
// which is what lets asynq.Unique (see enqueueSyncFace) collapse a burst of
// them into one queued run.
func NewSyncFaceTask() *asynq.Task {
	return asynq.NewTask(TaskTypeSyncFace, nil)
}

// enqueueSyncFace enqueues NewSyncFaceTask, deduped within syncFaceDebounce.
// A duplicate (another sync already queued/running) is expected, not an
// error -- only unexpected Enqueue failures are logged. queue may be nil
// (e.g. a caller with no queue configured), in which case this is a no-op:
// the embedding still gets stored, it just won't be matched/clustered until
// something else triggers a sync.
func enqueueSyncFace(queue Enqueuer) {
	if queue == nil {
		return
	}
	if _, err := queue.Enqueue(NewSyncFaceTask(), asynq.Unique(syncFaceDebounce)); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		log.Printf("embedding: enqueue %s: %v", TaskTypeSyncFace, err)
	}
}

// HandleSyncFace returns the asynq.Handler cmd/worker registers for
// TaskTypeSyncFace, bound to the given dependencies. driver must be non-nil
// -- cluster.Run always materializes to Neo4j -- so cmd/worker only
// registers this handler once Neo4j is reachable.
func HandleSyncFace(sqlDB *sql.DB, driver neo4j.DriverWithContext) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		return SyncFace(ctx, sqlDB, driver)
	}
}

// SyncFace runs biometricmatch.RunBand for FACE_AURAFACE_512 embeddings under
// the current face threshold version (read on every call, so a new version
// applies without a restart; DefaultFaceBand when none is set), then
// cluster.Run to reconcile clusters from whatever confirmed decisions resulted -- the face-only automatic pipeline
// requested; fingerprint has no embedding/matching pipeline to drive yet.
// Safe to call arbitrarily often: matching only ever touches
// feature_embeddings rows not yet matched (marking them matched as it goes),
// and cluster.Run's reconcile is a pure function of the current
// confirmed-decision set, so back-to-back calls with nothing new to do just
// redo the same (cheap, index-backed) reads and write nothing.
func SyncFace(ctx context.Context, sqlDB *sql.DB, driver neo4j.DriverWithContext) error {
	version, err := biometricmatch.CurrentBand(ctx, sqlDB, EmbeddingType, DefaultFaceBand)
	if err != nil {
		return fmt.Errorf("embedding: sync face match: %w", err)
	}
	if _, err := biometricmatch.RunBand(ctx, sqlDB, EmbeddingType, version, EmbeddingType); err != nil {
		return fmt.Errorf("embedding: sync face match: %w", err)
	}
	if _, err := cluster.Run(ctx, sqlDB, driver); err != nil {
		return fmt.Errorf("embedding: sync face cluster: %w", err)
	}
	return nil
}

// VectorText renders v in pgvector's text input format ("[v1,v2,...]"),
// matching UpsertFeatureEmbedding's convention (see its comment in
// db/queries.sql) of casting a plain string rather than using a
// pgvector-aware driver/codec.
func VectorText(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.FormatFloat(float64(x), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
