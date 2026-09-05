package vision

import (
	"fmt"
	"math"
	"sort"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	detInputSize = 640
	detMean      = 127.5
	detStd       = 128.0
	detThresh    = 0.5
	nmsThresh    = 0.4

	detectorInputName = "input.1"
)

// Output order matches the scrfd_10g_bnkps.onnx graph exactly: 3 strides x
// (score, bbox, kps), see internal/vision's onnx introspection notes.
var detectorOutputNames = []string{
	"448", "471", "494", // scores, strides 8/16/32
	"451", "474", "497", // bbox distances, strides 8/16/32
	"454", "477", "500", // kps distances, strides 8/16/32
}

var featStrides = [3]int{8, 16, 32}

const numAnchors = 2

// Detection is one detected face before alignment/embedding.
type Detection struct {
	Box   [4]float32 // x1, y1, x2, y2 in original image pixels
	Score float32
	Kps   [5][2]float32 // 5 landmark points in original image pixels
}

func (s *Service) Detect(img *rgbImage) ([]Detection, error) {
	newW, newH, detScale := letterboxSize(img.w, img.h, detInputSize, detInputSize)
	resized := img.resize(newW, newH)

	canvas := &rgbImage{w: detInputSize, h: detInputSize, pix: make([]uint8, detInputSize*detInputSize*3)}
	for y := 0; y < newH; y++ {
		copy(canvas.pix[(y*detInputSize)*3:(y*detInputSize+newW)*3], resized.pix[(y*newW)*3:(y*newW+newW)*3])
	}

	blob := blobNCHW(canvas, detInputSize, detInputSize, detMean, detStd)
	inputTensor, err := ort.NewTensor(ort.NewShape(1, 3, detInputSize, detInputSize), blob)
	if err != nil {
		return nil, fmt.Errorf("build detector input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	outputs := make([]ort.Value, len(detectorOutputNames))
	if err := s.detector.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, fmt.Errorf("run detector: %w", err)
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
	}()

	var candidates []Detection
	for idx, stride := range featStrides {
		scoreTensor, ok := outputs[idx].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected output type for scores at stride %d", stride)
		}
		bboxTensor, ok := outputs[idx+3].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected output type for bbox at stride %d", stride)
		}
		kpsTensor, ok := outputs[idx+6].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected output type for kps at stride %d", stride)
		}

		scores := scoreTensor.GetData()
		bboxes := bboxTensor.GetData()
		kpsData := kpsTensor.GetData()

		height := detInputSize / stride
		width := detInputSize / stride

		for cell := 0; cell < height*width; cell++ {
			cy := float32((cell / width) * stride)
			cx := float32((cell % width) * stride)

			for a := 0; a < numAnchors; a++ {
				row := cell*numAnchors + a
				score := scores[row]
				if score < detThresh {
					continue
				}

				bb := bboxes[row*4 : row*4+4]
				x1 := cx - bb[0]*float32(stride)
				y1 := cy - bb[1]*float32(stride)
				x2 := cx + bb[2]*float32(stride)
				y2 := cy + bb[3]*float32(stride)

				kp := kpsData[row*10 : row*10+10]
				var kps [5][2]float32
				for j := 0; j < 5; j++ {
					kps[j][0] = cx + kp[2*j]*float32(stride)
					kps[j][1] = cy + kp[2*j+1]*float32(stride)
				}

				candidates = append(candidates, Detection{
					Box:   [4]float32{x1 / detScale, y1 / detScale, x2 / detScale, y2 / detScale},
					Score: score,
					Kps:   scaleKps(kps, detScale),
				})
			}
		}
	}

	return nms(candidates, nmsThresh), nil
}

func scaleKps(kps [5][2]float32, scale float32) [5][2]float32 {
	var out [5][2]float32
	for i := range kps {
		out[i][0] = kps[i][0] / scale
		out[i][1] = kps[i][1] / scale
	}
	return out
}

// letterboxSize replicates insightface's SCRFD.detect() resize math: fit the
// image into targetW x targetH preserving aspect ratio, anchored top-left
// (the rest of the canvas stays zero-padded).
func letterboxSize(srcW, srcH, targetW, targetH int) (newW, newH int, scale float32) {
	imRatio := float64(srcH) / float64(srcW)
	modelRatio := float64(targetH) / float64(targetW)
	if imRatio > modelRatio {
		newH = targetH
		newW = int(float64(newH) / imRatio)
	} else {
		newW = targetW
		newH = int(float64(newW) * imRatio)
	}
	scale = float32(newH) / float32(srcH)
	return
}

func nms(dets []Detection, thresh float32) []Detection {
	sort.Slice(dets, func(i, j int) bool { return dets[i].Score > dets[j].Score })

	suppressed := make([]bool, len(dets))
	var keep []Detection
	for i := range dets {
		if suppressed[i] {
			continue
		}
		keep = append(keep, dets[i])
		for j := i + 1; j < len(dets); j++ {
			if suppressed[j] {
				continue
			}
			if IoU(dets[i].Box, dets[j].Box) > thresh {
				suppressed[j] = true
			}
		}
	}
	return keep
}

// IoU is the intersection-over-union of two boxes (x1,y1,x2,y2), usable by
// any caller that needs to relate two detections' boxes (e.g. matching a
// cheap detect-only box to a full detect+embed box from a nearby frame).
func IoU(a, b [4]float32) float32 {
	xx1 := max32(a[0], b[0])
	yy1 := max32(a[1], b[1])
	xx2 := min32(a[2], b[2])
	yy2 := min32(a[3], b[3])
	w := max32(0, xx2-xx1+1)
	h := max32(0, yy2-yy1+1)
	inter := w * h
	areaA := (a[2] - a[0] + 1) * (a[3] - a[1] + 1)
	areaB := (b[2] - b[0] + 1) * (b[3] - b[1] + 1)
	return inter / (areaA + areaB - inter)
}

func max32(a, b float32) float32 {
	return float32(math.Max(float64(a), float64(b)))
}

func min32(a, b float32) float32 {
	return float32(math.Min(float64(a), float64(b)))
}
