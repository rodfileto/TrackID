// Package fingerprint extracts and matches fingerprint templates through the
// sourceafis-sidecar service (SourceAFIS for Java, run as its own container:
// see sourceafis-sidecar/Sidecar.java), stores templates in
// biometric_templates, and defines the Asynq tasks that do this from
// cmd/worker -- the fingerprint counterpart of the embedding package.
//
// Client is the Go side of the sidecar's HTTP boundary. A pure-Go SourceAFIS
// port can later replace it behind the same two methods.
package fingerprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnusableImage is returned by Extract when the sidecar can't decode the
// image or extract a template from it. Retrying won't change the result.
var ErrUnusableImage = errors.New("fingerprint: image unusable for extraction")

// Client calls a sourceafis-sidecar service.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient connects to the sidecar at baseURL (e.g. "http://localhost:58090")
// and checks it answers /healthz.
func NewClient(ctx context.Context, baseURL string) (*Client, error) {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: sidecar health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fingerprint: sidecar health check: %s", resp.Status)
	}
	return c, nil
}

// Extract returns the SourceAFIS template of a fingerprint image (any format
// SourceAFIS decodes: PNG, JPEG, BMP, WSQ, ...), assuming 500 dpi.
func (c *Client) Extract(ctx context.Context, image []byte) ([]byte, error) {
	body, status, err := c.post(ctx, "/extract", "application/octet-stream", image)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		return body, nil
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("%w: %s", ErrUnusableImage, strings.TrimSpace(string(body)))
	default:
		return nil, fmt.Errorf("fingerprint: extract: %d: %s", status, strings.TrimSpace(string(body)))
	}
}

// Match scores probe against each candidate template, returning one
// SourceAFIS similarity score per candidate in order. Scores are unbounded
// above; SourceAFIS's documented match threshold is 40.
func (c *Client) Match(ctx context.Context, probe []byte, candidates [][]byte) ([]float64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	// encoding/json writes []byte as base64, which is what the sidecar reads.
	payload, err := json.Marshal(struct {
		Probe      []byte   `json:"probe"`
		Candidates [][]byte `json:"candidates"`
	}{probe, candidates})
	if err != nil {
		return nil, err
	}
	body, status, err := c.post(ctx, "/match", "application/json", payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("fingerprint: match: %d: %s", status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Scores []float64 `json:"scores"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("fingerprint: match: decode response: %w", err)
	}
	if len(out.Scores) != len(candidates) {
		return nil, fmt.Errorf("fingerprint: match: %d scores for %d candidates", len(out.Scores), len(candidates))
	}
	return out.Scores, nil
}

func (c *Client) post(ctx context.Context, path, contentType string, payload []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fingerprint: %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("fingerprint: %s: read response: %w", path, err)
	}
	return body, resp.StatusCode, nil
}
