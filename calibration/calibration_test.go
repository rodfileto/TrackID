package calibration

import (
	"context"
	"math"
	"testing"
)

type fakeSource []Face

func (f fakeSource) Sample(context.Context, int, int, int64) ([]Face, error) { return f, nil }
func (f fakeSource) Describe() string                                        { return "fake" }

// unit is a 2-d vector at an angle: the cosine of two of them is the cosine of their angle's difference.
func unit(degrees float64) []float32 {
	r := degrees * math.Pi / 180
	return []float32{float32(math.Cos(r)), float32(math.Sin(r))}
}

func TestRunSeparatesTheCurves(t *testing.T) {
	// Three people, two faces each, 10 degrees apart (cosine 0.985); the people 60 or more degrees apart (≤ 0.5).
	faces := fakeSource{
		{1, "a", unit(0)}, {2, "a", unit(10)},
		{3, "b", unit(70)}, {4, "b", unit(80)},
		{5, "c", unit(140)}, {6, "c", unit(150)},
	}
	report, err := Run(context.Background(), faces, Options{Step: 0.1, FMRTargets: []float64{0.1}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Identities != 3 || report.GenuinePairs != 3 || report.ImpostorPairs != 12 {
		t.Fatalf("pairs: %+v", report)
	}
	at := func(threshold float64) Row {
		for _, row := range report.Rows {
			if math.Abs(row.Threshold-threshold) < 1e-9 {
				return row
			}
		}
		t.Fatalf("no row at %v", threshold)
		return Row{}
	}
	// At 0.6 every impostor (cosines ≤ 0.5) is below and every genuine pair (0.985) above.
	if row := at(0.6); row.FMR != 0 || row.FNMR != 0 {
		t.Fatalf("at 0.6: %+v", row)
	}
	// At 0.0 the impostors at 60 and 70 degrees (cosines 0.5, 0.34, ...) count as matches.
	if row := at(0.0); row.FMR == 0 || row.FNMR != 0 {
		t.Fatalf("at 0.0: %+v", row)
	}
	if target := report.Targets[0]; !target.Reached || target.Threshold > 0.6 || target.FNMR != 0 {
		t.Fatalf("target: %+v", target)
	}
	if _, err := Run(context.Background(), fakeSource{{1, "a", unit(0)}, {2, "b", unit(90)}}, Options{}); err != ErrTooFew {
		t.Fatalf("no genuine pair: %v", err)
	}
}

func TestParseKey(t *testing.T) {
	if key, err := ParseKey(" person , fiscal "); err != nil || len(key) != 2 || key[0] != "person" {
		t.Fatalf("%v, %v", key, err)
	}
	for _, bad := range []string{"", "name", "person,cpf"} {
		if _, err := ParseKey(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
