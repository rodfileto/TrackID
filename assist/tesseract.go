package assist

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

// Tesseract is a Reader over the tesseract command (Tesseract OCR, Apache 2.0): no language model,
// a second or two a page on the CPU. It reads screenshots and scanned pages well, photos of paper
// less so; Layered sends those on to the OCR model.
type Tesseract struct {
	path string
	lang string
}

// NewTesseract finds the tesseract command. lang is its language list, "por" when empty; ok is
// false when the command is not installed.
func NewTesseract(lang string) (*Tesseract, bool) {
	path, err := exec.LookPath("tesseract")
	if err != nil {
		return nil, false
	}
	if lang == "" {
		lang = "por"
	}
	return &Tesseract{path: path, lang: lang}, true
}

// tesseractMinSide is the size an image is scaled up to before reading: Tesseract is trained on
// text at about 300 dpi, and a screenshot's 12-pixel letters read far better doubled.
const tesseractMinSide = 2400

// Read returns the text in the image.
func (t *Tesseract) Read(ctx context.Context, img []byte) (string, error) {
	text, _, err := t.ReadConfidence(ctx, img)
	return text, err
}

// ReadConfidence returns the text and Tesseract's mean word confidence, 0 to 100, weighted by
// word length; 0 when it found no words.
func (t *Tesseract) ReadConfidence(ctx context.Context, img []byte) (string, float64, error) {
	start := time.Now()
	in, err := prepareForTesseract(img)
	if err != nil {
		return "", 0, err
	}
	dir, err := os.MkdirTemp("", "assist-ocr-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(dir)
	src, out := filepath.Join(dir, "in.png"), filepath.Join(dir, "out")
	if err := os.WriteFile(src, in, 0o600); err != nil {
		return "", 0, err
	}
	// One run writes both the plain text (out.txt) and the words with their confidence (out.tsv).
	cmd := exec.CommandContext(ctx, t.path, src, out, "-l", t.lang, "--psm", "3", "txt", "tsv")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("assist: tesseract: %v: %s", err, strings.TrimSpace(string(msg)))
	}
	txt, err := os.ReadFile(out + ".txt")
	if err != nil {
		return "", 0, err
	}
	tsv, err := os.ReadFile(out + ".tsv")
	if err != nil {
		return "", 0, err
	}
	text := tidyOCR(string(txt))
	conf := meanConfidence(tsv)
	log.Printf("assist: tesseract read %d characters at %.0f%% confidence in %s", len([]rune(text)), conf, time.Since(start).Round(100*time.Millisecond))
	return text, conf, nil
}

// prepareForTesseract decodes the image, scales a small one up to tesseractMinSide and writes it
// as grey PNG: lossless, so the letters' edges stay sharp.
func prepareForTesseract(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("assist: decode image: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if longest := max(w, h); longest > 0 && longest < tesseractMinSide {
		scale := min(float64(tesseractMinSide)/float64(longest), 3)
		w, h = int(float64(w)*scale), int(float64(h)*scale)
	}
	dst := image.NewGray(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("assist: encode image: %w", err)
	}
	return out.Bytes(), nil
}

// tidyOCR trims each line and keeps at most one blank line in a row.
func tidyOCR(s string) string {
	var lines []string
	blank := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(lines) > 0 {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		blank = false
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// meanConfidence reads Tesseract's TSV (level, ..., conf, text) and averages the confidence of
// its words, weighted by their length so a stray "|" counts little.
func meanConfidence(tsv []byte) float64 {
	r := csv.NewReader(bytes.NewReader(tsv))
	r.Comma, r.LazyQuotes, r.FieldsPerRecord = '\t', true, -1
	var sum, weight float64
	for first := true; ; first = false {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || first || len(rec) < 12 || rec[0] != "5" {
			continue
		}
		conf, err := strconv.ParseFloat(rec[10], 64)
		word := strings.TrimSpace(rec[11])
		if err != nil || conf < 0 || word == "" {
			continue
		}
		n := float64(len([]rune(word)))
		sum, weight = sum+conf*n, weight+n
	}
	if weight == 0 {
		return 0
	}
	return sum / weight
}

// Layered reads with Tesseract first and asks Then, the OCR model, only for an image Tesseract read
// poorly: a page with text it was unsure of. An image with almost no words (a face, a scene) is
// taken as read: it has no text worth a minute of the model.
type Layered struct {
	First *Tesseract
	Then  Reader // may be nil: Tesseract alone
}

// layeredMinConfidence is the confidence below which a page goes to the model. Tesseract scores a
// clean screenshot in the 90s and a skewed phone photo of a page in the 50s or 60s.
const layeredMinConfidence = 75

// layeredMinRunes is how much text makes an image a page worth a second reading; less is a photo
// with a caption or a timestamp, or noise.
const layeredMinRunes = 40

func (l Layered) Read(ctx context.Context, img []byte) (string, error) {
	text, conf, err := l.First.ReadConfidence(ctx, img)
	if err != nil && l.Then == nil {
		return "", err
	}
	if err == nil && (l.Then == nil || conf >= layeredMinConfidence || len([]rune(text)) < layeredMinRunes) {
		return text, nil
	}
	if err != nil {
		log.Printf("assist: tesseract failed, asking the OCR model: %v", err)
	} else {
		log.Printf("assist: tesseract confidence %.0f%% is low, asking the OCR model", conf)
	}
	return l.Then.Read(ctx, img)
}
