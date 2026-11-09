package assist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/rodfileto/trackid/storage"
)

// draftObjectPrefix is where staged files live in the bucket: content-addressed under a draft
// prefix, so a file staged twice writes the same object, and nothing here depends on a case that
// does not exist yet (cases.UploadEvidenceFile needs a case id, so it is not this path's job).
const draftObjectPrefix = "drafts"

// UploadDraftFile stores one uploaded file's bytes in object storage under the draft prefix and
// returns the StagedFile to hand to the worker. It is Extract's companion for callers that hold
// raw bytes, the counterpart to cases.UploadEvidenceFile: the object layout stays this package's,
// not each caller's.
//
// The content type is sniffed from the bytes, and any type is accepted (a draft may hold a PDF
// ofício, an image, or a plain text note); Extract decides what it can read. Staging the same
// bytes again writes the same object.
func UploadDraftFile(ctx context.Context, store *storage.Client, data []byte, name string) (StagedFile, error) {
	if store == nil {
		return StagedFile{}, fmt.Errorf("assist: object storage is not configured")
	}
	key, f, err := stageFile(data, name)
	if err != nil {
		return StagedFile{}, err
	}
	if f.StorageRef, err = store.Upload(ctx, key, data, f.ContentType); err != nil {
		return StagedFile{}, fmt.Errorf("assist: upload draft file: %w", err)
	}
	return f, nil
}

// stageFile is UploadDraftFile without the upload: the object key and the StagedFile to fill in,
// from the bytes alone. StorageRef is left for the caller.
func stageFile(data []byte, name string) (string, StagedFile, error) {
	if len(data) == 0 {
		return "", StagedFile{}, fmt.Errorf("assist: empty file")
	}
	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:])
	contentType := http.DetectContentType(data)
	return draftObjectKey(hashHex), StagedFile{
		SHA256:      hashHex,
		Name:        name,
		ContentType: contentType,
	}, nil
}

func draftObjectKey(hashHex string) string {
	return fmt.Sprintf("%s/%s", draftObjectPrefix, hashHex)
}
