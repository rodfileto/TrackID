package vision

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// oracleFace mirrors the JSON dumped from Python's insightface FaceAnalysis
// running the same AuraFace-v1 weights (testdata/oracle_t1.json), used as a
// ground-truth reference for this from-scratch Go reimplementation.
type oracleFace struct {
	Bbox      [4]float64    `json:"bbox"`
	DetScore  float64       `json:"det_score"`
	Kps       [5][2]float64 `json:"kps"`
	Embedding []float64     `json:"embedding"`
}

func loadOracle(t *testing.T) []oracleFace {
	t.Helper()
	data, err := os.ReadFile("testdata/oracle_t1.json")
	if err != nil {
		t.Fatalf("read oracle fixture: %v", err)
	}
	var faces []oracleFace
	if err := json.Unmarshal(data, &faces); err != nil {
		t.Fatalf("parse oracle fixture: %v", err)
	}
	return faces
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(Config{
		DetectorPath:      "../../models/scrfd_10g_bnkps.onnx",
		RecognizerPath:    "../../models/glintr100.onnx",
		SharedLibraryPath: "../../onnxruntime-gpu/libonnxruntime.so",
	})
	if err != nil {
		t.Fatalf("new vision service: %v", err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func iou(a [4]float32, b [4]float64) float64 {
	ax1, ay1, ax2, ay2 := float64(a[0]), float64(a[1]), float64(a[2]), float64(a[3])
	bx1, by1, bx2, by2 := b[0], b[1], b[2], b[3]

	interX1, interY1 := math.Max(ax1, bx1), math.Max(ay1, by1)
	interX2, interY2 := math.Min(ax2, bx2), math.Min(ay2, by2)
	interW, interH := math.Max(0, interX2-interX1), math.Max(0, interY2-interY1)
	inter := interW * interH

	areaA := (ax2 - ax1) * (ay2 - ay1)
	areaB := (bx2 - bx1) * (by2 - by1)
	return inter / (areaA + areaB - inter)
}

func cosineSim32to64(a []float32, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * b[i]
		na += float64(a[i]) * float64(a[i])
		nb += b[i] * b[i]
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// TestDetectAndEmbedMatchesOracle cross-validates this from-scratch Go
// SCRFD-decode + 5-point-alignment + ArcFace-embedding pipeline against
// Python's insightface FaceAnalysis running the identical AuraFace-v1 ONNX
// weights on the same image (testdata/t1.jpg, insightface's own bundled demo
// image with 6 faces). A close IoU + high cosine similarity per face is the
// strongest available evidence that the reimplementation's numerics (anchor
// decoding, NMS, Umeyama alignment, preprocessing) are correct.
func TestDetectAndEmbedMatchesOracle(t *testing.T) {
	oracle := loadOracle(t)
	svc := newTestService(t)

	f, err := os.Open("testdata/t1.jpg")
	if err != nil {
		t.Fatalf("open testdata/t1.jpg: %v", err)
	}
	defer f.Close()

	faces, err := svc.DetectAndEmbed(f)
	if err != nil {
		t.Fatalf("DetectAndEmbed: %v", err)
	}

	if len(faces) != len(oracle) {
		t.Fatalf("detected %d faces, oracle found %d", len(faces), len(oracle))
	}

	used := make([]bool, len(faces))
	for _, o := range oracle {
		bestIdx, bestIoU := -1, 0.0
		for i, gf := range faces {
			if used[i] {
				continue
			}
			if v := iou(gf.Box, o.Bbox); v > bestIoU {
				bestIoU, bestIdx = v, i
			}
		}
		if bestIdx == -1 || bestIoU < 0.7 {
			t.Fatalf("no matching detection for oracle bbox %v (best IoU %.3f)", o.Bbox, bestIoU)
		}
		used[bestIdx] = true

		sim := cosineSim32to64(faces[bestIdx].Embedding, o.Embedding)
		// Oracle embedding is raw (un-normalized); cosine similarity is
		// scale-invariant so this compares direction only, which is what
		// matters for the pgvector cosine search this feeds.
		if sim < 0.98 {
			t.Errorf("face at bbox %v: embedding cosine similarity to oracle = %.4f, want >= 0.98", o.Bbox, sim)
		}
		t.Logf("bbox %v: IoU=%.3f cosine_sim=%.5f det_score=%.3f (oracle %.3f)",
			o.Bbox, bestIoU, sim, faces[bestIdx].Score, o.DetScore)
	}
}
