package imageproc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
)

// Face images (docs/facial-comparison-tool-plan.md) are processed in colour and have no
// resolution to reach: the face recipe offers only the techniques FISWG's Image Processing
// Techniques for Facial Image Comparison (v1.0, §5.1) lists, every one applied to the whole image
// (§4.5). Nothing adaptive or local (lighting correction, CLAHE, Gabor) is offered for faces.

// FaceRecipeVersion is the version of the face recipe format and of the rendering it names.
const FaceRecipeVersion = 1

// faceMaxPixels bounds a face render (three float planes are held while it works).
const faceMaxPixels = 40_000_000

// faceFill is the grey given to pixels outside the original after a rotation: neutral, so it
// reads as no image rather than as part of the scene.
const faceFill = 128

// FaceRecipe is what a face render does, in a fixed order (see RenderFace). Steps left nil are off.
type FaceRecipe struct {
	Version int     `json:"version"`
	Crop    *Rect   `json:"crop,omitempty"`
	Rotate  float64 `json:"rotate"` // degrees, clockwise
	Mirror  bool    `json:"mirror"`
	// Scale is rendered pixels per original pixel (1 keeps the size). Uniform: a face is never
	// stretched in one direction only, which would change its proportions.
	Scale              float64             `json:"scale"`
	Interpolation      string              `json:"interpolation"`
	ColorBalance       *ColorBalance       `json:"colorBalance,omitempty"`
	Grey               string              `json:"grey,omitempty"` // "" keeps colour; else a Channel*
	BrightnessContrast *BrightnessContrast `json:"brightnessContrast,omitempty"`
	Levels             *Levels             `json:"levels,omitempty"`
	Curves             *Curves             `json:"curves,omitempty"`
}

// ColorBalance multiplies each channel by a gain, as a neutral picked in the image gives them.
type ColorBalance struct {
	Red   float64 `json:"red"`
	Green float64 `json:"green"`
	Blue  float64 `json:"blue"`
}

// BrightnessContrast is v' = (v - 128) * (100 + c) / (100 - c) + 128 + 1.275 * b, with b and c
// in -100..100 (c up to 99).
type BrightnessContrast struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
}

// Curves maps tones through a monotone cubic (Fritsch-Carlson) passing through its points, input
// to output in 0..255. The ends (0, 0) and (255, 255) are implied unless a point sets them.
type Curves struct {
	Points [][2]float64 `json:"points"`
}

// FaceResult is a face render and what its record needs.
type FaceResult struct {
	PNG    []byte
	SHA256 string
	Width  int
	Height int
	Grey   bool
	// Transform maps original pixel coordinates to the render's.
	Transform Affine
	// Histogram is the luminance histogram levels and curves work on, for their controls.
	Histogram      [256]int
	AntialiasSigma float64
}

// Normalize fills defaults and checks the recipe against an original of width x height.
func (r *FaceRecipe) Normalize(width, height int) error {
	if r.Version == 0 {
		r.Version = FaceRecipeVersion
	}
	if r.Version != FaceRecipeVersion {
		return ValidationError(fmt.Sprintf("face recipe version %d is not supported (this is %d)", r.Version, FaceRecipeVersion))
	}
	if r.Scale == 0 {
		r.Scale = 1
	}
	if math.IsNaN(r.Scale) || r.Scale < 0.05 || r.Scale > 8 {
		return ValidationError("the scale must be between 0.05 and 8")
	}
	if r.Interpolation == "" {
		r.Interpolation = InterpolationBicubic
	}
	switch r.Interpolation {
	case InterpolationNearest, InterpolationBilinear, InterpolationBicubic:
	default:
		return ValidationError("unknown interpolation " + r.Interpolation)
	}
	switch r.Grey {
	case "", ChannelLuminance, ChannelRed, ChannelGreen, ChannelBlue:
	default:
		return ValidationError("unknown channel " + r.Grey)
	}
	if r.Crop != nil {
		c := r.Crop
		c.X1, c.X2 = math.Max(0, math.Min(c.X1, c.X2)), math.Min(float64(width), math.Max(c.X1, c.X2))
		c.Y1, c.Y2 = math.Max(0, math.Min(c.Y1, c.Y2)), math.Min(float64(height), math.Max(c.Y1, c.Y2))
		if c.X2-c.X1 < 8 || c.Y2-c.Y1 < 8 {
			return ValidationError("the crop is smaller than 8 pixels")
		}
	}
	if math.IsNaN(r.Rotate) || math.Abs(r.Rotate) > 360 {
		return ValidationError("rotation must be between -360 and 360 degrees")
	}
	if cb := r.ColorBalance; cb != nil {
		for _, g := range []float64{cb.Red, cb.Green, cb.Blue} {
			if math.IsNaN(g) || g < 0.25 || g > 4 {
				return ValidationError("colour balance gains must be between 0.25 and 4")
			}
		}
	}
	if bc := r.BrightnessContrast; bc != nil {
		if bc.Brightness < -100 || bc.Brightness > 100 || bc.Contrast < -100 || bc.Contrast > 99 {
			return ValidationError("brightness must be within -100..100 and contrast within -100..99")
		}
	}
	if l := r.Levels; l != nil {
		if l.InBlack < 0 || l.InWhite > 255 || l.InWhite-l.InBlack < 1 {
			return ValidationError("levels: input black must be below input white, within 0-255")
		}
		if l.Gamma < 0.1 || l.Gamma > 9.99 {
			return ValidationError("levels: gamma must be between 0.10 and 9.99")
		}
		if l.OutBlack < 0 || l.OutWhite > 255 || l.OutBlack > l.OutWhite {
			return ValidationError("levels: output black must not exceed output white, within 0-255")
		}
	}
	if cv := r.Curves; cv != nil {
		if len(cv.Points) == 0 || len(cv.Points) > 16 {
			return ValidationError("curves take 1 to 16 points")
		}
		sort.Slice(cv.Points, func(i, j int) bool { return cv.Points[i][0] < cv.Points[j][0] })
		for i, p := range cv.Points {
			if p[0] < 0 || p[0] > 255 || p[1] < 0 || p[1] > 255 {
				return ValidationError("curve points must be within 0-255")
			}
			if i > 0 && p[0]-cv.Points[i-1][0] < 1 {
				return ValidationError("curve points must be at least one tone apart")
			}
		}
	}
	g := affineGeometry(r.Crop, r.Rotate, r.Mirror, r.Scale, width, height)
	if g.width*g.height > faceMaxPixels {
		return ValidationError(fmt.Sprintf("the render would be %d × %d pixels; reduce the scale or crop", g.width, g.height))
	}
	return nil
}

