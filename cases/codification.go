package cases

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/storage"
)

// ErrNoCodification is returned when a trace's trace_type has no
// codification_type mapping (graph.CodificationTypeForTraceType). Shouldn't
// happen given case_traces.trace_type's CHECK constraint, but is guarded
// against rather than assumed.
var ErrNoCodification = errors.New("cases: trace type has no codification")

// ErrPointsNotSupported is returned by ListPoints/AddPoints/UpdatePoint/
// DeletePoint for a trace whose codification isn't MINUTIAE -- manual point
// marking is a FINGERPRINT-only workflow. A FACIAL trace's FACE_EMBEDDING
// codification is a computed vector (feature_embeddings), not something
// marked point by point.
var ErrPointsNotSupported = errors.New("cases: manual points are only supported for fingerprint traces")

// Point is one manually marked point within a trace's codification -- e.g.
// one fingerprint minutia (a ridge ending or bifurcation) for a
// FINGERPRINT_LIFT trace's MINUTIAE codification. PointType/Angle are
// free-form and optional; nothing in this package enforces a vocabulary for
// them.
type Point struct {
	ID        int64   `json:"id"`
	Sequence  int16   `json:"sequence"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	PointType string  `json:"pointType,omitempty"`
	Angle     float64 `json:"angle,omitempty"`
}

// PointInput is one point to add via AddPoints.
type PointInput struct {
	X         float64
	Y         float64
	PointType string
	Angle     float64
}

// CaseCodification is one case_codifications row plus enough about its trace
// (box, sequence) and the trace's evidence file (sequence, id, filename) for
// a caller to render every codification across a whole case -- e.g. every
// face codified in a FACIAL case -- without a request per trace. Label it
// "<evidenceSequence>-<traceSequence>-<sequence>" (the numbering scheme
// analysts use to refer to one codification).
type CaseCodification struct {
	ID               int64   `json:"id"`
	Sequence         int16   `json:"sequence"`
	CodificationType string  `json:"codificationType"`
	CaseFileID       int64   `json:"caseFileId,omitempty"`
	TraceID          int64   `json:"traceId"`
	TraceSequence    int16   `json:"traceSequence"`
	BoxX1            float64 `json:"boxX1"`
	BoxY1            float64 `json:"boxY1"`
	BoxX2            float64 `json:"boxX2"`
	BoxY2            float64 `json:"boxY2"`
	EvidenceSequence int16   `json:"evidenceSequence"`
	EvidenceFileID   int64   `json:"evidenceFileId"`
	EvidenceFilename string  `json:"evidenceFilename"`
}

// ListCaseCodifications returns every codification recorded across every
// trace of a case, in evidence-then-trace-then-codification sequence order.
func ListCaseCodifications(ctx context.Context, sqlDB *sql.DB, caseID string) ([]CaseCodification, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rows, err := q.ListCaseCodificationsByCriminalCase(ctx, caseRow.ID)
	if err != nil {
		return nil, fmt.Errorf("cases: list case_codifications for case %d: %w", caseRow.ID, err)
	}

	codifications := make([]CaseCodification, 0, len(rows))
	for _, row := range rows {
		codifications = append(codifications, CaseCodification{
			ID:               row.CodificationID,
			Sequence:         row.CodificationSequence,
			CodificationType: row.CodificationType,
			CaseFileID:       row.CodificationFileID.Int64,
			TraceID:          row.TraceID,
			TraceSequence:    row.TraceSequence,
			BoxX1:            row.BoxX1.Float64,
			BoxY1:            row.BoxY1.Float64,
			BoxX2:            row.BoxX2.Float64,
			BoxY2:            row.BoxY2.Float64,
			EvidenceSequence: row.EvidenceSequence,
			EvidenceFileID:   row.EvidenceFileID,
			EvidenceFilename: row.EvidenceFilename.String,
		})
	}
	return codifications, nil
}

// resolveCodification scopes traceID to the given case + evidence file (see
// GetCaseTraceForFile), then finds or creates its codification row. Every
// trace gets exactly one codification (sequence 1) -- CreateTraces already
// creates it, so this only actually inserts for a trace that predates that
// (e.g. legacy data backfilled directly into case_traces). Works for any
// modality; callers that are points-only (FINGERPRINT/MINUTIAE) additionally
// call resolveMinutiaeCodification. It also returns the codification_type.
func resolveCodification(ctx context.Context, q *db.Queries, criminalCaseID, evidenceFileID, traceID int64) (int64, string, error) {
	traceRow, err := q.GetCaseTraceForFile(ctx, db.GetCaseTraceForFileParams{
		ID:             traceID,
		CriminalCaseID: criminalCaseID,
		CaseFileID:     sql.NullInt64{Int64: evidenceFileID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", ErrNotFound
		}
		return 0, "", err
	}

	codificationType, ok := graph.CodificationTypeForTraceType(traceRow.TraceType)
	if !ok {
		return 0, "", ErrNoCodification
	}

	codificationID, err := q.GetCaseCodificationByTrace(ctx, traceID)
	if err == nil {
		return codificationID, codificationType, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}

	codificationID, err = q.UpsertCaseCodification(ctx, db.UpsertCaseCodificationParams{
		TraceID:          traceID,
		Sequence:         1,
		CodificationType: codificationType,
	})
	if err != nil {
		return 0, "", fmt.Errorf("cases: create case_codifications for trace %d: %w", traceID, err)
	}
	return codificationID, codificationType, nil
}

// resolveMinutiaeCodification is resolveCodification plus the points-only
// gate: it fails with ErrPointsNotSupported for anything but a MINUTIAE
// (FINGERPRINT) codification.
func resolveMinutiaeCodification(ctx context.Context, q *db.Queries, criminalCaseID, evidenceFileID, traceID int64) (int64, error) {
	traceRow, err := q.GetCaseTraceForFile(ctx, db.GetCaseTraceForFileParams{
		ID:             traceID,
		CriminalCaseID: criminalCaseID,
		CaseFileID:     sql.NullInt64{Int64: evidenceFileID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	codificationType, ok := graph.CodificationTypeForTraceType(traceRow.TraceType)
	if !ok {
		return 0, ErrNoCodification
	}
	if codificationType != graph.CodificationTypeMinutiae {
		return 0, ErrPointsNotSupported
	}
	codificationID, _, err := resolveCodification(ctx, q, criminalCaseID, evidenceFileID, traceID)
	return codificationID, err
}

func pointFromRow(id int64, sequence int16, x, y float64, pointType sql.NullString, angle sql.NullFloat64) Point {
	return Point{
		ID:        id,
		Sequence:  sequence,
		X:         x,
		Y:         y,
		PointType: pointType.String,
		Angle:     angle.Float64,
	}
}

// ListPoints returns every point marked on a trace's codification, in
// sequence order.
func ListPoints(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID int64) ([]Point, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	codificationID, err := resolveMinutiaeCodification(ctx, q, caseRow.ID, evidenceFileID, traceID)
	if err != nil {
		return nil, err
	}

	rows, err := q.ListCodificationPoints(ctx, codificationID)
	if err != nil {
		return nil, fmt.Errorf("cases: list case_codification_points for codification %d: %w", codificationID, err)
	}

	points := make([]Point, 0, len(rows))
	for _, row := range rows {
		points = append(points, pointFromRow(row.ID, row.Sequence, row.X, row.Y, row.PointType, row.Angle))
	}
	return points, nil
}

// AddPoints appends one or more points to a trace's codification, creating
// the codification row on first use (see resolveCodification).
func AddPoints(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID int64, inputs []PointInput) ([]Point, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	q := db.New(tx)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	codificationID, err := resolveMinutiaeCodification(ctx, q, caseRow.ID, evidenceFileID, traceID)
	if err != nil {
		return nil, err
	}

	nextSequence, err := q.MaxCodificationPointSequence(ctx, codificationID)
	if err != nil {
		return nil, fmt.Errorf("cases: max case_codification_points sequence: %w", err)
	}

	points := make([]Point, 0, len(inputs))
	for _, input := range inputs {
		nextSequence++

		pointType := sql.NullString{String: input.PointType, Valid: input.PointType != ""}
		angle := sql.NullFloat64{Float64: input.Angle, Valid: input.Angle != 0}

		pointID, err := q.CreateCodificationPoint(ctx, db.CreateCodificationPointParams{
			CodificationID: codificationID,
			Sequence:       nextSequence,
			X:              input.X,
			Y:              input.Y,
			PointType:      pointType,
			Angle:          angle,
		})
		if err != nil {
			return nil, fmt.Errorf("cases: create case_codification_points (codification %d, sequence %d): %w", codificationID, nextSequence, err)
		}

		points = append(points, Point{
			ID:        pointID,
			Sequence:  nextSequence,
			X:         input.X,
			Y:         input.Y,
			PointType: input.PointType,
			Angle:     input.Angle,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return points, nil
}

// UpdatePoint replaces one point's coordinates/type/angle in place (its
// sequence is unchanged).
func UpdatePoint(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID, pointID int64, input PointInput) error {
	if sqlDB == nil {
		return fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	codificationID, err := resolveMinutiaeCodification(ctx, q, caseRow.ID, evidenceFileID, traceID)
	if err != nil {
		return err
	}

	updated, err := q.UpdateCodificationPoint(ctx, db.UpdateCodificationPointParams{
		ID:             pointID,
		CodificationID: codificationID,
		X:              input.X,
		Y:              input.Y,
		PointType:      sql.NullString{String: input.PointType, Valid: input.PointType != ""},
		Angle:          sql.NullFloat64{Float64: input.Angle, Valid: input.Angle != 0},
	})
	if err != nil {
		return fmt.Errorf("cases: update case_codification_points %d: %w", pointID, err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePoint removes one point from a trace's codification.
func DeletePoint(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID, traceID, pointID int64) error {
	if sqlDB == nil {
		return fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	codificationID, err := resolveMinutiaeCodification(ctx, q, caseRow.ID, evidenceFileID, traceID)
	if err != nil {
		return err
	}

	deleted, err := q.DeleteCodificationPoint(ctx, db.DeleteCodificationPointParams{
		ID:             pointID,
		CodificationID: codificationID,
	})
	if err != nil {
		return fmt.Errorf("cases: delete case_codification_points %d: %w", pointID, err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveCodificationImage stores the rendered result of manually adjusting a
// trace's codification image -- CodificationEditorModal's crop to the
// trace's box plus brightness/contrast/saturation/interpolation -- as a
// case_files row (category "codification_image"), and links it from the
// codification, creating the codification row on first use the same way
// AddPoints/ListPoints do (see resolveCodification). Works for either
// modality: a FACIAL trace's codification image is just the adjusted crop,
// no points involved.
//
// Calling this again for the same trace uploads the new bytes and re-points
// the link; the previous file is left in storage, not deleted -- nothing
// else in the schema references case_files by anything other than id, so an
// orphaned old version is harmless and (deliberately) not cleaned up here.
//
// On success, if queue is non-nil, it enqueues the codification's processing task
// (see enqueueCodificationTasks) so cmd/worker picks up the new image and computes
// its embedding or fingerprint template asynchronously. A nil queue (Redis not
// configured) or an enqueue error only logs -- the image is already saved, and the
// embedding can still be produced later by a backfill run.
func SaveCodificationImage(ctx context.Context, sqlDB *sql.DB, store *storage.Client, queue embedding.Enqueuer, caseID string, evidenceFileID, traceID int64, data []byte) (File, error) {
	if sqlDB == nil {
		return File{}, fmt.Errorf("cases: nil db")
	}
	if store == nil {
		return File{}, fmt.Errorf("cases: object storage is not configured")
	}

	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return File{}, ErrNotImage
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback()

	q := db.New(tx)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return File{}, ErrNotFound
		}
		return File{}, err
	}

	codificationID, codificationType, err := resolveCodification(ctx, q, caseRow.ID, evidenceFileID, traceID)
	if err != nil {
		return File{}, err
	}

	hash := sha256.Sum256(data)
	hashHex := hex.EncodeToString(hash[:])

	objectKey := fmt.Sprintf("codification/%s/%d/%s", caseID, traceID, hashHex)
	storageRef, err := store.Upload(ctx, objectKey, data, contentType)
	if err != nil {
		return File{}, fmt.Errorf("cases: upload codification image: %w", err)
	}

	filename := fmt.Sprintf("trace-%d-codification", traceID)
	fileRow, err := q.UpsertCaseFile(ctx, db.UpsertCaseFileParams{
		CriminalCaseID: caseRow.ID,
		Category:       "codification_image",
		MediaType:      sql.NullString{String: "image", Valid: true},
		HashID:         sql.NullString{String: hashHex, Valid: true},
		Filename:       sql.NullString{String: filename, Valid: true},
		StorageRef:     sql.NullString{String: storageRef, Valid: true},
		ContentType:    sql.NullString{String: contentType, Valid: true},
		SizeBytes:      sql.NullInt64{Int64: int64(len(data)), Valid: true},
	})
	if err != nil {
		return File{}, fmt.Errorf("cases: create case file: %w", err)
	}

	if err := q.SetCaseCodificationFile(ctx, db.SetCaseCodificationFileParams{
		ID:         codificationID,
		CaseFileID: sql.NullInt64{Int64: fileRow.ID, Valid: true},
	}); err != nil {
		return File{}, fmt.Errorf("cases: link codification image (codification %d, file %d): %w", codificationID, fileRow.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return File{}, err
	}

	enqueueCodificationTasks(queue, codificationType, []int64{codificationID})

	return File{
		ID:          fileRow.ID,
		Category:    "codification_image",
		MediaType:   "image",
		Filename:    filename,
		StorageRef:  storageRef,
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		CreatedAt:   fileRow.CreatedAt,
	}, nil
}

// enqueueCodificationTasks enqueues the task that turns each codification's
// image into what matching reads: a face embedding (embedding.ComputeForCodification)
// for FACE_EMBEDDING, a fingerprint template (fingerprint.ExtractForCodification)
// for MINUTIAE. A nil queue or an enqueue error only logs.
func enqueueCodificationTasks(queue embedding.Enqueuer, codificationType string, codificationIDs []int64) {
	if queue == nil {
		return
	}
	for _, codificationID := range codificationIDs {
		var task *asynq.Task
		var err error
		switch codificationType {
		case graph.CodificationTypeFaceEmbedding:
			task, err = embedding.NewComputeCodificationTask(codificationID)
		case graph.CodificationTypeMinutiae:
			task, err = fingerprint.NewExtractCodificationTask(codificationID)
		default:
			continue
		}
		if err != nil {
			log.Printf("cases: build task for codification %d: %v", codificationID, err)
			continue
		}
		if _, err := queue.Enqueue(task); err != nil {
			log.Printf("cases: enqueue %s for codification %d: %v", task.Type(), codificationID, err)
		}
	}
}
