package requests

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rodfileto/trackid/collections"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/storage"
)

// maxDocumentBytes bounds a request's document: an ePol task's export or a scanned ofício.
const maxDocumentBytes = 50 << 20

// DocumentContent is a request document's bytes, for download.
type DocumentContent struct {
	Data        []byte
	ContentType string
	Filename    string
}

// AddDocument keeps the request's own document -- the ePol task it arrived as, or an ofício -- with
// one of the cases it opened or joined. It is the record of what was asked, not material to
// examine, so it is no evidence. A PDF, an image or plain text.
func AddDocument(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, requestID int64, filename string, data []byte) (File, error) {
	if store == nil {
		return File{}, fmt.Errorf("requests: object storage is not configured")
	}
	if len(data) == 0 {
		return File{}, ValidationError("empty file")
	}
	if len(data) > maxDocumentBytes {
		return File{}, ValidationError("the document is too large")
	}
	contentType := http.DetectContentType(data)
	mediaType, ok := documentMediaType(contentType)
	if !ok {
		return File{}, ValidationError("unsupported document type; allowed: pdf, an image or plain text")
	}
	// The request must be one of the case's (the same check material goes through).
	if err := collections.CheckOrigin(ctx, sqlDB, caseID, collections.Origin{RequestID: requestID}); err != nil {
		var v collections.ValidationError
		if errors.As(err, &v) {
			return File{}, ValidationError(v.Error())
		}
		return File{}, err
	}
	q := db.New(sqlDB)
	caseRow, err := q.GetCaseForRequest(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrCaseNotFound
	}
	if err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:])
	ref, err := store.Upload(ctx, fmt.Sprintf("request-documents/%s/%s", caseID, hashHex), data, contentType)
	if err != nil {
		return File{}, fmt.Errorf("requests: upload document: %w", err)
	}
	row, err := q.InsertRequestDocument(ctx, db.InsertRequestDocumentParams{
		BiometricCaseID: caseRow.ID, MediaType: mediaType, HashID: hashHex, Filename: filename, StorageRef: ref,
		ContentType: contentType, SizeBytes: int64(len(data)), CaseRequestID: requestID,
	})
	if err != nil {
		return File{}, fmt.Errorf("requests: record document: %w", err)
	}
	return File{ID: row.ID, Filename: filename, SHA256: hashHex, ContentType: contentType, SizeBytes: int64(len(data)), AddedAt: row.CreatedAt}, nil
}

// Document downloads a request document of a case.
func Document(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, fileID int64) (DocumentContent, error) {
	row, err := db.New(sqlDB).GetRequestDocument(ctx, db.GetRequestDocumentParams{ID: fileID, CaseID: caseID})
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentContent{}, ErrCaseNotFound
	}
	if err != nil {
		return DocumentContent{}, err
	}
	data, err := store.Download(ctx, row.StorageRef.String)
	if err != nil {
		return DocumentContent{}, fmt.Errorf("requests: download document: %w", err)
	}
	return DocumentContent{Data: data, ContentType: row.ContentType.String, Filename: row.Filename.String}, nil
}

func documentMediaType(contentType string) (string, bool) {
	switch {
	case contentType == "application/pdf":
		return "pdf", true
	case strings.HasPrefix(contentType, "image/"):
		return "image", true
	case strings.HasPrefix(contentType, "text/"):
		return "text", true
	}
	return "", false
}
