package cases

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/storage"
)

// FaceVision is the face detection and recognition this package uses.
// *vision.Service satisfies it.
type FaceVision interface {
	DetectFaces(r io.Reader) ([]vision.Detection, error)
	DetectAndEmbed(r io.Reader) ([]vision.Face, error)
}

// ErrVisionUnavailable is returned when face detection/recognition is needed
// but no FaceVision is configured (VISION_* env vars unset, or the models
// failed to load).
var ErrVisionUnavailable = errors.New("cases: face vision is not configured")

// ErrUnsupportedImage is returned when an image is in a format trackid-vision
// can't decode.
var ErrUnsupportedImage = errors.New("unsupported image format for face analysis (JPEG, PNG, GIF, BMP, TIFF, and WebP only)")

// supportedImageFormats mirrors the decoders trackid-vision's vision package
// registers (see its image.go) -- image.DecodeConfig's format string only
// recognizes a format once something in the binary has blank-imported its
// decoder, which importing vision already does for all of these.
var supportedImageFormats = map[string]bool{
	"jpeg": true,
	"png":  true,
	"gif":  true,
	"bmp":  true,
	"tiff": true,
	"webp": true,
}

// FaceProposal is one face the detector found on an evidence image: a box in
// the image's pixel coordinates plus the detector's confidence. Same shape as
// a trace to create, so accepting one is just sending it to CreateTraces.
type FaceProposal struct {
	BoxX1 float64 `json:"boxX1"`
	BoxY1 float64 `json:"boxY1"`
	BoxX2 float64 `json:"boxX2"`
	BoxY2 float64 `json:"boxY2"`
	Score float64 `json:"score"`
}

// DetectFaces runs face detection on one evidence image and returns what it
// found as proposals. It writes nothing: whether a proposal becomes a trace
// is the analyst's decision, made by sending it to CreateTraces.
func DetectFaces(ctx context.Context, sqlDB *sql.DB, store *storage.Client, vis FaceVision, caseID string, evidenceFileID int64) ([]FaceProposal, error) {
	if vis == nil {
		return nil, ErrVisionUnavailable
	}

	content, err := DownloadEvidence(ctx, sqlDB, store, caseID, evidenceFileID)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(content.ContentType, "image/") {
		return nil, ErrNotImage
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(content.Data))
	if err != nil || !supportedImageFormats[format] {
		return nil, ErrUnsupportedImage
	}

	detections, err := vis.DetectFaces(bytes.NewReader(content.Data))
	if err != nil {
		return nil, fmt.Errorf("cases: detect faces on evidence %d: %w", evidenceFileID, err)
	}

	// SCRFD can place a box partly outside the frame for faces cut off at the
	// edge; clamp so every proposal crops cleanly.
	width, height := float64(cfg.Width), float64(cfg.Height)
	proposals := make([]FaceProposal, 0, len(detections))
	for _, d := range detections {
		proposal := FaceProposal{
			BoxX1: clamp(float64(d.Box[0]), 0, width),
			BoxY1: clamp(float64(d.Box[1]), 0, height),
			BoxX2: clamp(float64(d.Box[2]), 0, width),
			BoxY2: clamp(float64(d.Box[3]), 0, height),
			Score: float64(d.Score),
		}
		if proposal.BoxX2 <= proposal.BoxX1 || proposal.BoxY2 <= proposal.BoxY1 {
			continue
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func clamp(v, lo, hi float64) float64 {
	return max(lo, min(v, hi))
}
