package assist

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"time"

	// Decoders for the image types a request's files come in, besides the standard library's.
	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Reader reads the text in an image: a scanned page, a photographed ofício, a screenshot of an ePol
// task. A nil Reader means no OCR model is configured, and images are read as having no text.
type Reader interface {
	Read(ctx context.Context, image []byte) (string, error)
}

// OCRClient is a Reader over an OpenAI-compatible chat endpoint that takes image_url parts, such as
// the platform's PaddleOCR-VL (llm.OCR). The image never leaves the institute's own server.
type OCRClient struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
}

// NewOCRClient builds a Reader. endpoint is the chat API base URL (the client appends
// "/chat/completions"); timeout bounds one page, DefaultTimeout when zero.
func NewOCRClient(endpoint, model, apiKey string, timeout time.Duration) *OCRClient {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &OCRClient{endpoint: strings.TrimRight(endpoint, "/"), model: model, apiKey: apiKey, http: &http.Client{Timeout: timeout}}
}

// ocrMaxSide bounds the longer side of the image sent: a page at about 200 dpi. A phone photo is
// several times that, which costs tokens and time and reads no better.
const ocrMaxSide = 2200

// ocrPrompt is PaddleOCR-VL's task prompt for plain text recognition.
const ocrPrompt = "OCR:"

// Read sends the image and returns the text the model read, trimmed; "" for an image with none.
func (c *OCRClient) Read(ctx context.Context, img []byte) (string, error) {
	jpg, err := prepareImage(img)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"model":       c.model,
		"temperature": 0,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg)}},
				{"type": "text", "text": ocrPrompt},
			},
		}},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("assist: marshal OCR request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("assist: OCR request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("assist: read OCR response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("assist: OCR model %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("assist: parse OCR response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("assist: OCR model error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("assist: OCR model returned no answer")
	}
	return strings.TrimSpace(stripThinking(out.Choices[0].Message.Content)), nil
}

// prepareImage decodes an image of any type a request sends, scales it down to ocrMaxSide and
// encodes it as JPEG, which every vision endpoint takes.
func prepareImage(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("assist: decode image: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if longest := max(w, h); longest > ocrMaxSide {
		w, h = w*ocrMaxSide/longest, h*ocrMaxSide/longest
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("assist: encode image: %w", err)
	}
	return out.Bytes(), nil
}
