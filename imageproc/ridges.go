package imageproc

import (
	"math"
	"math/cmplx"
	"sort"
)

// RidgePeriod is the dominant ridge period of a grey plane, in its pixels, from the peak of its
// radially averaged power spectrum within [minPeriod, maxPeriod]. Confidence is the peak's
// power over the median of the rings searched: near 1 means no clear periodic pattern.
func ridgePeriod(src *plane, minPeriod, maxPeriod float64) (period, confidence float64) {
	n := 1
	for n*2 <= min(src.w, src.h) && n*2 <= 1024 {
		n *= 2
	}
	if n < 32 {
		return 0, 0
	}
	// The centred n x n window, mean removed, under a 2D Hann window.
	x0, y0 := (src.w-n)/2, (src.h-n)/2
	var mean float64
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			mean += float64(src.at(x0+x, y0+y))
		}
	}
	mean /= float64(n * n)
	data := make([]complex128, n*n)
	hann := make([]float64, n)
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			data[y*n+x] = complex((float64(src.at(x0+x, y0+y))-mean)*hann[x]*hann[y], 0)
		}
	}
	fft2(data, n)
	maxRing := n / 2
	power := make([]float64, maxRing+1)
	count := make([]float64, maxRing+1)
	for v := 0; v < n; v++ {
		fv := v
		if fv > n/2 {
			fv -= n
		}
		for u := 0; u < n; u++ {
			fu := u
			if fu > n/2 {
				fu -= n
			}
			r := int(math.Round(math.Hypot(float64(fu), float64(fv))))
			if r <= maxRing {
				a := cmplx.Abs(data[v*n+u])
				power[r] += a * a
				count[r]++
			}
		}
	}
	lo := max(2, int(math.Floor(float64(n)/maxPeriod)))
	hi := min(maxRing-1, int(math.Ceil(float64(n)/minPeriod)))
	if hi <= lo {
		return 0, 0
	}
	best := lo
	rings := make([]float64, 0, hi-lo+1)
	for r := lo; r <= hi; r++ {
		power[r] /= math.Max(count[r], 1)
		rings = append(rings, power[r])
	}
	for r := lo; r <= hi; r++ {
		if power[r] > power[best] {
			best = r
		}
	}
	// Parabolic refinement of the peak between its neighbours.
	peak := float64(best)
	if best > lo && best < hi {
		a, b, c := power[best-1], power[best], power[best+1]
		if d := a - 2*b + c; d != 0 {
			peak += 0.5 * (a - c) / d
		}
	}
	sort.Float64s(rings)
	median := rings[len(rings)/2]
	if median <= 0 {
		return float64(n) / peak, 0
	}
	return float64(n) / peak, power[best] / median
}

// fft2 is an in-place 2D FFT of an n x n array, n a power of two.
func fft2(data []complex128, n int) {
	row := make([]complex128, n)
	for y := 0; y < n; y++ {
		fft(data[y*n : (y+1)*n])
	}
	for x := 0; x < n; x++ {
		for y := 0; y < n; y++ {
			row[y] = data[y*n+x]
		}
		fft(row)
		for y := 0; y < n; y++ {
			data[y*n+x] = row[y]
		}
	}
}

// fft is an iterative radix-2 Cooley-Tukey FFT, in place.
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := a[start+k], a[start+k+size/2]*w
				a[start+k], a[start+k+size/2] = u+v, u-v
				w *= step
			}
		}
	}
}

// Ridge period limits at OutputPPI: adult ridge spacing is about 0.4-0.5 mm, 8-10 pixels at
// 500 ppi; the window allows for children's prints and a wrong source resolution.
const (
	minPeriodAtOutput     = 5
	maxPeriodAtOutput     = 20
	defaultPeriodAtOutput = 9
)

