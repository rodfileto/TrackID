package vision

import "io"

// Face is one detected, embedded face, ready for pgvector similarity search
// (Embedding is L2-normalized).
type Face struct {
	Box       [4]float32
	Score     float32
	Kps       [5][2]float32
	Embedding []float32
}

// DetectAndEmbed runs the full pipeline on an image: decode, detect faces,
// align each one to the canonical 112x112 pose, and embed it.
func (s *Service) DetectAndEmbed(r io.Reader) ([]Face, error) {
	img, err := decodeRGB(r)
	if err != nil {
		return nil, err
	}

	detections, err := s.Detect(img)
	if err != nil {
		return nil, err
	}

	faces := make([]Face, 0, len(detections))
	for _, d := range detections {
		aligned := alignFace(img, d.Kps)
		raw, err := s.Embed(aligned)
		if err != nil {
			return nil, err
		}
		faces = append(faces, Face{
			Box:       d.Box,
			Score:     d.Score,
			Kps:       d.Kps,
			Embedding: Normalize(raw),
		})
	}
	return faces, nil
}

// DetectFaces runs detection only, skipping alignment and embedding -- for
// callers that just need boxes cheaply (e.g. sampling most frames of a video
// without paying the embedding cost on every one).
func (s *Service) DetectFaces(r io.Reader) ([]Detection, error) {
	img, err := decodeRGB(r)
	if err != nil {
		return nil, err
	}
	return s.Detect(img)
}
