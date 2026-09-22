package fingerprint

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
	"time"

	"github.com/hibiken/asynq"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/storage"
)

// TemplateType/ModelVersion identify the templates this package stores in
// biometric_templates. TemplateType is also the system_source on the SYSTEM
// decisions they produce.
const (
	TemplateType = "FINGERPRINT_SOURCEAFIS"
	ModelVersion = "sourceafis:3.18.1"
)

// MatchThreshold is the SourceAFIS score at or above which RunSync records a
// SYSTEM POSITIVE decision: SourceAFIS's documented threshold, stated as a
// false match rate of 0.01%. On trackid-sim's generated prints (tools/fpscore)
// every rolled genuine pair clears it (median 673), latent-vs-rolled genuine
// pairs only 39.5% of the time (median 13.9), and 0.08% of rolled impostors do.
const MatchThreshold = 40.0

// Asynq task types cmd/worker registers handlers for.
const (
	// TaskTypeExtractCodification extracts a QUESTIONED fingerprint lift's
	// template (a MINUTIAE codification), enqueued where
	// embedding.TaskTypeComputeCodification is for a face.
	TaskTypeExtractCodification = "fingerprint:extract_codification"
	// TaskTypeExtractIdentityFeature extracts a KNOWN ten-print's template
	// (a FINGERPRINT_TEMPLATE biometricfeature), enqueued by identity.Ingest.
	TaskTypeExtractIdentityFeature = "fingerprint:extract_identity_feature"
	// TaskTypeSync runs matching (and clustering, when Neo4j is available)
	// after a template lands. See RunSync.
	TaskTypeSync = "fingerprint:sync"
)

// Enqueuer is the one asynq.Client method producers need.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// ExtractCodificationPayload is TaskTypeExtractCodification's JSON payload.
type ExtractCodificationPayload struct {
	CodificationID int64 `json:"codificationId"`
}

// ExtractIdentityFeaturePayload is TaskTypeExtractIdentityFeature's JSON payload.
type ExtractIdentityFeaturePayload struct {
	BiometricFeatureID int64 `json:"biometricFeatureId"`
}

// NewExtractCodificationTask builds the task enqueued for codificationID.
func NewExtractCodificationTask(codificationID int64) (*asynq.Task, error) {
	payload, err := json.Marshal(ExtractCodificationPayload{CodificationID: codificationID})
	if err != nil {
		return nil, fmt.Errorf("fingerprint: marshal task payload: %w", err)
	}
	return asynq.NewTask(TaskTypeExtractCodification, payload), nil
}

// NewExtractIdentityFeatureTask builds the task enqueued for biometricFeatureID.
func NewExtractIdentityFeatureTask(biometricFeatureID int64) (*asynq.Task, error) {
	payload, err := json.Marshal(ExtractIdentityFeaturePayload{BiometricFeatureID: biometricFeatureID})
	if err != nil {
		return nil, fmt.Errorf("fingerprint: marshal task payload: %w", err)
	}
	return asynq.NewTask(TaskTypeExtractIdentityFeature, payload), nil
}

// HandleExtractCodification returns the asynq.Handler for
// TaskTypeExtractCodification. queue may be nil (no follow-up sync is
// enqueued then).
func HandleExtractCodification(sqlDB *sql.DB, store *storage.Client, client *Client, queue Enqueuer) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload ExtractCodificationPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: unmarshal task payload: %w", asynq.SkipRetry, err)
		}
		ok, err := ExtractForCodification(ctx, sqlDB, store, client, payload.CodificationID)
		if err != nil {
			return err
		}
		if ok {
			enqueueSync(queue)
		}
		return nil
	}
}

