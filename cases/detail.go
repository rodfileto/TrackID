package cases

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/storage"
)

// ErrNotFound is returned by Get/AddEvidence when a case does not exist.
var ErrNotFound = errors.New("cases: case not found")

// File is one case_files row (a digital file attached to a case, e.g. an
// evidence photo or document).
type File struct {
	ID          int64     `json:"id"`
	Category    string    `json:"category"`
	MediaType   string    `json:"mediaType,omitempty"`
	Filename    string    `json:"filename"`
	StorageRef  string    `json:"storageRef,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CaseDetail is a case plus its evidence files.
type CaseDetail struct {
	CaseID      string `json:"caseId"`
	CaseType    string `json:"caseType"`
	Description string `json:"description"`
	Evidences   []File `json:"evidences"`
}

// Get loads one criminal case and its evidence files by case_id.
func Get(ctx context.Context, sqlDB *sql.DB, caseID string) (CaseDetail, error) {
	if sqlDB == nil {
		return CaseDetail{}, fmt.Errorf("cases: nil db")
	}

	q := db.New(sqlDB)

	row, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CaseDetail{}, ErrNotFound
		}
		return CaseDetail{}, err
	}

	files, err := q.ListCaseFilesByCriminalCase(ctx, row.ID)
	if err != nil {
		return CaseDetail{}, err
	}

	evidences := make([]File, 0, len(files))
	for _, f := range files {
		if f.Category != "evidence" {
			continue
		}
		evidences = append(evidences, fileFromRow(f))
	}

	return CaseDetail{
		CaseID:      row.CaseID,
		CaseType:    row.CaseType,
		Description: row.Description,
		Evidences:   evidences,
	}, nil
}

// EvidenceFileInput is the content of one evidence file being attached to a
// case.
type EvidenceFileInput struct {
	Filename string
	Data     []byte
}

// AddEvidence stores one digital evidence file (pdf or an image, including
// webp) in object storage and records it as a case_files row under the given
// case. The file type is validated by sniffing the content, not the extension.
func AddEvidence(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, input EvidenceFileInput) (File, error) {
	if sqlDB == nil {
		return File{}, fmt.Errorf("cases: nil db")
	}
	if store == nil {
		return File{}, fmt.Errorf("cases: object storage is not configured")
	}

	contentType := http.DetectContentType(input.Data)
	mediaType, ok := classifyContentType(contentType)
	if !ok {
		return File{}, ValidationError("unsupported file type; allowed: pdf, jpg, png, webp, gif, tiff, bmp")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return File{}, ErrNotFound
		}
		return File{}, err
	}

	hash := sha256.Sum256(input.Data)
	hashHex := hex.EncodeToString(hash[:])

	objectKey := fmt.Sprintf("evidence/%s/%s", caseID, hashHex)
	storageRef, err := store.Upload(ctx, objectKey, input.Data, contentType)
	if err != nil {
		return File{}, fmt.Errorf("cases: upload evidence: %w", err)
	}

	row, err := q.CreateCaseFile(ctx, db.CreateCaseFileParams{
		CriminalCaseID: caseRow.ID,
		Category:       "evidence",
		MediaType:      sql.NullString{String: mediaType, Valid: true},
		HashID:         sql.NullString{String: hashHex, Valid: true},
		Filename:       sql.NullString{String: input.Filename, Valid: true},
		StorageRef:     sql.NullString{String: storageRef, Valid: true},
		ContentType:    sql.NullString{String: contentType, Valid: true},
		SizeBytes:      sql.NullInt64{Int64: int64(len(input.Data)), Valid: true},
	})
	if err != nil {
		return File{}, fmt.Errorf("cases: create case file: %w", err)
	}

	return File{
		ID:          row.ID,
		Category:    "evidence",
		MediaType:   mediaType,
		Filename:    input.Filename,
		StorageRef:  storageRef,
		ContentType: contentType,
		SizeBytes:   int64(len(input.Data)),
		CreatedAt:   row.CreatedAt,
	}, nil
}

// ErrEvidenceHasTraces is returned by DeleteEvidence when the evidence file
// already has traces marked on it -- deleting it would leave those traces,
// and their biometricfeature/feature_embeddings, pointing at an image that
// no longer exists. Delete the traces first.
var ErrEvidenceHasTraces = errors.New("cases: evidence has traces marked on it; delete them first")

// DeleteEvidence removes one evidence file (case_files row, category
// "evidence") from a case. Object storage content is left in place -- like
// AddEvidence's storage.Upload writes that a re-upload just overwrites the
// same content-hashed key, nothing in this package deletes from storage.
func DeleteEvidence(ctx context.Context, sqlDB *sql.DB, caseID string, evidenceFileID int64) error {
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

	traceCount, err := q.CountCaseTracesByCaseFile(ctx, sql.NullInt64{Int64: evidenceFileID, Valid: true})
	if err != nil {
		return fmt.Errorf("cases: count traces for file %d: %w", evidenceFileID, err)
	}
	if traceCount > 0 {
		return ErrEvidenceHasTraces
	}

	deleted, err := q.DeleteCaseFile(ctx, db.DeleteCaseFileParams{
		ID:             evidenceFileID,
		CriminalCaseID: caseRow.ID,
	})
	if err != nil {
		return fmt.Errorf("cases: delete case_files %d: %w", evidenceFileID, err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

// classifyContentType maps a detected content type to its media_type category
// ("image" or "pdf"), rejecting anything else.
func classifyContentType(contentType string) (string, bool) {
	switch contentType {
	case "application/pdf":
		return "pdf", true
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/bmp", "image/tiff":
		return "image", true
	default:
		return "", false
	}
}

// EvidenceContent is a downloaded evidence file's bytes and metadata.
type EvidenceContent struct {
	Data        []byte
	ContentType string
	Filename    string
}

// DownloadEvidence fetches one evidence file's content by case_id and file id.
// The file must belong to the given case and be of category "evidence".
func DownloadEvidence(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, evidenceID int64) (EvidenceContent, error) {
	if sqlDB == nil {
		return EvidenceContent{}, fmt.Errorf("cases: nil db")
	}
	if store == nil {
		return EvidenceContent{}, fmt.Errorf("cases: object storage is not configured")
	}

	q := db.New(sqlDB)

	caseRow, err := q.GetCriminalCaseByCaseID(ctx, caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EvidenceContent{}, ErrNotFound
		}
		return EvidenceContent{}, err
	}

	fileRow, err := q.GetCaseFile(ctx, db.GetCaseFileParams{
		ID:             evidenceID,
		CriminalCaseID: caseRow.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EvidenceContent{}, ErrNotFound
		}
		return EvidenceContent{}, err
	}
	if fileRow.Category != "evidence" {
		return EvidenceContent{}, ErrNotFound
	}

	data, err := store.Download(ctx, fileRow.StorageRef.String)
	if err != nil {
		return EvidenceContent{}, fmt.Errorf("cases: download evidence: %w", err)
	}

	return EvidenceContent{
		Data:        data,
		ContentType: fileRow.ContentType.String,
		Filename:    fileRow.Filename.String,
	}, nil
}

func fileFromRow(r db.ListCaseFilesByCriminalCaseRow) File {
	return File{
		ID:          r.ID,
		Category:    r.Category,
		MediaType:   r.MediaType.String,
		Filename:    r.Filename.String,
		StorageRef:  r.StorageRef.String,
		ContentType: r.ContentType.String,
		SizeBytes:   r.SizeBytes.Int64,
		CreatedAt:   r.CreatedAt,
	}
}
