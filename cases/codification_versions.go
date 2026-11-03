package cases

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/sqlc-dev/pqtype"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/imageproc"
	"github.com/rodfileto/trackid/storage"
)

// Codification versions (case_codifications, db/migrations/005_cases.sql): a face can have several
// codifications, the original and versions rendered from it with a recipe (imageproc.FaceRecipe:
// mirror, rotation, scale, colour balance, grey, brightness/contrast, levels, curves), to see how
// a recognition system takes the face. Each has its own embedding; the matcher uses the one chosen
// for recognition, the original while none is chosen.

// Errors of codification versions.
var (
	ErrVersionNotFound    = errors.New("cases: codification version not found")
	ErrVersionInUse       = errors.New("cases: the original and the version used for recognition cannot be deleted")
	ErrVersionsFacialOnly = errors.New("cases: codification versions are for faces")
)

// versionCropPadding widens the face box on each side, by this fraction of its size, for the
// rendered crop: the detector needs some of the head around the face to find it again.
const versionCropPadding = 0.5

// MaxVersionLabel bounds a version's label, in bytes.
const MaxVersionLabel = 200

// CodificationVersion is one codification of a face, as the versions editor lists it.
type CodificationVersion struct {
	ID       int64  `json:"id"`
	Sequence int16  `json:"sequence"`
	Label    string `json:"label,omitempty"`
	// Recipe is how the version was rendered from the evidence image; nil for the original.
	Recipe *imageproc.FaceRecipe `json:"recipe,omitempty"`
	// Record describes the processing in words, for a report (imageproc.DescribeFace).
	Record string `json:"record,omitempty"`
	// HasImage says the codification has its own image (a render, or an image saved for it).
	HasImage    bool   `json:"hasImage"`
	ImageSHA256 string `json:"imageSha256,omitempty"`
	// HasEmbedding says the worker has computed its embedding (false: not yet, or no face found).
	HasEmbedding bool `json:"hasEmbedding"`
	// ForRecognition marks the version the matcher and clustering use for this face.
	ForRecognition bool      `json:"forRecognition"`
	CreatedBy      *Author   `json:"createdBy,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Author is who made something.
type Author struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// faceTrace scopes a trace to its case and evidence file and returns the case row id and the trace
// type, refusing traces that are not faces.
func faceTrace(ctx context.Context, q *db.Queries, caseID string, evidenceFileID, traceID int64) (int64, error) {
	caseRow, err := q.GetBiometricCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	tr, err := q.GetCaseTraceForFile(ctx, db.GetCaseTraceForFileParams{ID: traceID, BiometricCaseID: caseRow.ID,
		CaseFileID: sql.NullInt64{Int64: evidenceFileID, Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if t, ok := graph.CodificationTypeForTraceType(tr.TraceType); !ok || t != graph.CodificationTypeFaceEmbedding {
		return 0, ErrVersionsFacialOnly
	}
	return caseRow.ID, nil
}

// ListCodificationVersions returns a face's codifications, the original first.
func ListCodificationVersions(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID int64) ([]CodificationVersion, error) {
	q := db.New(sqlDB)
	caseRowID, err := faceTrace(ctx, q, caseID, evidenceFileID, traceID)
	if err != nil {
		return nil, err
	}
	// The original exists from the moment the face was marked; make sure of it for older rows.
	if _, _, err := resolveCodification(ctx, q, caseRowID, evidenceFileID, traceID); err != nil {
		return nil, err
	}
	rows, err := q.ListTraceCodifications(ctx, traceID)
	if err != nil {
		return nil, err
	}
	out := make([]CodificationVersion, 0, len(rows))
	for _, r := range rows {
		v := CodificationVersion{ID: r.ID, Sequence: r.Sequence, Label: r.Label.String, HasImage: r.CaseFileID.Valid,
			ImageSHA256: r.ImageSha256.String, HasEmbedding: r.HasEmbedding, ForRecognition: r.ForRecognition, CreatedAt: r.CreatedAt}
		if r.Recipe.Valid {
			var recipe imageproc.FaceRecipe
			if err := json.Unmarshal(r.Recipe.RawMessage, &recipe); err == nil {
				v.Recipe = &recipe
			}
		}
		if r.CreatedByID.Valid {
			v.CreatedBy = &Author{ID: r.CreatedByID.Int64, Username: r.CreatedByUsername.String, Name: r.CreatedByName}
		}
		out = append(out, v)
	}
	return out, nil
}

// RenderedVersion is a recipe rendered on a face, not yet saved.
type RenderedVersion struct {
	Result imageproc.FaceResult
	Recipe imageproc.FaceRecipe
	Record string
}

// RenderCodificationVersion renders recipe on the face of a trace: the evidence image cropped to
// the face box widened by versionCropPadding (the crop is always the server's, whatever the recipe
// says), then the recipe's geometry and tones.
func RenderCodificationVersion(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, evidenceFileID, traceID int64,
	recipe imageproc.FaceRecipe, examiner string) (RenderedVersion, error) {
	if store == nil {
		return RenderedVersion{}, fmt.Errorf("cases: object storage is not configured")
	}
	q := db.New(sqlDB)
	caseRowID, err := faceTrace(ctx, q, caseID, evidenceFileID, traceID)
	if err != nil {
		return RenderedVersion{}, err
	}
	content, err := DownloadEvidence(ctx, sqlDB, store, caseID, evidenceFileID)
	if err != nil {
		return RenderedVersion{}, err
	}
	original, err := imageproc.Decode(content.Data)
	if err != nil {
		return RenderedVersion{}, ErrNotImage
	}
	traces, err := q.ListCaseTracesByCaseFile(ctx, db.ListCaseTracesByCaseFileParams{BiometricCaseID: caseRowID,
		CaseFileID: sql.NullInt64{Int64: evidenceFileID, Valid: true}})
	if err != nil {
		return RenderedVersion{}, err
	}
	var crop *imageproc.Rect
	for _, t := range traces {
		if t.ID == traceID && t.BoxX1.Valid {
			w, h := t.BoxX2.Float64-t.BoxX1.Float64, t.BoxY2.Float64-t.BoxY1.Float64
			crop = &imageproc.Rect{X1: t.BoxX1.Float64 - w*versionCropPadding, Y1: t.BoxY1.Float64 - h*versionCropPadding,
				X2: t.BoxX2.Float64 + w*versionCropPadding, Y2: t.BoxY2.Float64 + h*versionCropPadding}
			crop.X1, crop.Y1 = math.Round(crop.X1), math.Round(crop.Y1)
			crop.X2, crop.Y2 = math.Round(crop.X2), math.Round(crop.Y2)
		}
	}
	if crop == nil {
		return RenderedVersion{}, ErrNotFound
	}
	recipe.Crop = crop
	if err := recipe.Normalize(original.Width, original.Height); err != nil {
		return RenderedVersion{}, err
	}
	res, err := imageproc.RenderFace(original, recipe)
	if err != nil {
		return RenderedVersion{}, err
	}
	record := imageproc.DescribeFace(imageproc.FaceRecord{Filename: content.Filename, Original: original, Recipe: recipe, Result: res,
		Examiner: examiner, RenderedAt: time.Now()})
	return RenderedVersion{Result: res, Recipe: recipe, Record: record}, nil
}

// CreateCodificationVersion renders recipe on a face and saves it as a new codification of it,
// with the render as its image, and queues its embedding.
func CreateCodificationVersion(ctx context.Context, sqlDB *sql.DB, store *storage.Client, queue embedding.Enqueuer, caseID string,
	evidenceFileID, traceID int64, recipe imageproc.FaceRecipe, label string, userID int64, examiner string) (CodificationVersion, error) {
	if len(label) > MaxVersionLabel {
		return CodificationVersion{}, ValidationError("the label is too long")
	}
	rendered, err := RenderCodificationVersion(ctx, sqlDB, store, caseID, evidenceFileID, traceID, recipe, examiner)
	if err != nil {
		return CodificationVersion{}, err
	}
	sum := sha256.Sum256(rendered.Result.PNG)
	hash := hex.EncodeToString(sum[:])
	ref, err := store.Upload(ctx, fmt.Sprintf("codification/%s/%d/%s", caseID, traceID, hash), rendered.Result.PNG, "image/png")
	if err != nil {
		return CodificationVersion{}, fmt.Errorf("cases: upload codification version: %w", err)
	}
	recipeJSON, err := json.Marshal(rendered.Recipe)
	if err != nil {
		return CodificationVersion{}, err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return CodificationVersion{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	caseRow, err := q.GetBiometricCaseByCaseID(ctx, caseID)
	if err != nil {
		return CodificationVersion{}, err
	}
	if _, _, err := resolveCodification(ctx, q, caseRow.ID, evidenceFileID, traceID); err != nil { // the original first
		return CodificationVersion{}, err
	}
	next, err := q.NextCodificationSequence(ctx, traceID)
	if err != nil {
		return CodificationVersion{}, err
	}
	file, err := q.UpsertCaseFile(ctx, db.UpsertCaseFileParams{
		BiometricCaseID: caseRow.ID, Category: "codification_image",
		MediaType: sql.NullString{String: "image", Valid: true}, HashID: sql.NullString{String: hash, Valid: true},
		Filename:   sql.NullString{String: fmt.Sprintf("trace-%d-codification-%d.png", traceID, next), Valid: true},
		StorageRef: sql.NullString{String: ref, Valid: true}, ContentType: sql.NullString{String: "image/png", Valid: true},
		SizeBytes: sql.NullInt64{Int64: int64(len(rendered.Result.PNG)), Valid: true},
	})
	if err != nil {
		return CodificationVersion{}, err
	}
	id, err := q.InsertCodificationVersion(ctx, db.InsertCodificationVersionParams{
		TraceID: traceID, Sequence: next, CodificationType: graph.CodificationTypeFaceEmbedding,
		Label:      sql.NullString{String: label, Valid: label != ""},
		Recipe:     pqtype.NullRawMessage{RawMessage: recipeJSON, Valid: true},
		CreatedBy:  sql.NullInt64{Int64: userID, Valid: userID > 0},
		CaseFileID: sql.NullInt64{Int64: file.ID, Valid: true},
	})
	if err != nil {
		return CodificationVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return CodificationVersion{}, err
	}
	enqueueCodificationTasks(queue, graph.CodificationTypeFaceEmbedding, []int64{id})

	versions, err := ListCodificationVersions(ctx, sqlDB, caseID, evidenceFileID, traceID)
	if err != nil {
		return CodificationVersion{}, err
	}
	for _, v := range versions {
		if v.ID == id {
			v.Record = rendered.Record
			return v, nil
		}
	}
	return CodificationVersion{}, ErrVersionNotFound
}

// SetRecognitionVersion makes a codification the one the matcher and clustering use for its face:
// its embedding becomes the face's (matched again on the next sync), or, while it has none yet, the
// worker computes it and then it does.
func SetRecognitionVersion(ctx context.Context, sqlDB *sql.DB, queue embedding.Enqueuer, caseID string, evidenceFileID, traceID, codificationID int64) error {
	q := db.New(sqlDB)
	if _, err := faceTrace(ctx, q, caseID, evidenceFileID, traceID); err != nil {
		return err
	}
	if _, err := q.GetTraceCodification(ctx, db.GetTraceCodificationParams{ID: codificationID, TraceID: traceID}); errors.Is(err, sql.ErrNoRows) {
		return ErrVersionNotFound
	} else if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qt := db.New(tx)
	if err := qt.SetTraceRecognitionCodification(ctx, db.SetTraceRecognitionCodificationParams{ID: traceID,
		RecognitionCodificationID: sql.NullInt64{Int64: codificationID, Valid: true}}); err != nil {
		return err
	}
	emb, err := qt.GetCodificationEmbedding(ctx, db.GetCodificationEmbeddingParams{CodificationID: codificationID, EmbeddingType: embedding.EmbeddingType})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := tx.Commit(); err != nil {
			return err
		}
		enqueueCodificationTasks(queue, graph.CodificationTypeFaceEmbedding, []int64{codificationID})
		return nil
	case err != nil:
		return err
	}
	src, err := qt.GetCodificationSource(ctx, codificationID)
	if err != nil {
		return err
	}
	if _, err := qt.UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{BiometricFeatureID: src.BiometricFeatureID,
		EmbeddingType: embedding.EmbeddingType, Embedding: emb.Embedding, ModelVersion: emb.ModelVersion}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	embedding.EnqueueSyncFace(queue)
	return nil
}

// DeleteCodificationVersion removes a rendered version that is not the one used for recognition.
func DeleteCodificationVersion(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID, codificationID int64) error {
	q := db.New(sqlDB)
	if _, err := faceTrace(ctx, q, caseID, evidenceFileID, traceID); err != nil {
		return err
	}
	if _, err := q.GetTraceCodification(ctx, db.GetTraceCodificationParams{ID: codificationID, TraceID: traceID}); errors.Is(err, sql.ErrNoRows) {
		return ErrVersionNotFound
	} else if err != nil {
		return err
	}
	n, err := q.DeleteCodificationVersion(ctx, db.DeleteCodificationVersionParams{ID: codificationID, TraceID: traceID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionInUse
	}
	return nil
}

// CodificationVersionImage returns a codification's own image, when it has one.
func CodificationVersionImage(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, evidenceFileID, traceID, codificationID int64) ([]byte, string, error) {
	if store == nil {
		return nil, "", fmt.Errorf("cases: object storage is not configured")
	}
	q := db.New(sqlDB)
	if _, err := faceTrace(ctx, q, caseID, evidenceFileID, traceID); err != nil {
		return nil, "", err
	}
	c, err := q.GetTraceCodification(ctx, db.GetTraceCodificationParams{ID: codificationID, TraceID: traceID})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !c.StorageRef.Valid) {
		return nil, "", ErrVersionNotFound
	}
	if err != nil {
		return nil, "", err
	}
	data, err := store.Download(ctx, c.StorageRef.String)
	if err != nil {
		return nil, "", err
	}
	return data, c.ContentType.String, nil
}
