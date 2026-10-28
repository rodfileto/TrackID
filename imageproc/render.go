package imageproc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"math"
)

// Result is a render and what its record needs.
type Result struct {
	Image  *image.Gray
	PNG    []byte
	SHA256 string // of PNG
	Width  int
	Height int
	// Transform maps original pixel coordinates to the render's.
	Transform Affine
	// LevelsHistogram is the histogram levels work on (after the geometry and the lighting
	// correction), for the levels control.
	LevelsHistogram [256]int
	// What the steps computed, for the record.
	Scale          float64 // OutputPPI / source ppi
	AntialiasSigma float64 // source pixels; 0 when not applied
	Fill           float64 // grey given to pixels outside the original after rotation
	FlattenSigmaPX float64
	GaborPeriodPX  float64 // ridge period the Gabor filters were tuned to, at OutputPPI
	GaborEstimated bool    // false when no clear period was found and the default was used
}

// Render applies r to o. r must have been normalised against o (Recipe.Normalize). The order
// is fixed: channel; crop, rotation, mirror and resampling to OutputPPI in one transform (one
// interpolation); lighting correction; levels; CLAHE; Gabor; inversion. Steps that depend on
// distances (lighting correction, Gabor) run at OutputPPI, where those distances are known.
func Render(o Original, r Recipe) (Result, error) {
	g := newGeometry(r, o.Width, o.Height)
	// Work on the crop plus a margin for the interpolation and anti-alias kernels only.
	sigma := antialiasSigma(g.scale, r.Interpolation)
	margin := int(math.Ceil(3*sigma)) + 3
	region := image.Rect(
		int(math.Floor(g.crop.X1))-margin, int(math.Floor(g.crop.Y1))-margin,
		int(math.Ceil(g.crop.X2))+margin, int(math.Ceil(g.crop.Y2))+margin,
	).Intersect(image.Rect(0, 0, o.Width, o.Height))
	sub := subImage(o.Image, region)
	src := grey(sub, r.Channel)
	shifted := r
	crop := Rect{g.crop.X1 - float64(region.Min.X), g.crop.Y1 - float64(region.Min.Y), g.crop.X2 - float64(region.Min.X), g.crop.Y2 - float64(region.Min.Y)}
	shifted.Crop = &crop
	local := newGeometry(shifted, src.w, src.h)

	fill := float32(cropMean(src, crop))
	if sigma > 0 {
		src = gaussianBlur(src, sigma)
	}
	pl, valid := warp(src, local, r.Interpolation, fill)

	res := Result{Scale: g.scale, AntialiasSigma: sigma, Fill: float64(fill)}
	f := local.forward
	ox, oy := float64(region.Min.X), float64(region.Min.Y)
	res.Transform = Affine{f.A, f.B, f.C - f.A*ox - f.B*oy, f.D, f.E, f.F - f.D*ox - f.E*oy}

	if r.Flatten != nil {
		res.FlattenSigmaPX = r.Flatten.SigmaMM / 25.4 * OutputPPI
		pl = flatten(pl, res.FlattenSigmaPX)
	}
	res.LevelsHistogram = pl.histogram()
	if r.Levels != nil {
		pl = levels(pl, *r.Levels)
	}
	if r.CLAHE != nil {
		pl = clahe(pl, r.CLAHE.Tiles, r.CLAHE.ClipLimit)
	}
	if r.Gabor != nil {
		paper := float32(255)
		if r.Invert {
			paper = 0 // white once inverted
		}
		var period float64
		pl, period = gaborEnhance(pl, valid, r.Gabor.Orientations, paper)
		res.GaborPeriodPX = period
		res.GaborEstimated = period != defaultPeriodAtOutput
	}
	if r.Invert {
		pl = invert(pl)
	}
	res.Image = pl.toGray()
	res.Width, res.Height = pl.w, pl.h
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, res.Image); err != nil {
		return Result{}, err
	}
	res.PNG = withPHYs(buf.Bytes(), OutputPPI)
	sum := sha256.Sum256(res.PNG)
	res.SHA256 = hex.EncodeToString(sum[:])
	return res, nil
}

// EstimateRidgePeriod is the original's dominant ridge period in its own pixels, inside crop
// (the whole image when nil), for estimating the source resolution.
func EstimateRidgePeriod(o Original, crop *Rect, channel string) (period, confidence float64) {
	region := image.Rect(0, 0, o.Width, o.Height)
	if crop != nil {
		region = image.Rect(int(crop.X1), int(crop.Y1), int(crop.X2), int(crop.Y2)).Intersect(region)
	}
	if channel == "" {
		channel = ChannelLuminance
	}
	pl := grey(subImage(o.Image, region), channel)
	// Periods from 3 pixels up to a quarter of the window: at least four ridges in view.
	return ridgePeriod(pl, 3, float64(min(pl.w, pl.h, 1024))/4)
}

func subImage(img image.Image, r image.Rectangle) image.Image {
	if s, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(r.Add(img.Bounds().Min))
	}
	return img
}

func cropMean(pl *plane, c Rect) float64 {
	var sum float64
	n := 0
	for y := int(c.Y1); y < min(int(math.Ceil(c.Y2)), pl.h); y++ {
		for x := int(c.X1); x < min(int(math.Ceil(c.X2)), pl.w); x++ {
			sum += float64(pl.at(x, y))
			n++
		}
	}
	if n == 0 {
		return 128
	}
	return sum / float64(n)
}
