package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/rodfileto/trackid/storage"
)

// UploadFile stores one enrollment file's bytes in object storage under the register's layout and
// returns the FileInput to add to Enrollment.Files. It is Ingest's companion for callers that
// hold raw bytes: Ingest works from a StorageRef and never touches storage itself, and the object
// layout stays this package's, not each caller's.
//
// The content type is sniffed from the bytes (a NIST file, which has no signature
// http.DetectContentType knows, comes out as application/octet-stream). SourcePath is left for the
// caller: it names the file in the caller's own system, which this package cannot know.
func UploadFile(ctx context.Context, store *storage.Client, registerNumber, fileType string, sequence int16, data []byte) (FileInput, error) {
	if store == nil {
		return FileInput{}, fmt.Errorf("identity: object storage is not configured")
	}
	key, f, err := enrollmentFile(registerNumber, fileType, sequence, data)
	if err != nil {
		return FileInput{}, err
	}
	if f.StorageRef, err = store.Upload(ctx, key, data, f.ContentType); err != nil {
		return FileInput{}, fmt.Errorf("identity: upload %s: %w", fileType, err)
	}
	return f, nil
}

// enrollmentFile is UploadFile without the upload: the object key and the FileInput to fill in.
func enrollmentFile(registerNumber, fileType string, sequence int16, data []byte) (string, FileInput, error) {
	if registerNumber == "" || fileType == "" {
		return "", FileInput{}, fmt.Errorf("identity: register number and file type are required")
	}
	if len(data) == 0 {
		return "", FileInput{}, fmt.Errorf("identity: empty %s file", fileType)
	}
	sum := sha256.Sum256(data)
	return enrollmentObjectKey(registerNumber, fileType, hex.EncodeToString(sum[:])), FileInput{
		FileType:    fileType,
		Sequence:    sequence,
		ContentType: http.DetectContentType(data),
		SizeBytes:   int64(len(data)),
	}, nil
}

// enrollmentObjectKey is where an enrollment file lives in the bucket: content-addressed within
// its register and file type, mirroring cases' evidence/<case>/<sha256>.
func enrollmentObjectKey(registerNumber, fileType, hashHex string) string {
	return fmt.Sprintf("identity/%s/%s/%s", registerNumber, fileType, hashHex)
}