// RenderFace applies r to o. r must have been normalised against o (FaceRecipe.Normalize). The
// order is fixed: crop, rotation, mirror and scaling in one transform (one interpolation); colour
// balance; conversion to grey; brightness/contrast; levels; curves.
func RenderFace(o Original, r FaceRecipe) (FaceResult, error) {
	g := affineGeometry(r.Crop, r.Rotate, r.Mirror, r.Scale, o.Width, o.Height)
	sigma := antialiasSigma(g.scale, r.Interpolation)
	margin := int(math.Ceil(3*sigma)) + 3
	region := image.Rect(
		int(math.Floor(g.crop.X1))-margin, int(math.Floor(g.crop.Y1))-margin,
		int(math.Ceil(g.crop.X2))+margin, int(math.Ceil(g.crop.Y2))+margin,
	).Intersect(image.Rect(0, 0, o.Width, o.Height))
	src := rgbPlanes(subImage(o.Image, region))
	crop := Rect{g.crop.X1 - float64(region.Min.X), g.crop.Y1 - float64(region.Min.Y), g.crop.X2 - float64(region.Min.X), g.crop.Y2 - float64(region.Min.Y)}
	local := affineGeometry(&crop, r.Rotate, r.Mirror, r.Scale, src[0].w, src[0].h)

	res := FaceResult{AntialiasSigma: sigma}
	f := local.forward
	ox, oy := float64(region.Min.X), float64(region.Min.Y)
	res.Transform = Affine{f.A, f.B, f.C - f.A*ox - f.B*oy, f.D, f.E, f.F - f.D*ox - f.E*oy}

	planes := make([]*plane, 3)
	for i, pl := range src {
		if sigma > 0 {
			pl = gaussianBlur(pl, sigma)
		}
		planes[i], _ = warp(pl, local, r.Interpolation, faceFill)
	}
	if cb := r.ColorBalance; cb != nil {
		for i, gain := range []float64{cb.Red, cb.Green, cb.Blue} {
			planes[i] = mapPlane(planes[i], func(v float64) float64 { return v * gain })
		}
	}
	if r.Grey != "" {
		planes = []*plane{toGrey(planes, r.Grey)}
		res.Grey = true
	}
	if bc := r.BrightnessContrast; bc != nil {
		factor := (100 + bc.Contrast) / (100 - bc.Contrast)
		for i := range planes {
			planes[i] = mapPlane(planes[i], func(v float64) float64 { return (v-128)*factor + 128 + 1.275*bc.Brightness })
		}
	}
	res.Histogram = luminanceHistogram(planes)
	if r.Levels != nil {
		for i := range planes {
			planes[i] = levels(planes[i], *r.Levels)
		}
	}
	if r.Curves != nil {
		curve := monotoneCurve(r.Curves.Points)
		for i := range planes {
			planes[i] = mapPlane(planes[i], curve)
		}
	}

	res.Width, res.Height = planes[0].w, planes[0].h
	var img image.Image
	if res.Grey {
		img = planes[0].toGray()
	} else {
		rgba := image.NewNRGBA(image.Rect(0, 0, res.Width, res.Height))
		for i := range planes[0].p {
			for c := 0; c < 3; c++ {
				rgba.Pix[4*i+c] = uint8(clamp(math.Round(float64(planes[c].p[i])), 0, 255))
			}
			rgba.Pix[4*i+3] = 255
		}
		img = rgba
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return FaceResult{}, err
	}
	res.PNG = buf.Bytes()
	sum := sha256.Sum256(res.PNG)
	res.SHA256 = hex.EncodeToString(sum[:])
	return res, nil
}

