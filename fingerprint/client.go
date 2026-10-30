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
	"hash/fnv"
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

// DefaultDPI is the resolution Extract is told when the image's own is not
// known: trackid-sim's prints and ten-print cards are 500 ppi.
const DefaultDPI = 500

// Extract returns the SourceAFIS template of a fingerprint image (any format
// SourceAFIS decodes: PNG, JPEG, BMP, WSQ, ...) at dpi pixels per inch.
// SourceAFIS measures minutia distances in millimetres through it, so a wrong
// dpi spoils the template.
func (c *Client) Extract(ctx context.Context, image []byte, dpi float64) ([]byte, error) {
	if dpi <= 0 {
		dpi = DefaultDPI
	}
	body, status, err := c.post(ctx, fmt.Sprintf("/extract?dpi=%g", dpi), "application/octet-stream", image)
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
//
// It goes through the sidecar's gallery (/match-cached): templates are sent by
// key, the hash of their bytes, and the sidecar keeps them deserialised, so each
// is sent and rebuilt once rather than for every probe; the sidecar also spreads
// the candidates over its cores. Templates the gallery lacks (the first time, or
// after a sidecar restart) are sent and the match retried. A sidecar without the
// gallery gets the plain /match.
func (c *Client) Match(ctx context.Context, probe []byte, candidates [][]byte) ([]float64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	byKey := map[string][]byte{}
	key := func(t []byte) string {
		h := fnv.New64a()
		h.Write(t)
		k := fmt.Sprintf("%016x-%d", h.Sum64(), len(t))
		byKey[k] = t
		return k
	}
	probeKey := key(probe)
	keys := make([]string, len(candidates))
	for i, t := range candidates {
		keys[i] = key(t)
	}
	payload, err := json.Marshal(struct {
		Probe      string   `json:"probe"`
		Candidates []string `json:"candidates"`
	}{probeKey, keys})
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		body, status, err := c.post(ctx, "/match-cached", "application/json", payload)
		if err != nil {
			return nil, err
		}
		switch {
		case status == http.StatusOK:
			return decodeScores(body, len(candidates))
		case status == http.StatusNotFound:
			return c.matchPlain(ctx, probe, candidates)
		case status == http.StatusConflict && attempt == 0:
			var out struct {
				Missing []string `json:"missing"`
			}
			if err := json.Unmarshal(body, &out); err != nil {
				return nil, fmt.Errorf("fingerprint: match: decode missing templates: %w", err)
			}
			if err := c.addToGallery(ctx, out.Missing, byKey); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("fingerprint: match: %d: %s", status, strings.TrimSpace(string(body)))
		}
	}
}

// galleryBatch caps how many templates go to the sidecar's gallery in one request.
const galleryBatch = 2000

func (c *Client) addToGallery(ctx context.Context, keys []string, byKey map[string][]byte) error {
	type item struct {
		Key      string `json:"key"`
		Template []byte `json:"template"` // base64, as the sidecar reads it
	}
	for start := 0; start < len(keys); start += galleryBatch {
		var items []item
		for _, k := range keys[start:min(start+galleryBatch, len(keys))] {
			t, ok := byKey[k]
			if !ok {
				return fmt.Errorf("fingerprint: gallery asked for template %s, not one sent", k)
			}
			items = append(items, item{k, t})
		}
		payload, err := json.Marshal(struct {
			Templates []item `json:"templates"`
		}{items})
		if err != nil {
			return err
		}
		body, status, err := c.post(ctx, "/gallery", "application/json", payload)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("fingerprint: gallery: %d: %s", status, strings.TrimSpace(string(body)))
		}
	}
	return nil
}

// matchPlain sends the templates themselves, for a sidecar without the gallery.
func (c *Client) matchPlain(ctx context.Context, probe []byte, candidates [][]byte) ([]float64, error) {
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
	return decodeScores(body, len(candidates))
}

func decodeScores(body []byte, n int) ([]float64, error) {
	var out struct {
		Scores []float64 `json:"scores"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("fingerprint: match: decode response: %w", err)
	}
	if len(out.Scores) != n {
		return nil, fmt.Errorf("fingerprint: match: %d scores for %d candidates", len(out.Scores), n)
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
