package embedding

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"

	"github.com/rodfileto/trackid-vision/vision"
)

// faceContextScale is how much larger than the face box the image handed to
// the detector is, box centered. SCRFD misses faces that fill its whole input
// -- which is what a codification image or a face crop is -- so the box is
// always presented at about a third of the frame, padded with neutral gray
// wherever that runs past the image edge. Measured on 60 real tight face
// crops: 0 faces found as-is, 59 found padded.
const faceContextScale = 3

// FaceEmbedder is the trackid-vision call EmbedFaceInBox needs.
// *vision.Service satisfies it.
type FaceEmbedder interface {
	DetectAndEmbed(r io.Reader) ([]vision.Face, error)
}

// EmbedFaceInBox detects faces around box in img (see faceContextScale) and
// returns the L2-normalized embedding of the highest-scoring face centered
// inside box, or nil if there is none. Pass img.Bounds() as box when the
// whole image is the face (a codification image or face crop).
func EmbedFaceInBox(vis FaceEmbedder, img image.Image, box image.Rectangle) ([]float32, error) {
	pad := image.Pt(
		box.Dx()*(faceContextScale-1)/2,
		box.Dy()*(faceContextScale-1)/2,
	)
	region := image.Rectangle{Min: box.Min.Sub(pad), Max: box.Max.Add(pad)}

	canvas := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.Gray{Y: 128}}, image.Point{}, draw.Src)
	visible := region.Intersect(img.Bounds())
	draw.Draw(canvas, visible.Sub(region.Min), img, visible.Min, draw.Src)

	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&encoded, canvas); err != nil {
		return nil, err
	}

	faces, err := vis.DetectAndEmbed(&encoded)
	if err != nil {
		return nil, err
	}

	target := box.Sub(region.Min)
	var best []float32
	var bestScore float32
	for _, face := range faces {
		center := image.Pt(
			int((face.Box[0]+face.Box[2])/2),
			int((face.Box[1]+face.Box[3])/2),
		)
		if !center.In(target) {
			continue
		}
		if best == nil || face.Score > bestScore {
			best, bestScore = face.Embedding, face.Score
		}
	}
	return best, nil
}
