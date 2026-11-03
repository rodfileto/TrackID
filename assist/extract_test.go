package assist

import (
	"testing"
)

func TestExtractTextPlain(t *testing.T) {
	got := extractText([]byte("texto simples"), "text/plain")
	if got != "texto simples" {
		t.Errorf("plain text not passed through: %q", got)
	}
}

func TestExtractTextImageAndEmpty(t *testing.T) {
	if got := extractText([]byte{0x89, 0x50}, "image/png"); got != "" {
		t.Errorf("image should have no text: %q", got)
	}
	if got := extractText(nil, "application/pdf"); got != "" {
		t.Errorf("empty should have no text: %q", got)
	}
	if got := extractText([]byte("x"), ""); got != "" {
		t.Errorf("unknown type should have no text: %q", got)
	}
}

func TestExtractTextTruncates(t *testing.T) {
	in := make([]byte, maxDocumentText+100)
	for i := range in {
		in[i] = 'a'
	}
	got := extractText(in, "text/plain")
	if len([]rune(got)) != maxDocumentText {
		t.Errorf("text not truncated to %d runes: %d", maxDocumentText, len([]rune(got)))
	}
}

func TestPDFTextMalformed(t *testing.T) {
	// A non-PDF should yield empty text, not panic.
	if got := extractText([]byte("isto não é um pdf"), "application/pdf"); got != "" {
		t.Errorf("malformed pdf should have no text: %q", got)
	}
}
