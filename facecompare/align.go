package facecompare

import (
	"errors"
	"math"
	"math/cmplx"

	"github.com/rodfileto/trackid/imageproc"
)

// Similarity is a transform that only rotates, scales uniformly and translates: it keeps shapes
// and proportions, as FISWG's methodology guidelines (v2.1, §6.3.2) require of alignment for
// superimposition. It maps the known image's rendered pixels onto the questioned image's.
type Similarity struct {
	Transform imageproc.Affine `json:"transform"`
	Scale     float64          `json:"scale"`
	Rotation  float64          `json:"rotation"` // degrees, clockwise on screen
	// RMS is the root mean square distance, in questioned rendered pixels, between the questioned
	// points and the known points carried over: how well the points agree once aligned.
	RMS   float64 `json:"rms"`
	Pairs int     `json:"pairs"`
}

// ErrTooFewPairs is Fit's refusal with fewer than two point pairs.
var ErrTooFewPairs = errors.New("alignment needs at least two pairs of corresponding points")

// Fit is the least-squares similarity carrying each k onto its q (q ≈ s·R·k + t). Written with
// points as complex numbers, the optimum is a = Σ(q−q̄)·conj(k−k̄) / Σ|k−k̄|², t = q̄ − a·k̄.
func Fit(q, k [][2]float64) (Similarity, error) {
	if len(q) != len(k) || len(q) < 2 {
		return Similarity{}, ErrTooFewPairs
	}
	n := complex(float64(len(q)), 0)
	var qm, km complex128
	for i := range q {
		qm += complex(q[i][0], q[i][1])
		km += complex(k[i][0], k[i][1])
	}
	qm, km = qm/n, km/n
	var num complex128
	var den float64
	for i := range q {
		dq := complex(q[i][0], q[i][1]) - qm
		dk := complex(k[i][0], k[i][1]) - km
		num += dq * cmplx.Conj(dk)
		den += real(dk * cmplx.Conj(dk))
	}
	if den < 1e-9 {
		return Similarity{}, errors.New("the known points coincide")
	}
	a := num / complex(den, 0)
	t := qm - a*km
	sim := Similarity{
		Transform: imageproc.Affine{A: real(a), B: -imag(a), C: real(t), D: imag(a), E: real(a), F: imag(t)},
		Scale:     cmplx.Abs(a),
		Rotation:  cmplx.Phase(a) * 180 / math.Pi,
		Pairs:     len(q),
	}
	var sq float64
	for i := range q {
		x, y := sim.Transform.Apply(k[i][0], k[i][1])
		sq += (x-q[i][0])*(x-q[i][0]) + (y-q[i][1])*(y-q[i][1])
	}
	sim.RMS = math.Sqrt(sq / float64(len(q)))
	return sim, nil
}

// Align fits the similarity between the two rendered images from point pairs given in each
// image's original pixels, carried through each image's processing (qt, kt: original to render).
func Align(pairs []PointPair, qt, kt imageproc.Affine) (Similarity, error) {
	q := make([][2]float64, len(pairs))
	k := make([][2]float64, len(pairs))
	for i, p := range pairs {
		q[i][0], q[i][1] = qt.Apply(p.Q[0], p.Q[1])
		k[i][0], k[i][1] = kt.Apply(p.K[0], p.K[1])
	}
	return Fit(q, k)
}

// EyePairs pairs the two images' eye centres from their landmarks, the default alignment.
func EyePairs(q, k Landmarks) []PointPair {
	return []PointPair{{Q: q.Points[0], K: k.Points[0]}, {Q: q.Points[1], K: k.Points[1]}}
}

// Compose returns the affine that applies first then second.
func Compose(first, second imageproc.Affine) imageproc.Affine {
	return imageproc.Affine{
		A: second.A*first.A + second.B*first.D,
		B: second.A*first.B + second.B*first.E,
		C: second.A*first.C + second.B*first.F + second.C,
		D: second.D*first.A + second.E*first.D,
		E: second.D*first.B + second.E*first.E,
		F: second.D*first.C + second.E*first.F + second.F,
	}
}
