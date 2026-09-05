package vision

import (
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	recMean = 127.5
	recStd  = 127.5

	recognizerInputName  = "data"
	recognizerOutputName = "1333"
)

// Embed runs the recognition model on an already-aligned 112x112 face crop,
// returning the raw 512-d embedding (not yet L2-normalized).
func (s *Service) Embed(aligned *rgbImage) ([]float32, error) {
	blob := blobNCHW(aligned, alignedSize, alignedSize, recMean, recStd)
	inputTensor, err := ort.NewTensor(ort.NewShape(1, 3, alignedSize, alignedSize), blob)
	if err != nil {
		return nil, fmt.Errorf("build recognizer input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	outputs := make([]ort.Value, 1)
	if err := s.recognizer.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, fmt.Errorf("run recognizer: %w", err)
	}
	defer outputs[0].Destroy()

	tensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected recognizer output type")
	}
	data := tensor.GetData()
	embedding := make([]float32, len(data))
	copy(embedding, data)
	return embedding, nil
}

// Normalize returns the L2-normalized (unit-length) embedding, matching the
// convention documented for face_embeddings in Postgres (CLAUDE.md: "512-d
// normalized ArcFace vectors").
func Normalize(embedding []float32) []float32 {
	var sumSq float64
	for _, v := range embedding {
		sumSq += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return embedding
	}
	out := make([]float32, len(embedding))
	for i, v := range embedding {
		out[i] = float32(float64(v) / norm)
	}
	return out
}
