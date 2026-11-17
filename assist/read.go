package assist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// What a file sent with a request is.
const (
	// KindDocument is a file that says what is asked: the ePol task, an ofício, a report. Its text
	// is what the draft is made from.
	KindDocument = "document"
	// KindEvidence is material to examine: a photo of a face, a camera frame, an identity photo. It
	// becomes an evidence of the case; its text, if any, is not read as the request's.
	KindEvidence = "evidence"
)

// documentRunes is how much text an image needs to count as a document: a page of an ofício or an
// ePol task has hundreds of characters, a photo at most a timestamp or a caption.
const documentRunes = 120

// ocrMaxPages bounds how many pages of a scanned PDF are read: a request's document is a page or
// two, and each page is a model call.
const ocrMaxPages = 10

// ReadDocument reduces one uploaded file to a Document, reading images and scanned PDFs through
// ocr when there is one: text and PDFs with a text layer as ExtractDocument does, an image by its
// text. A file with a page's worth of text is a document; an image with little or none is an
// evidence. Kind is a suggestion the operator can change before drafting. With no ocr, an image is
// an evidence with no text.
func ReadDocument(ctx context.Context, ocr Reader, name string, data []byte) (Document, error) {
	contentType := http.DetectContentType(data)
	sum := sha256.Sum256(data)
	doc := Document{Name: name, SHA256: hex.EncodeToString(sum[:]), ContentType: contentType}
	switch {
	case strings.HasPrefix(contentType, "image/"):
		if ocr != nil {
			text, err := ocr.Read(ctx, data)
			if err != nil {
				return doc, err
			}
			doc.Text, doc.OCR = truncate(text), true
		}
		doc.Kind = KindEvidence
		if utf8.RuneCountInString(doc.Text) >= documentRunes {
			doc.Kind = KindDocument
		}
		return doc, nil
	case strings.HasPrefix(contentType, "application/pdf"):
		doc.Text, doc.Kind = extractText(data, contentType), KindDocument
		if strings.TrimSpace(doc.Text) == "" && ocr != nil {
			text, err := readScannedPDF(ctx, ocr, data)
			if err != nil {
				return doc, err
			}
			doc.Text, doc.OCR = truncate(text), text != ""
		}
		return doc, nil
	default:
		doc.Text, doc.Kind = extractText(data, contentType), KindDocument
		return doc, nil
	}
}

// readScannedPDF renders the pages of a PDF with no text layer and reads each through ocr. It needs
// poppler's pdftoppm; without it a scanned PDF is read as having no text, as before.
func readScannedPDF(ctx context.Context, ocr Reader, data []byte) (string, error) {
	tool, err := exec.LookPath("pdftoppm")
	if err != nil {
		return "", nil
	}
	dir, err := os.MkdirTemp("", "assist-pdf-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, tool, "-r", "150", "-l", fmt.Sprint(ocrMaxPages), "-jpeg", in, filepath.Join(dir, "page"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("assist: render PDF pages: %v: %s", err, strings.TrimSpace(string(out)))
	}
	pages, err := filepath.Glob(filepath.Join(dir, "page*.jpg"))
	if err != nil {
		return "", err
	}
	sort.Strings(pages) // pdftoppm pads the page numbers, so names sort in page order
	var texts []string
	for _, page := range pages {
		img, err := os.ReadFile(page)
		if err != nil {
			return "", err
		}
		text, err := ocr.Read(ctx, img)
		if err != nil {
			return "", err
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n\n"), nil
}
