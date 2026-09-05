package videoproc

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
)

const cropPadding = 0.4 // extra context around the detected box, as a fraction of its size

// cropDataURI crops the padded face box out of the frame image and returns
// it as a base64 JPEG data URI, ready for an <img src="..."> in the UI.
func cropDataURI(img image.Image, box [4]float32) (string, error) {
	bounds := img.Bounds()
	w, h := box[2]-box[0], box[3]-box[1]
	padX, padY := w*cropPadding, h*cropPadding

	x0 := clampInt(int(box[0]-padX), bounds.Min.X, bounds.Max.X)
	y0 := clampInt(int(box[1]-padY), bounds.Min.Y, bounds.Max.Y)
	x1 := clampInt(int(box[2]+padX), bounds.Min.X, bounds.Max.X)
	y1 := clampInt(int(box[3]+padY), bounds.Min.Y, bounds.Max.Y)
	if x1 <= x0 || y1 <= y0 {
		return "", nil
	}

	cropped := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			cropped.Set(x-x0, y-y0, color.RGBAModel.Convert(img.At(x, y)))
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 90}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
