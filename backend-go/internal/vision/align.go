package vision

// arcfaceTemplate is insightface's canonical 5-point layout for a 112x112
// aligned face crop (utils/face_align.py's arcface_dst).
var arcfaceTemplate = [5][2]float64{
	{38.2946, 51.6963},
	{73.5318, 51.5014},
	{56.0252, 71.7366},
	{41.5493, 92.3655},
	{70.7299, 92.2041},
}

const alignedSize = 112

// similarity is a 2D similarity transform (uniform scale + rotation +
// translation), represented as complex multiplier `a` and offset `b`:
// dst = a*src + b (src, dst, a, b all treated as complex numbers x+iy).
type similarity struct {
	a complex128
	b complex128
}

// estimateSimilarity finds the least-squares similarity transform mapping
// src points onto dst points. This is the 2D specialization of the Umeyama
// algorithm (skimage's SimilarityTransform.estimate uses the same math via
// SVD; for 2D points a direct complex-linear-regression solution is
// equivalent and avoids needing a general SVD routine): treating each point
// as a complex number, dst_i - dst_mean = a*(src_i - src_mean) is exactly the
// non-reflective similarity least-squares fit.
func estimateSimilarity(src, dst [5][2]float64) similarity {
	var srcMean, dstMean complex128
	for i := 0; i < 5; i++ {
		srcMean += complex(src[i][0], src[i][1])
		dstMean += complex(dst[i][0], dst[i][1])
	}
	srcMean /= 5
	dstMean /= 5

	var num complex128
	var den float64
	for i := 0; i < 5; i++ {
		sc := complex(src[i][0], src[i][1]) - srcMean
		dc := complex(dst[i][0], dst[i][1]) - dstMean
		num += cmplxConj(sc) * dc
		den += real(sc)*real(sc) + imag(sc)*imag(sc)
	}

	a := num / complex(den, 0)
	b := dstMean - a*srcMean
	return similarity{a: a, b: b}
}

func cmplxConj(z complex128) complex128 {
	return complex(real(z), -imag(z))
}

// warpAffine produces the alignedSize x alignedSize crop by inverse-mapping
// each output pixel through the transform (matching cv2.warpAffine with
// forward matrix M: it samples input at M^-1(output)).
func warpAffine(img *rgbImage, s similarity) *rgbImage {
	// Invert: src = a^-1 * (dst - b)
	inv := 1 / s.a
	out := &rgbImage{w: alignedSize, h: alignedSize, pix: make([]uint8, alignedSize*alignedSize*3)}
	for oy := 0; oy < alignedSize; oy++ {
		for ox := 0; ox < alignedSize; ox++ {
			dstPt := complex(float64(ox), float64(oy))
			srcPt := inv * (dstPt - s.b)
			r, g, b := img.bilinear(real(srcPt), imag(srcPt))
			i := (oy*alignedSize + ox) * 3
			out.pix[i] = clampU8(r)
			out.pix[i+1] = clampU8(g)
			out.pix[i+2] = clampU8(b)
		}
	}
	return out
}

// alignFace crops+warps the face at the given 5-point landmarks into a
// canonical 112x112 image ready for the recognition model.
func alignFace(img *rgbImage, kps [5][2]float32) *rgbImage {
	var src [5][2]float64
	for i, p := range kps {
		src[i] = [2]float64{float64(p[0]), float64(p[1])}
	}
	s := estimateSimilarity(src, arcfaceTemplate)
	return warpAffine(img, s)
}
