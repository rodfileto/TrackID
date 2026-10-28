package imageproc

import (
	"image"
	"math"
	"sort"
)

// plane is a grey image as float32 in 0..255, the working form of every step: no step rounds
// to 8 bits until the end.
type plane struct {
	w, h int
	p    []float32
}

func newPlane(w, h int) *plane { return &plane{w, h, make([]float32, w*h)} }

func (pl *plane) at(x, y int) float32 { return pl.p[y*pl.w+x] }

// grey converts img to one channel.
func grey(img image.Image, channel string) *plane {
	b := img.Bounds()
	pl := newPlane(b.Dx(), b.Dy())
	for y := 0; y < pl.h; y++ {
		for x := 0; x < pl.w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			R, G, B := float64(r)/257, float64(g)/257, float64(bl)/257
			var v float64
			switch channel {
			case ChannelRed:
				v = R
			case ChannelGreen:
				v = G
			case ChannelBlue:
				v = B
			default:
				v = 0.299*R + 0.587*G + 0.114*B
			}
			pl.p[y*pl.w+x] = float32(v)
		}
	}
	return pl
}

// toGray rounds to 8 bits.
func (pl *plane) toGray() *image.Gray {
	out := image.NewGray(image.Rect(0, 0, pl.w, pl.h))
	for i, v := range pl.p {
		out.Pix[i] = uint8(clamp(math.Round(float64(v)), 0, 255))
	}
	return out
}

func (pl *plane) histogram() [256]int {
	var h [256]int
	for _, v := range pl.p {
		h[int(clamp(math.Round(float64(v)), 0, 255))]++
	}
	return h
}

func (pl *plane) mean() float64 {
	if len(pl.p) == 0 {
		return 0
	}
	var s float64
	for _, v := range pl.p {
		s += float64(v)
	}
	return s / float64(len(pl.p))
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// gaussianBlur is a separable Gaussian, cut at 3 sigma, with edges extended.
func gaussianBlur(src *plane, sigma float64) *plane {
	if sigma <= 0 {
		out := newPlane(src.w, src.h)
		copy(out.p, src.p)
		return out
	}
	radius := int(math.Ceil(3 * sigma))
	kernel := make([]float32, 2*radius+1)
	var sum float64
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		kernel[i+radius] = float32(v)
		sum += v
	}
	for i := range kernel {
		kernel[i] /= float32(sum)
	}
	tmp := newPlane(src.w, src.h)
	for y := 0; y < src.h; y++ {
		row := src.p[y*src.w : (y+1)*src.w]
		for x := 0; x < src.w; x++ {
			var acc float32
			for k := -radius; k <= radius; k++ {
				xx := min(max(x+k, 0), src.w-1)
				acc += row[xx] * kernel[k+radius]
			}
			tmp.p[y*src.w+x] = acc
		}
	}
	out := newPlane(src.w, src.h)
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			var acc float32
			for k := -radius; k <= radius; k++ {
				yy := min(max(y+k, 0), src.h-1)
				acc += tmp.p[yy*src.w+x] * kernel[k+radius]
			}
			out.p[y*src.w+x] = acc
		}
	}
	return out
}

// flatten divides by the blurred image, scaled back to the image's mean: multiplicative uneven
// lighting (a finger lit from one side) is removed, the ridges' local contrast kept.
func flatten(src *plane, sigmaPX float64) *plane {
	blur := gaussianBlur(src, sigmaPX)
	m := float32(src.mean())
	out := newPlane(src.w, src.h)
	for i, v := range src.p {
		out.p[i] = float32(clamp(float64(v/max(blur.p[i], 1)*m), 0, 255))
	}
	return out
}

func levels(src *plane, l Levels) *plane {
	out := newPlane(src.w, src.h)
	span := l.InWhite - l.InBlack
	for i, v := range src.p {
		t := clamp((float64(v)-l.InBlack)/span, 0, 1)
		t = math.Pow(t, 1/l.Gamma)
		out.p[i] = float32(l.OutBlack + t*(l.OutWhite-l.OutBlack))
	}
	return out
}

func invert(src *plane) *plane {
	out := newPlane(src.w, src.h)
	for i, v := range src.p {
		out.p[i] = 255 - v
	}
	return out
}

// clahe equalises each tile's histogram with its counts clipped at clipLimit times the mean
// count (the excess spread over all bins), and blends the four nearest tiles' mappings
// bilinearly, as OpenCV's createCLAHE.
func clahe(src *plane, tiles int, clipLimit float64) *plane {
	tw := int(math.Ceil(float64(src.w) / float64(tiles)))
	th := int(math.Ceil(float64(src.h) / float64(tiles)))
	maps := make([][256]float32, tiles*tiles)
	for ty := 0; ty < tiles; ty++ {
		for tx := 0; tx < tiles; tx++ {
			var hist [256]float64
			n := 0
			for y := ty * th; y < min((ty+1)*th, src.h); y++ {
				for x := tx * tw; x < min((tx+1)*tw, src.w); x++ {
					hist[int(clamp(math.Round(float64(src.at(x, y))), 0, 255))]++
					n++
				}
			}
			m := &maps[ty*tiles+tx]
			if n == 0 {
				for i := range m {
					m[i] = float32(i)
				}
				continue
			}
			limit := math.Max(1, clipLimit*float64(n)/256)
			var excess float64
			for i := range hist {
				if hist[i] > limit {
					excess += hist[i] - limit
					hist[i] = limit
				}
			}
			var cdf float64
			for i := range hist {
				cdf += hist[i] + excess/256
				m[i] = float32(cdf / float64(n) * 255)
			}
		}
	}
	out := newPlane(src.w, src.h)
	for y := 0; y < src.h; y++ {
		fy := (float64(y)+0.5)/float64(th) - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)
		y0c, y1c := min(max(y0, 0), tiles-1), min(max(y0+1, 0), tiles-1)
		for x := 0; x < src.w; x++ {
			fx := (float64(x)+0.5)/float64(tw) - 0.5
			x0 := int(math.Floor(fx))
			wx := fx - float64(x0)
			x0c, x1c := min(max(x0, 0), tiles-1), min(max(x0+1, 0), tiles-1)
			v := int(clamp(math.Round(float64(src.at(x, y))), 0, 255))
			a := float64(maps[y0c*tiles+x0c][v])
			b := float64(maps[y0c*tiles+x1c][v])
			c := float64(maps[y1c*tiles+x0c][v])
			d := float64(maps[y1c*tiles+x1c][v])
			out.p[y*src.w+x] = float32((a*(1-wx)+b*wx)*(1-wy) + (c*(1-wx)+d*wx)*wy)
		}
	}
	return out
}

// percentile of the plane's values, p in 0..1.
func (pl *plane) percentile(p float64) float64 {
	vals := make([]float64, len(pl.p))
	for i, v := range pl.p {
		vals[i] = math.Abs(float64(v))
	}
	sort.Float64s(vals)
	if len(vals) == 0 {
		return 0
	}
	return vals[min(len(vals)-1, int(p*float64(len(vals))))]
}
