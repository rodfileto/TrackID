package videoproc

import "image"

// blurScore is the variance of the Laplacian over a cropped region -- a
// standard cheap sharpness measure. A crisp region has strong edges in many
// directions, which the Laplacian (a discrete second-derivative operator)
// turns into large values; blurring smooths those out, so the variance
// collapses toward zero. Higher is sharper.
func blurScore(img image.Image, box [4]float32) float64 {
	bounds := img.Bounds()
	x0 := clampInt(int(box[0]), bounds.Min.X, bounds.Max.X)
	y0 := clampInt(int(box[1]), bounds.Min.Y, bounds.Max.Y)
	x1 := clampInt(int(box[2]), bounds.Min.X, bounds.Max.X)
	y1 := clampInt(int(box[3]), bounds.Min.Y, bounds.Max.Y)
	w, h := x1-x0, y1-y0
	if w < 3 || h < 3 {
		return 0
	}

	gray := make([][]float64, h)
	for y := 0; y < h; y++ {
		gray[y] = make([]float64, w)
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x0+x, y0+y).RGBA()
			gray[y][x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
		}
	}

	var lap []float64
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			v := gray[y-1][x] + gray[y+1][x] + gray[y][x-1] + gray[y][x+1] - 4*gray[y][x]
			lap = append(lap, v)
		}
	}
	if len(lap) == 0 {
		return 0
	}

	var mean float64
	for _, v := range lap {
		mean += v
	}
	mean /= float64(len(lap))

	var variance float64
	for _, v := range lap {
		d := v - mean
		variance += d * d
	}
	return variance / float64(len(lap))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
