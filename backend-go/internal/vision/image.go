package vision

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
)

// rgbImage is pixel data as interleaved R,G,B bytes (0-255), matching what
// Go's stdlib image decoders hand back -- no BGR swap needed, since
// insightface's own preprocessing (swapRB=True) converts BGR to RGB anyway.
type rgbImage struct {
	w, h int
	pix  []uint8
}

func decodeRGB(r io.Reader) (*rgbImage, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := &rgbImage{w: w, h: h, pix: make([]uint8, w*h*3)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r32, g32, b32, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			i := (y*w + x) * 3
			out.pix[i] = uint8(r32 >> 8)
			out.pix[i+1] = uint8(g32 >> 8)
			out.pix[i+2] = uint8(b32 >> 8)
		}
	}
	return out, nil
}

func (img *rgbImage) at(x, y int) (r, g, b float64) {
	if x < 0 || y < 0 || x >= img.w || y >= img.h {
		return 0, 0, 0
	}
	i := (y*img.w + x) * 3
	return float64(img.pix[i]), float64(img.pix[i+1]), float64(img.pix[i+2])
}

// bilinear samples with zero padding outside bounds, matching cv2's default
// border behavior for our resize/warp use cases.
func (img *rgbImage) bilinear(x, y float64) (r, g, b float64) {
	x0, y0 := int(x), int(y)
	x1, y1 := x0+1, y0+1
	fx, fy := x-float64(x0), y-float64(y0)

	r00, g00, b00 := img.at(x0, y0)
	r10, g10, b10 := img.at(x1, y0)
	r01, g01, b01 := img.at(x0, y1)
	r11, g11, b11 := img.at(x1, y1)

	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	r = lerp(lerp(r00, r10, fx), lerp(r01, r11, fx), fy)
	g = lerp(lerp(g00, g10, fx), lerp(g01, g11, fx), fy)
	b = lerp(lerp(b00, b10, fx), lerp(b01, b11, fx), fy)
	return
}

// resize does a plain bilinear resize (no aspect-ratio handling -- callers
// that need letterboxing compute newW/newH themselves).
func (img *rgbImage) resize(newW, newH int) *rgbImage {
	out := &rgbImage{w: newW, h: newH, pix: make([]uint8, newW*newH*3)}
	if newW <= 1 || newH <= 1 {
		return out
	}
	scaleX := float64(img.w) / float64(newW)
	scaleY := float64(img.h) / float64(newH)
	for y := 0; y < newH; y++ {
		srcY := (float64(y)+0.5)*scaleY - 0.5
		for x := 0; x < newW; x++ {
			srcX := (float64(x)+0.5)*scaleX - 0.5
			r, g, b := img.bilinear(srcX, srcY)
			i := (y*newW + x) * 3
			out.pix[i] = clampU8(r)
			out.pix[i+1] = clampU8(g)
			out.pix[i+2] = clampU8(b)
		}
	}
	return out
}

func clampU8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// blobNCHW builds a planar (channel-major) float32 tensor from a canvasW x
// canvasH region of img starting at (0,0), normalized as (px-mean)/std,
// matching cv2.dnn.blobFromImage(img, 1/std, size, (mean,mean,mean), swapRB=True).
func blobNCHW(img *rgbImage, canvasW, canvasH int, mean, std float32) []float32 {
	out := make([]float32, 3*canvasH*canvasW)
	planeSize := canvasH * canvasW
	for y := 0; y < canvasH; y++ {
		for x := 0; x < canvasW; x++ {
			r, g, b := img.at(x, y)
			idx := y*canvasW + x
			out[0*planeSize+idx] = (float32(r) - mean) / std
			out[1*planeSize+idx] = (float32(g) - mean) / std
			out[2*planeSize+idx] = (float32(b) - mean) / std
		}
	}
	return out
}
