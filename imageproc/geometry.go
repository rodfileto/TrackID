package imageproc

import "math"

// Affine maps original pixel coordinates to rendered ones: x' = A*x + B*y + C, y' = D*x + E*y + F.
// Points marked on the original are drawn on the render through it, and back through Invert.
type Affine struct {
	A, B, C, D, E, F float64
}

// Apply maps (x, y).
func (m Affine) Apply(x, y float64) (float64, float64) {
	return m.A*x + m.B*y + m.C, m.D*x + m.E*y + m.F
}

// Invert is the inverse map.
func (m Affine) Invert() Affine {
	det := m.A*m.E - m.B*m.D
	a, b, d, e := m.E/det, -m.B/det, -m.D/det, m.A/det
	return Affine{a, b, -(a*m.C + b*m.F), d, e, -(d*m.C + e*m.F)}
}

// geometry is the crop, rotation, mirror and scale to OutputPPI as one affine transform, so the
// image is interpolated once.
type geometry struct {
	crop          Rect
	scale         float64
	forward       Affine
	width, height int
}

func newGeometry(r Recipe, width, height int) geometry {
	return affineGeometry(r.Crop, r.Rotate, r.Mirror, OutputPPI/r.Source.PPI, width, height)
}

// affineGeometry is the crop (the whole image when nil), clockwise rotation in degrees, mirror and
// scale as one transform.
func affineGeometry(cropRect *Rect, rotate float64, mirror bool, scale float64, width, height int) geometry {
	crop := Rect{0, 0, float64(width), float64(height)}
	if cropRect != nil {
		crop = *cropRect
	}
	theta := rotate * math.Pi / 180
	cos, sin := math.Cos(theta), math.Sin(theta)
	mx := 1.0
	if mirror {
		mx = -1 // after the rotation: a horizontal flip of the image as rendered
	}
	// Linear part: scale * Mirror * Rotation (clockwise on screen, y pointing down).
	a, b := scale*mx*cos, -scale*mx*sin
	d, e := scale*sin, scale*cos
	cx, cy := (crop.X1+crop.X2)/2, (crop.Y1+crop.Y2)/2
	// The rendered size is the bounding box of the transformed crop.
	var minX, minY, maxX, maxY = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range [][2]float64{{crop.X1, crop.Y1}, {crop.X2, crop.Y1}, {crop.X1, crop.Y2}, {crop.X2, crop.Y2}} {
		x, y := a*(p[0]-cx)+b*(p[1]-cy), d*(p[0]-cx)+e*(p[1]-cy)
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	w := max(1, int(math.Round(maxX-minX)))
	h := max(1, int(math.Round(maxY-minY)))
	ox, oy := float64(w)/2, float64(h)/2
	return geometry{
		crop:    crop,
		scale:   scale,
		forward: Affine{a, b, ox - (a*cx + b*cy), d, e, oy - (d*cx + e*cy)},
		width:   w,
		height:  h,
	}
}

// antialiasSigma is the Gaussian blur applied before reducing the image (scale < 1), so ridges
// and noise finer than the new pixel spacing do not alias. Zero when enlarging or for nearest
// neighbour, which keeps original pixel values by definition.
func antialiasSigma(scale float64, interpolation string) float64 {
	if scale >= 1 || interpolation == InterpolationNearest {
		return 0
	}
	return 0.5 * math.Sqrt(1/(scale*scale)-1)
}

// warp renders src through g, sampling inside the crop only; rendered pixels whose source falls
// outside it take fill, and are false in the returned mask.
func warp(src *plane, g geometry, interpolation string, fill float32) (*plane, []bool) {
	out := newPlane(g.width, g.height)
	valid := make([]bool, g.width*g.height)
	inv := g.forward.Invert()
	x0, y0 := g.crop.X1, g.crop.Y1
	x1, y1 := g.crop.X2, g.crop.Y2
	for j := 0; j < g.height; j++ {
		for i := 0; i < g.width; i++ {
			// Pixel centres: the centre of pixel (i, j) is at (i+0.5, j+0.5).
			sx, sy := inv.Apply(float64(i)+0.5, float64(j)+0.5)
			if sx < x0 || sy < y0 || sx >= x1 || sy >= y1 {
				out.p[j*g.width+i] = fill
				continue
			}
			out.p[j*g.width+i] = sample(src, sx-0.5, sy-0.5, interpolation, g.crop)
			valid[j*g.width+i] = true
		}
	}
	return out, valid
}

// sample reads src at (x, y) in pixel-index coordinates, clamping to the crop's pixels.
func sample(src *plane, x, y float64, interpolation string, crop Rect) float32 {
	minX, minY := int(math.Floor(crop.X1)), int(math.Floor(crop.Y1))
	maxX, maxY := min(int(math.Ceil(crop.X2))-1, src.w-1), min(int(math.Ceil(crop.Y2))-1, src.h-1)
	px := func(ix, iy int) float64 {
		return float64(src.at(min(max(ix, minX), maxX), min(max(iy, minY), maxY)))
	}
	switch interpolation {
	case InterpolationNearest:
		return float32(px(int(math.Round(x)), int(math.Round(y))))
	case InterpolationBilinear:
		fx, fy := math.Floor(x), math.Floor(y)
		tx, ty := x-fx, y-fy
		ix, iy := int(fx), int(fy)
		top := px(ix, iy)*(1-tx) + px(ix+1, iy)*tx
		bottom := px(ix, iy+1)*(1-tx) + px(ix+1, iy+1)*tx
		return float32(top*(1-ty) + bottom*ty)
	default:
		fx, fy := math.Floor(x), math.Floor(y)
		tx, ty := x-fx, y-fy
		ix, iy := int(fx), int(fy)
		var acc float64
		for m := -1; m <= 2; m++ {
			wy := catmullRom(float64(m) - ty)
			var row float64
			for n := -1; n <= 2; n++ {
				row += px(ix+n, iy+m) * catmullRom(float64(n)-tx)
			}
			acc += row * wy
		}
		return float32(clamp(acc, 0, 255))
	}
}

// catmullRom is the cubic convolution kernel with a = -0.5 (Keys, 1981).
func catmullRom(t float64) float64 {
	t = math.Abs(t)
	switch {
	case t < 1:
		return 1.5*t*t*t - 2.5*t*t + 1
	case t < 2:
		return -0.5*t*t*t + 2.5*t*t - 4*t + 2
	}
	return 0
}
