package cases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/rodfileto/trackid/storage"
)

// UploadEvidenceFile stores one evidence file's bytes in object storage under the case's evidence
// layout and returns the FileInput to attach as EvidenceInput.File. It is Ingest's companion for
// callers that hold raw bytes (an organization's import job, trackid-sim): Ingest itself stays a
// DB-only transaction, and the object layout stays this package's, not each caller's.
//
// The content type is sniffed from the bytes, as AddEvidence does, and must be one AddEvidence
// accepts. Uploading the same bytes for the same case again writes the same object.
func UploadEvidenceFile(ctx context.Context, store *storage.Client, caseID string, data []byte, filename string) (FileInput, error) {
	if store == nil {
		return FileInput{}, fmt.Errorf("cases: object storage is not configured")
	}
	key, f, err := evidenceFile(caseID, data, filename)
	if err != nil {
		return FileInput{}, err
	}
	if f.StorageRef, err = store.Upload(ctx, key, data, f.ContentType); err != nil {
		return FileInput{}, fmt.Errorf("cases: upload evidence: %w", err)
	}
	return f, nil
}

// evidenceFile is UploadEvidenceFile without the upload: the object key and the FileInput to
// fill in, from the bytes alone. StorageRef is left for the caller.
func evidenceFile(caseID string, data []byte, filename string) (string, FileInput, error) {
	if caseID == "" {
		return "", FileInput{}, fmt.Errorf("cases: CaseID is required")
	}
	if len(data) == 0 {
		return "", FileInput{}, ValidationError("empty file")
	}
	contentType := http.DetectContentType(data)
	if _, ok := classifyContentType(contentType); !ok {
		return "", FileInput{}, ValidationError("unsupported file type; allowed: pdf, jpg, png, webp, gif, tiff, bmp")
	}
	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:])
	return evidenceObjectKey(caseID, hashHex), FileInput{
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		HashID:      hashHex,
		Filename:    filename,
	}, nil
}

// evidenceObjectKey is where an evidence file lives in the bucket: content-addressed within its
// case, so re-uploading identical bytes is a no-op overwrite.
func evidenceObjectKey(caseID, hashHex string) string {
	return fmt.Sprintf("evidence/%s/%s", caseID, hashHex)
}
