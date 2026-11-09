// Package calibration measures how a face model's similarity scores separate the same person from different
// people, to choose a matching threshold (cmd/calibrate-face-threshold, then cmd/match-threshold). It knows nothing
// of where the truth comes from: a Source says which faces belong to one identity, and each deployment picks the
// source it trusts (IdentitySource over the identity tables, configured by which field marks one person, or a
// Source of its own). It reads vectors and writes nothing.
//
// From a random sample of identities, every pair of faces of one identity is a genuine pair, and every pair across
// two identities an impostor pair. FNMR(t) is the share of genuine pairs scoring below t; FMR(t) the share of
// impostor pairs at or above it.
package calibration

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// Face is one face vector of a sampled identity.
type Face struct {
	ID       int64  // the vector's own id, for the source's bookkeeping
	Identity string // faces sharing it are the same person; a source's own key
	Vector   []float32
}

// Source gives a random sample of identities that have at least two faces each, with all their faces (up to
// maxFaces each), drawn with seed. Which faces count as one person is the source's whole job.
type Source interface {
	Sample(ctx context.Context, identities, maxFaces int, seed int64) ([]Face, error)
	// Describe says what the source is, for the report: where the truth comes from.
	Describe() string
}

// Options configures Run.
type Options struct {
	Identities       int       // identities sampled; default 3000
	MaxFaces         int       // faces kept per identity; default 5
	MaxImpostorPairs int       // impostor pairs scored, drawn at random when there are more; default 5 000 000
	Seed             int64     // the sample's seed; default 1
	Step             float64   // the threshold grid's spacing; default 0.02
	FMRTargets       []float64 // false match rates to find the threshold of; default 1e-2, 1e-3, 1e-4
}

func (o Options) withDefaults() Options {
	if o.Identities <= 0 {
		o.Identities = 3000
	}
	if o.MaxFaces < 2 {
		o.MaxFaces = 5
	}
	if o.MaxImpostorPairs <= 0 {
		o.MaxImpostorPairs = 5_000_000
	}
	if o.Seed == 0 {
		o.Seed = 1
	}
	if o.Step <= 0 {
		o.Step = 0.02
	}
	if len(o.FMRTargets) == 0 {
		o.FMRTargets = []float64{1e-2, 1e-3, 1e-4}
	}
	return o
}

// Row is the curves at one threshold.
type Row struct {
	Threshold float64
	FMR       float64 // impostor pairs at or above, over impostor pairs
	FNMR      float64 // genuine pairs below, over genuine pairs
}

// Target is the lowest threshold whose FMR is at or below a target rate, and what it misses there.
type Target struct {
	FMR       float64
	Threshold float64
	FNMR      float64
	Reached   bool // false when no grid threshold gets that low: too few impostor pairs to tell
}

// Report is what Run measured.
type Report struct {
	Source              string
	Identities, Faces   int
	GenuinePairs        int
	ImpostorPairs       int
	GenuinePercentiles  map[int]float64 // 1, 5, 50 → score
	ImpostorPercentiles map[int]float64 // 50, 99, 99.9 (as 999) → score
	Rows                []Row
	Targets             []Target
	EqualError, EqualAt float64 // where FMR and FNMR cross
	LowestMeasurableFMR float64 // one impostor pair over all of them: rates below it can't be told apart from 0
}

// ErrTooFew is a sample with too few faces to score both curves.
var ErrTooFew = errors.New("calibration: the sample has too few identities with two faces to measure")

// Run samples the source and measures the curves.
func Run(ctx context.Context, source Source, options Options) (Report, error) {
	o := options.withDefaults()
	faces, err := source.Sample(ctx, o.Identities, o.MaxFaces, o.Seed)
	if err != nil {
		return Report{}, err
	}
	genuine, impostor, identities := score(faces, o.MaxImpostorPairs, o.Seed)
	if len(genuine) == 0 || len(impostor) == 0 {
		return Report{}, ErrTooFew
	}
	return report(source.Describe(), identities, len(faces), genuine, impostor, o), nil
}