// HandleExtractIdentityFeature returns the asynq.Handler for
// TaskTypeExtractIdentityFeature. queue may be nil.
func HandleExtractIdentityFeature(sqlDB *sql.DB, store *storage.Client, client *Client, queue Enqueuer) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload ExtractIdentityFeaturePayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: unmarshal task payload: %w", asynq.SkipRetry, err)
		}
		ok, err := ExtractForIdentityFeature(ctx, sqlDB, store, client, payload.BiometricFeatureID)
		if err != nil {
			return err
		}
		if ok {
			enqueueSync(queue)
		}
		return nil
	}
}

// ExtractForCodification resolves a MINUTIAE codification's source image
// the way embedding.ComputeForCodification does (GetCodificationSource): the
// analyst's codification image, else the trace's own crop, else the evidence
// image cut down to the trace's box. It extracts a template from it and
// upserts it into biometric_templates for the trace's biometricfeature.
//
// ok is false with a nil error when there is nothing to do: a
// non-MINUTIAE codification, or no image yet. An image the sidecar can't
// use fails with asynq.SkipRetry.
func ExtractForCodification(ctx context.Context, sqlDB *sql.DB, store *storage.Client, client *Client, codificationID int64) (ok bool, err error) {
	if sqlDB == nil {
		return false, fmt.Errorf("fingerprint: nil db")
	}
	if store == nil {
		return false, fmt.Errorf("fingerprint: object storage is not configured")
	}
	if client == nil {
		return false, fmt.Errorf("fingerprint: sidecar is not configured")
	}

	q := db.New(sqlDB)
	row, err := q.GetCodificationSource(ctx, codificationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("fingerprint: get codification %d source: %w", codificationID, err)
	}
	if row.CodificationType != graph.CodificationTypeMinutiae {
		return false, nil
	}

	// A dedicated image is just the lift and goes to the sidecar as stored,
	// in whatever format it is (SourceAFIS also reads WSQ, which Go can't).
	// The evidence image is cut down to the trace's box first.
	storageRef, cropToBox := row.CodificationStorageRef, false
	if !storageRef.Valid {
		storageRef = row.TraceCropStorageRef
	}
	if !storageRef.Valid {
		storageRef, cropToBox = row.EvidenceStorageRef, true
		if !row.BoxX1.Valid {
			return false, nil
		}
	}
	if !storageRef.Valid {
		return false, nil
	}

	data, err := store.Download(ctx, storageRef.String)
	if err != nil {
		return false, fmt.Errorf("fingerprint: download codification %d source image: %w", codificationID, err)
	}
	if cropToBox {
		box := image.Rect(
			int(math.Floor(row.BoxX1.Float64)), int(math.Floor(row.BoxY1.Float64)),
			int(math.Ceil(row.BoxX2.Float64)), int(math.Ceil(row.BoxY2.Float64)),
		)
		data, err = cropPNG(data, box)
		if err != nil {
			return false, fmt.Errorf("%w: fingerprint: codification %d: %w", asynq.SkipRetry, codificationID, err)
		}
		if data == nil {
			return false, nil
		}
	}

	if err := extractAndStore(ctx, q, client, row.BiometricfeatureID, data); err != nil {
		return false, fmt.Errorf("fingerprint: codification %d: %w", codificationID, err)
	}
	return true, nil
}

// ExtractForIdentityFeature extracts the template of a KNOWN
// FINGERPRINT_TEMPLATE biometricfeature from its identity_file (see
// GetIdentityFeatureSource), the KNOWN-side counterpart to
// ExtractForCodification. The file goes to the sidecar as stored: an image
// of one finger in any format SourceAFIS reads. A multi-finger NIST record
// is not split into fingers here.
//
// ok is false with a nil error for a feature of another type. An image the
// sidecar can't use fails with asynq.SkipRetry.
func ExtractForIdentityFeature(ctx context.Context, sqlDB *sql.DB, store *storage.Client, client *Client, biometricFeatureID int64) (ok bool, err error) {
	if sqlDB == nil {
		return false, fmt.Errorf("fingerprint: nil db")
	}
	if store == nil {
		return false, fmt.Errorf("fingerprint: object storage is not configured")
	}
	if client == nil {
		return false, fmt.Errorf("fingerprint: sidecar is not configured")
	}

	q := db.New(sqlDB)
	row, err := q.GetIdentityFeatureSource(ctx, biometricFeatureID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("fingerprint: get identity feature %d source: %w", biometricFeatureID, err)
	}
	if row.FeatureType != graph.FeatureTypeFingerprintTemplate {
		return false, nil
	}

	data, err := store.Download(ctx, row.StorageRef)
	if err != nil {
		return false, fmt.Errorf("fingerprint: download identity feature %d source image: %w", biometricFeatureID, err)
	}
	if err := extractAndStore(ctx, q, client, row.BiometricfeatureID, data); err != nil {
		return false, fmt.Errorf("fingerprint: identity feature %d: %w", biometricFeatureID, err)
	}
	return true, nil
}

