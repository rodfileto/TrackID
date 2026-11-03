package biometricmatch

import "testing"

func TestBand(t *testing.T) {
	for _, b := range []Band{{0.6, 0.6}, {0.6, 0.7}, {0.01, 1}} {
		if err := b.Validate(); err != nil {
			t.Errorf("%+v: %v", b, err)
		}
	}
	for _, b := range []Band{{0, 0.7}, {0.7, 0.6}, {-0.1, 0.5}} {
		if err := b.Validate(); err == nil {
			t.Errorf("%+v: accepted", b)
		}
	}
	// A template matcher's scale is unbounded; a similarity's stops at 1.
	if err := (Band{40, 60}).Validate(); err != nil {
		t.Errorf("SourceAFIS band: %v", err)
	}
	if err := (Band{0.6, 0.7}).ValidateSimilarity(); err != nil {
		t.Errorf("similarity band: %v", err)
	}
	for _, b := range []Band{{0.6, 1.1}, {40, 60}, {0, 0.5}} {
		if err := b.ValidateSimilarity(); err == nil {
			t.Errorf("%+v: accepted as a similarity band", b)
		}
	}
	b := Band{Review: 0.6, Confirm: 0.7}
	for _, c := range []struct {
		conf      float64
		decision  string
		threshold float64
	}{{0.95, "POSITIVE", 0.7}, {0.7, "POSITIVE", 0.7}, {0.69, "INCONCLUSIVE", 0.6}, {0.6, "INCONCLUSIVE", 0.6}} {
		if d, th := b.classify(c.conf); d != c.decision || th != c.threshold {
			t.Errorf("classify(%v) = %s %v, want %s %v", c.conf, d, th, c.decision, c.threshold)
		}
	}
}