// score pairs the faces: within an identity, genuine; across, impostor (sampled down to max).
func score(faces []Face, maxImpostor int, seed int64) (genuine, impostor []float64, identities int) {
	byIdentity := map[string][]int{}
	for i, f := range faces {
		byIdentity[f.Identity] = append(byIdentity[f.Identity], i)
	}
	for _, members := range byIdentity {
		for a := 0; a < len(members); a++ {
			for b := a + 1; b < len(members); b++ {
				genuine = append(genuine, cosine(faces[members[a]].Vector, faces[members[b]].Vector))
			}
		}
	}
	n := len(faces)
	all := n * (n - 1) / 2
	if all-len(genuine) <= maxImpostor {
		for a := 0; a < n; a++ {
			for b := a + 1; b < n; b++ {
				if faces[a].Identity != faces[b].Identity {
					impostor = append(impostor, cosine(faces[a].Vector, faces[b].Vector))
				}
			}
		}
		return genuine, impostor, len(byIdentity)
	}
	random := rand.New(rand.NewSource(seed))
	for len(impostor) < maxImpostor {
		a, b := random.Intn(n), random.Intn(n)
		if a == b || faces[a].Identity == faces[b].Identity {
			continue
		}
		impostor = append(impostor, cosine(faces[a].Vector, faces[b].Vector))
	}
	return genuine, impostor, len(byIdentity)
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

func report(source string, identities, faces int, genuine, impostor []float64, o Options) Report {
	sort.Float64s(genuine)
	sort.Float64s(impostor)
	r := Report{
		Source: source, Identities: identities, Faces: faces, GenuinePairs: len(genuine), ImpostorPairs: len(impostor),
		GenuinePercentiles:  map[int]float64{1: percentile(genuine, 0.01), 5: percentile(genuine, 0.05), 50: percentile(genuine, 0.5)},
		ImpostorPercentiles: map[int]float64{50: percentile(impostor, 0.5), 99: percentile(impostor, 0.99), 999: percentile(impostor, 0.999)},
		LowestMeasurableFMR: 1 / float64(len(impostor)),
	}
	at := func(t float64) Row {
		below := sort.SearchFloat64s(genuine, t) // genuine < t
		atOrAbove := len(impostor) - sort.SearchFloat64s(impostor, t)
		return Row{Threshold: t, FMR: float64(atOrAbove) / float64(len(impostor)), FNMR: float64(below) / float64(len(genuine))}
	}
	steps := int(math.Round(1 / o.Step))
	for i := 0; i <= steps; i++ {
		r.Rows = append(r.Rows, at(math.Round(float64(i)*o.Step*1e6)/1e6))
	}
	for _, target := range o.FMRTargets {
		t := Target{FMR: target}
		for _, row := range r.Rows {
			if row.FMR <= target {
				t.Threshold, t.FNMR, t.Reached = row.Threshold, row.FNMR, target >= r.LowestMeasurableFMR
				break
			}
		}
		r.Targets = append(r.Targets, t)
	}
	best := math.Inf(1)
	for _, row := range r.Rows {
		if gap := math.Abs(row.FMR - row.FNMR); gap < best {
			best, r.EqualAt, r.EqualError = gap, row.Threshold, (row.FMR+row.FNMR)/2
		}
	}
	return r
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Round(q * float64(len(sorted)-1)))
	return sorted[i]
}

// String renders a report as text, with the threshold version to set.
func (r Report) String() string {
	out := fmt.Sprintf("source: %s\nsample: %d identities, %d faces; %d genuine pairs, %d impostor pairs\n",
		r.Source, r.Identities, r.Faces, r.GenuinePairs, r.ImpostorPairs)
	out += fmt.Sprintf("genuine scores:  p1 %.3f  p5 %.3f  median %.3f\n", r.GenuinePercentiles[1], r.GenuinePercentiles[5], r.GenuinePercentiles[50])
	out += fmt.Sprintf("impostor scores: median %.3f  p99 %.3f  p99.9 %.3f\n", r.ImpostorPercentiles[50], r.ImpostorPercentiles[99], r.ImpostorPercentiles[999])
	out += fmt.Sprintf("equal error rate %.4f at %.2f; FMR below %.1e can't be measured with this many impostor pairs\n\n", r.EqualError, r.EqualAt, r.LowestMeasurableFMR)
	out += "threshold      FMR      FNMR\n"
	for _, row := range r.Rows {
		if row.FMR < 1 && row.FNMR > 0 || row.FMR > 0 && row.FNMR < 1 {
			out += fmt.Sprintf("  %.2f   %.2e   %.4f\n", row.Threshold, row.FMR, row.FNMR)
		}
	}
	out += "\nlowest threshold at each false match rate:\n"
	for _, t := range r.Targets {
		if !t.Reached {
			out += fmt.Sprintf("  FMR %.0e: not measurable with this sample (raise -identities)\n", t.FMR)
			continue
		}
		out += fmt.Sprintf("  FMR %.0e: %.2f, missing %.2f%% of genuine pairs\n", t.FMR, t.Threshold, t.FNMR*100)
	}
	return out
}
