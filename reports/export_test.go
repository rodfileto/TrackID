package reports

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Exported for tests.
var (
	RenderForTest = func(d Data, imgs map[string]Image) ([]byte, error) {
		out, err := render(d, imgs)
		if err != nil {
			return nil, err
		}
		return out.bytes()
	}
	ParseForTest = func(b []byte) (string, []string, error) { p, err := parseDocx(b); return p.locked, p.regions, err }
)

// FillRegionsForTest stands in for the examiner typing in WordCraft: it puts text right after the
// permStart of the regions named by id (1-based), the way an editor inserts a run there.
func FillRegionsForTest(docx []byte, texts map[int]string) ([]byte, error) {
	files, err := readZip(docx)
	if err != nil {
		return nil, err
	}
	s := string(files["word/document.xml"])
	for id, text := range texts {
		tag := fmt.Sprintf(`<w:permStart w:id="%d" w:edGrp="everyone"/>`, id)
		s = strings.Replace(s, tag, tag+run(text, false, false), 1)
	}
	return rewriteForTest(docx, "word/document.xml", s)
}

// TamperForTest changes locked text, as an editor with protection removed could.
func TamperForTest(docx []byte, old, repl string) ([]byte, error) {
	files, err := readZip(docx)
	if err != nil {
		return nil, err
	}
	return rewriteForTest(docx, "word/document.xml", strings.Replace(string(files["word/document.xml"]), old, repl, 1))
}

func rewriteForTest(docx []byte, name, content string) ([]byte, error) {
	src, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range src.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		if f.Name == name {
			b = []byte(content)
		}
		fw, err := w.Create(f.Name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(b); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// TextForTest is a document's whole text (locked and regions), for assertions.
func TextForTest(docx []byte) (string, error) {
	files, err := readZip(docx)
	if err != nil {
		return "", err
	}
	return string(files["word/document.xml"]) + string(files["word/header1.xml"]), nil
}
