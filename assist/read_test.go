package assist

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/rodfileto/trackid/requests"
)

type fakeReader struct{ text string }

func (f fakeReader) Read(context.Context, []byte) (string, error) { return f.text, nil }

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReadDocumentKind(t *testing.T) {
	ctx, img := context.Background(), pngBytes(t)
	page := strings.Repeat("Tarefa ePol nº 102802/2026-2. Solicito exame de comparação facial. ", 4)
	for _, c := range []struct {
		name string
		ocr  Reader
		want string
	}{
		{"a page of text", fakeReader{page}, KindDocument},
		{"a photo with a timestamp", fakeReader{"2026-11-17 10:42"}, KindEvidence},
		{"no OCR model", nil, KindEvidence},
	} {
		doc, err := ReadDocument(ctx, c.ocr, "x.png", img)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Kind != c.want {
			t.Errorf("%s: kind %q, want %q", c.name, doc.Kind, c.want)
		}
		if c.ocr != nil && !doc.OCR {
			t.Errorf("%s: not marked as read by OCR", c.name)
		}
	}
	doc, err := ReadDocument(ctx, nil, "task.txt", []byte("Tarefa 1"))
	if err != nil || doc.Kind != KindDocument || doc.Text != "Tarefa 1" {
		t.Errorf("text file: %+v, %v", doc, err)
	}
}

func TestCoerceReferences(t *testing.T) {
	RegisterReferenceLabels("Tarefa E-POL")
	defer RegisterReferenceLabels()
	var d Draft
	d.Request.Input.References = []requests.Reference{{Label: "Tarefa E-POL", Number: " 102802/2026-2 "}, {Label: "Processo", Number: "1"}, {Label: "Tarefa E-POL", Number: ""}}
	out := Coerce(d, nil)
	if len(out.Request.Input.References) != 1 || out.Request.Input.References[0].Number != "102802/2026-2" {
		t.Errorf("references = %+v", out.Request.Input.References)
	}
	if len(out.Unsure) != 1 || out.Unsure[0] != "references[1]" {
		t.Errorf("unsure = %v", out.Unsure)
	}
}

func TestUserPromptSeparatesEvidence(t *testing.T) {
	p := userPrompt([]Document{{Name: "tarefa.pdf", Text: "Solicito exame", Kind: KindDocument}, {Name: "suspeito.jpg", Text: "10:42", Kind: KindEvidence}}, "")
	if !strings.Contains(p, "1 evidence image(s)") || !strings.Contains(p, "- suspeito.jpg") {
		t.Errorf("evidence not listed:\n%s", p)
	}
	if strings.Contains(p, "### suspeito.jpg") || strings.Contains(p, "10:42") {
		t.Errorf("an evidence's text went in as a document:\n%s", p)
	}
}