func extractAndStore(ctx context.Context, q *db.Queries, client *Client, biometricFeatureID int64, image []byte) error {
	template, err := client.Extract(ctx, image)
	if err != nil {
		if errors.Is(err, ErrUnusableImage) {
			return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
		}
		return err
	}
	if _, err := q.UpsertBiometricTemplate(ctx, db.UpsertBiometricTemplateParams{
		BiometricfeatureID: biometricFeatureID,
		TemplateType:       TemplateType,
		Template:           template,
		ModelVersion:       sql.NullString{String: ModelVersion, Valid: true},
	}); err != nil {
		return fmt.Errorf("store biometric_templates for biometricfeature %d: %w", biometricFeatureID, err)
	}
	return nil
}

// cropPNG decodes data (PNG or JPEG), cuts it to box (in image pixel
// coordinates relative to its top-left corner), and re-encodes the crop as
// PNG. It returns nil, nil when box doesn't overlap the image.
func cropPNG(data []byte, box image.Rectangle) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode evidence image: %w", err)
	}
	box = box.Add(img.Bounds().Min).Intersect(img.Bounds())
	if box.Empty() {
		return nil, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(crop, crop.Bounds(), img, box.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, crop); err != nil {
		return nil, fmt.Errorf("encode crop: %w", err)
	}
	return buf.Bytes(), nil
}

// syncDebounce collapses a burst of extractions (e.g. a bulk import) into one
// queued sync, as embedding's syncFaceDebounce does.
const syncDebounce = 30 * time.Second

// NewSyncTask builds the payload-less task that triggers RunSync.
func NewSyncTask() *asynq.Task {
	return asynq.NewTask(TaskTypeSync, nil)
}

func enqueueSync(queue Enqueuer) {
	if queue == nil {
		return
	}
	if _, err := queue.Enqueue(NewSyncTask(), asynq.Unique(syncDebounce)); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		log.Printf("fingerprint: enqueue %s: %v", TaskTypeSync, err)
	}
}

// HandleSync returns the asynq.Handler for TaskTypeSync. driver may be nil.
func HandleSync(sqlDB *sql.DB, client *Client, driver neo4j.DriverWithContext) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		return RunSync(ctx, sqlDB, client, driver)
	}
}

// RunSync scores every unmatched template (biometricmatch.RunTemplates at
// MatchThreshold), then reconciles clusters with cluster.Run when driver is
// non-nil. Unlike embedding.SyncFace, matching doesn't wait for Neo4j: the
// decisions are written either way, and any later cluster.Run (a face sync,
// cmd/cluster-biometrics) picks them up.
func RunSync(ctx context.Context, sqlDB *sql.DB, client *Client, driver neo4j.DriverWithContext) error {
	if _, err := biometricmatch.RunTemplates(ctx, sqlDB, client, TemplateType, MatchThreshold, TemplateType); err != nil {
		return fmt.Errorf("fingerprint: sync match: %w", err)
	}
	if driver == nil {
		return nil
	}
	if _, err := cluster.Run(ctx, sqlDB, driver); err != nil {
		return fmt.Errorf("fingerprint: sync cluster: %w", err)
	}
	return nil
}
