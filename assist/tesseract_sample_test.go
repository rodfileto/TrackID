package assist

import (
	"context"
	"os"
	"testing"
)

// TestTesseractSample reads the image ASSIST_OCR_SAMPLE names, to try Tesseract on a real
// document: ASSIST_OCR_SAMPLE=page.png go test ./assist -run TestTesseractSample -v
func TestTesseractSample(t *testing.T) {
	path := os.Getenv("ASSIST_OCR_SAMPLE")
	if path == "" {
		t.Skip("ASSIST_OCR_SAMPLE not set")
	}
	tess, ok := NewTesseract("por")
	if !ok {
		t.Skip("tesseract not installed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text, conf, err := tess.ReadConfidence(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("confidence %.0f%%\n%s", conf, text)
}