// rgbPlanes splits img into its red, green and blue planes in one pass.
func rgbPlanes(img image.Image) [3]*plane {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := [3]*plane{newPlane(w, h), newPlane(w, h), newPlane(w, h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := y*w + x
			out[0].p[i], out[1].p[i], out[2].p[i] = float32(r)/257, float32(g)/257, float32(bl)/257
		}
	}
	return out
}

// toGrey combines the red, green and blue planes into one, as grey does for an image.
func toGrey(rgb []*plane, channel string) *plane {
	switch channel {
	case ChannelRed:
		return rgb[0]
	case ChannelGreen:
		return rgb[1]
	case ChannelBlue:
		return rgb[2]
	}
	out := newPlane(rgb[0].w, rgb[0].h)
	for i := range out.p {
		out.p[i] = 0.299*rgb[0].p[i] + 0.587*rgb[1].p[i] + 0.114*rgb[2].p[i]
	}
	return out
}

func mapPlane(src *plane, fn func(float64) float64) *plane {
	out := newPlane(src.w, src.h)
	for i, v := range src.p {
		out.p[i] = float32(clamp(fn(float64(v)), 0, 255))
	}
	return out
}

func luminanceHistogram(planes []*plane) [256]int {
	if len(planes) == 1 {
		return planes[0].histogram()
	}
	return toGrey(planes, ChannelLuminance).histogram()
}

// monotoneCurve is the Fritsch-Carlson monotone cubic through the points (sorted by input), with
// the ends (0, 0) and (255, 255) added when no point sets them; flat beyond the end points.
func monotoneCurve(points [][2]float64) func(float64) float64 {
	pts := append([][2]float64(nil), points...)
	if pts[0][0] > 0 {
		pts = append([][2]float64{{0, 0}}, pts...)
	}
	if pts[len(pts)-1][0] < 255 {
		pts = append(pts, [2]float64{255, 255})
	}
	n := len(pts)
	delta := make([]float64, n-1)
	for i := range delta {
		delta[i] = (pts[i+1][1] - pts[i][1]) / (pts[i+1][0] - pts[i][0])
	}
	m := make([]float64, n)
	m[0], m[n-1] = delta[0], delta[n-2]
	for i := 1; i < n-1; i++ {
		if delta[i-1]*delta[i] <= 0 {
			m[i] = 0
		} else {
			m[i] = (delta[i-1] + delta[i]) / 2
		}
	}
	for i := range delta {
		if delta[i] == 0 {
			m[i], m[i+1] = 0, 0
			continue
		}
		a, b := m[i]/delta[i], m[i+1]/delta[i]
		if s := a*a + b*b; s > 9 {
			t := 3 / math.Sqrt(s)
			m[i], m[i+1] = t*a*delta[i], t*b*delta[i]
		}
	}
	var lut [1024]float64
	for k := range lut {
		x := float64(k) * 255 / float64(len(lut)-1)
		i := sort.Search(n-1, func(j int) bool { return pts[j+1][0] >= x })
		if i >= n-1 {
			i = n - 2
		}
		x0, x1 := pts[i][0], pts[i+1][0]
		h := x1 - x0
		t := clamp((x-x0)/h, 0, 1)
		h00, h10 := 2*t*t*t-3*t*t+1, t*t*t-2*t*t+t
		h01, h11 := -2*t*t*t+3*t*t, t*t*t-t*t
		lut[k] = h00*pts[i][1] + h10*h*m[i] + h01*pts[i+1][1] + h11*h*m[i+1]
	}
	return func(v float64) float64 {
		f := clamp(v, 0, 255) / 255 * float64(len(lut)-1)
		i := int(f)
		if i >= len(lut)-1 {
			return lut[len(lut)-1]
		}
		t := f - float64(i)
		return lut[i]*(1-t) + lut[i+1]*t
	}
}

// FaceTransform is the transform from original to rendered pixels a normalised recipe gives, as
// RenderFace's result carries it, without rendering.
func FaceTransform(r FaceRecipe, width, height int) Affine {
	return affineGeometry(r.Crop, r.Rotate, r.Mirror, r.Scale, width, height).forward
}