// gaborEnhance filters each pixel with an even-symmetric Gabor filter oriented along the local
// ridge flow and tuned to the image's ridge period (Hong, Wan & Jain, 1998). Orientation comes
// from smoothed gradients; the period from the power spectrum. Pixels without a ridge pattern take
// paper: flat areas, areas where the filters respond weakly (background noise has no consistent
// ridge flow at the ridge frequency), and the strip along the image's edge (valid false: outside
// the original after rotation), where the filters would ring. The result keeps the input's
// polarity: ridges that were dark stay dark.
func gaborEnhance(src *plane, valid []bool, orientations int, paper float32) (*plane, float64) {
	// Normalise to zero mean, unit variance.
	m := src.mean()
	var v float64
	for _, x := range src.p {
		d := float64(x) - m
		v += d * d
	}
	sd := math.Sqrt(v / float64(len(src.p)))
	if sd == 0 {
		sd = 1
	}
	norm := newPlane(src.w, src.h)
	for i, x := range src.p {
		norm.p[i] = float32((float64(x) - m) / sd)
	}

	period, confidence := ridgePeriod(norm, minPeriodAtOutput, maxPeriodAtOutput)
	if period == 0 || confidence < 2 {
		period = defaultPeriodAtOutput
	}

	// Orientation field: the doubled-angle average of the gradients.
	smooth := gaussianBlur(norm, 1)
	gxx, gyy, gxy := newPlane(src.w, src.h), newPlane(src.w, src.h), newPlane(src.w, src.h)
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			at := func(dx, dy int) float32 {
				return smooth.at(min(max(x+dx, 0), src.w-1), min(max(y+dy, 0), src.h-1))
			}
			gx := (at(1, -1) + 2*at(1, 0) + at(1, 1)) - (at(-1, -1) + 2*at(-1, 0) + at(-1, 1))
			gy := (at(-1, 1) + 2*at(0, 1) + at(1, 1)) - (at(-1, -1) + 2*at(0, -1) + at(1, -1))
			i := y*src.w + x
			gxx.p[i], gyy.p[i], gxy.p[i] = gx*gx, gy*gy, gx*gy
		}
	}
	blockSigma := 0.75 * period
	gxx, gyy, gxy = gaussianBlur(gxx, blockSigma), gaussianBlur(gyy, blockSigma), gaussianBlur(gxy, blockSigma)
	sin2, cos2 := newPlane(src.w, src.h), newPlane(src.w, src.h)
	for i := range gxx.p {
		den := math.Hypot(float64(gxy.p[i]), float64(gxx.p[i]-gyy.p[i])/2) + 1e-9
		sin2.p[i] = float32(float64(gxy.p[i]) / den)
		cos2.p[i] = float32(float64(gxx.p[i]-gyy.p[i]) / 2 / den)
	}
	sin2, cos2 = gaussianBlur(sin2, blockSigma), gaussianBlur(cos2, blockSigma)

	// Ridge mask: local standard deviation of the normalised image.
	sq := newPlane(src.w, src.h)
	for i, x := range norm.p {
		sq.p[i] = x * x
	}
	maskSigma := 1.5 * period
	localMean, localSq := gaussianBlur(norm, maskSigma), gaussianBlur(sq, maskSigma)

	// One kernel per orientation bin; the kernel's cosine runs across the ridges.
	sigma := 0.65 * period
	radius := int(math.Ceil(3 * sigma))
	size := 2*radius + 1
	kernels := make([][]float32, orientations)
	for k := range kernels {
		theta := math.Pi * float64(k) / float64(orientations) // ridge direction
		kern := make([]float64, size*size)
		var mean float64
		for y := -radius; y <= radius; y++ {
			for x := -radius; x <= radius; x++ {
				across := -float64(x)*math.Sin(theta) + float64(y)*math.Cos(theta)
				g := math.Exp(-float64(x*x+y*y)/(2*sigma*sigma)) * math.Cos(2*math.Pi*across/period)
				kern[(y+radius)*size+x+radius] = g
				mean += g
			}
		}
		mean /= float64(size * size)
		kernels[k] = make([]float32, size*size)
		for i, g := range kern {
			kernels[k][i] = float32(g - mean)
		}
	}

	resp := newPlane(src.w, src.h)
	masked := make([]bool, len(src.p))
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			i := y*src.w + x
			variance := float64(localSq.p[i]) - float64(localMean.p[i])*float64(localMean.p[i])
			if variance < 0.04 { // local sd under 0.2 of the image's
				masked[i] = true
				continue
			}
			// Gradient angle is half the doubled angle; ridges run perpendicular to it.
			theta := 0.5*math.Atan2(float64(sin2.p[i]), float64(cos2.p[i])) + math.Pi/2
			bin := int(math.Round(theta/math.Pi*float64(orientations))) % orientations
			if bin < 0 {
				bin += orientations
			}
			kern := kernels[bin]
			var acc float32
			for ky := -radius; ky <= radius; ky++ {
				yy := min(max(y+ky, 0), src.h-1)
				row := norm.p[yy*src.w:]
				krow := kern[(ky+radius)*size:]
				for kx := -radius; kx <= radius; kx++ {
					xx := min(max(x+kx, 0), src.w-1)
					acc += row[xx] * krow[kx+radius]
				}
			}
			resp.p[i] = acc
		}
	}
	// Away from the edge: a pixel within a kernel radius of an invalid one is masked too.
	if valid != nil {
		edge := newPlane(src.w, src.h)
		for i, ok := range valid {
			if ok {
				edge.p[i] = 1
			}
		}
		edge = gaussianBlur(edge, float64(radius)/2)
		for i := range masked {
			if edge.p[i] < 0.98 {
				masked[i] = true
			}
		}
	}
	// Ridge strength: the response's local magnitude, against the strongest ridge areas'.
	magnitude := newPlane(src.w, src.h)
	for i, r := range resp.p {
		if !masked[i] {
			magnitude.p[i] = float32(math.Abs(float64(r)))
		}
	}
	strength := gaussianBlur(magnitude, period)
	strong := newPlane(0, 0)
	for i, v := range strength.p {
		if !masked[i] {
			strong.p = append(strong.p, v)
		}
	}
	threshold := float32(0.3 * strong.percentile(0.95))
	for i, v := range strength.p {
		if v < threshold {
			masked[i] = true
		}
	}

	kept := newPlane(0, 0)
	for i, r := range resp.p {
		if !masked[i] {
			kept.p = append(kept.p, r)
		}
	}
	scale := kept.percentile(0.99)
	if scale == 0 {
		scale = 1
	}
	out := newPlane(src.w, src.h)
	for i, r := range resp.p {
		if masked[i] {
			out.p[i] = paper
			continue
		}
		out.p[i] = float32(127.5 + 127.5*clamp(float64(r)/scale, -1, 1))
	}
	return out, period
}
