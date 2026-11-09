package assist

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/rodfileto/trackid/storage"
)

// maxExtractBytes bounds how much of a single file Extract reads into memory and reduces to text.
// A ofício is short; this is a guard against a pathological PDF, not a feature.
const maxExtractBytes = 32 << 20

// maxDocumentText bounds how much extracted text one Document carries into the model, so a few
// hundred-page PDF cannot blow the model's context window. Truncation is by runes, never mid-word.
const maxDocumentText = 64 << 10

// Extract downloads each staged file and reduces it to a Document: PDFs to their text layer,
// plain text as-is, everything else (images, scans) to no text at all. OCR is not part of it --
// a scanned ofício has no text layer, and an image-only Document with empty Text is the model's
// honest answer "there is nothing here I can read". A file whose bytes are not what its content
// type claims simply yields empty text rather than an error.
func Extract(ctx context.Context, store *storage.Client, files []StagedFile) ([]Document, error) {
	if store == nil {
		return nil, fmt.Errorf("assist: object storage is not configured")
	}
	docs := make([]Document, 0, len(files))
	for _, f := range files {
		data, err := store.Download(ctx, f.StorageRef)
		if err != nil {
			return nil, fmt.Errorf("assist: download draft file %s: %w", f.Name, err)
		}
		docs = append(docs, Document{Name: f.Name, SHA256: f.SHA256, Text: extractText(data, f.ContentType)})
	}
	return docs, nil
}

// ExtractDocument reduces one uploaded file's bytes to a Document, from the bytes alone: PDFs to
// their text layer, plain text as-is, everything else (images, scans) to no text at all. It is
// Extract's single-file counterpart for callers that hold raw bytes without object storage -- the
// manual-testing assist lab, which keeps the documents in the page and stores nothing.
func ExtractDocument(name string, data []byte) Document {
	contentType := http.DetectContentType(data)
	sum := sha256.Sum256(data)
	return Document{Name: name, SHA256: hex.EncodeToString(sum[:]), Text: extractText(data, contentType)}
}

// extractText reduces one file's bytes to the text a model reads, from the bytes alone.
func extractText(data []byte, contentType string) string {
	if len(data) == 0 || len(data) > maxExtractBytes {
		return ""
	}
	switch {
	case strings.HasPrefix(contentType, "text/"):
		return truncate(string(data))
	case strings.HasPrefix(contentType, "application/pdf"):
		return truncate(pdfText(data))
	default:
		return ""
	}
}

// pdfText returns the text layer of a PDF, or "" when there is none or the file is not a readable
// PDF. ledongthuc/pdf is best-effort and can panic on a malformed file, so extraction recovers
// rather than letting a bad upload take down the worker.
func pdfText(data []byte) (text string) {
	defer func() { _ = recover() }()
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	out, err := reader.GetPlainText()
	if err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(out, maxExtractBytes))
	if err != nil {
		return ""
	}
	return string(b)
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) <= maxDocumentText {
		return s
	}
	return string(r[:maxDocumentText])
}
