package examination

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/rodfileto/trackid/analyses"
)

// TestOrdered: a pair keys the same whichever side it is given from, in byte order (as
// biometric_decisions' COLLATE "C" check), where "TRACE:1#feature" < "TRACE:10#feature".
func TestOrdered(t *testing.T) {
	for _, c := range []struct{ a, b, wantA, wantB string }{
		{"TRACE:10#feature", "TRACE:1#feature", "TRACE:1#feature", "TRACE:10#feature"},
		{"TRACE:1#feature", "TRACE:10#feature", "TRACE:1#feature", "TRACE:10#feature"},
		{"TRACE:5#feature", "101", "101", "TRACE:5#feature"},
	} {
		if got := ordered(c.a, c.b); got != (Pair{c.wantA, c.wantB}) {
			t.Errorf("ordered(%q, %q) = %+v", c.a, c.b, got)
		}
	}
}

// TestDisputed: a pair is disputed only once both of the first two examiners decided it, and
// differently; INCONCLUSIVE against POSITIVE is a disagreement, and the third examiner's decision
// does not settle it away.
func TestDisputed(t *testing.T) {
	d := func(v, r, i string) map[analyses.Step]Decision {
		m := map[analyses.Step]Decision{}
		for step, decision := range map[analyses.Step]string{analyses.StepVerificator: v, analyses.StepReviewer: r, analyses.StepInconsistence: i} {
			if decision != "" {
				m[step] = Decision{Step: step, Decision: decision}
			}
		}
		return m
	}
	for _, c := range []struct {
		name string
		in   map[analyses.Step]Decision
		want bool
	}{
		{"nothing decided", nil, false},
		{"first examiner only", d(Positive, "", ""), false},
		{"agreement", d(Negative, Negative, ""), false},
		{"disagreement", d(Positive, Negative, ""), true},
		{"inconclusive against positive", d(Inconclusive, Positive, ""), true},
		{"settled by the third examiner", d(Positive, Negative, Positive), true},
	} {
		if got := disputed(c.in); got != c.want {
			t.Errorf("%s: disputed = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestStepScope: the first two steps decide every pair; the third only the disputed ones, in the
// analysis's order.
func TestStepScope(t *testing.T) {
	p := func(b string) candidate { return candidate{pair: Pair{"1", b}} }
	all := []candidate{p("a"), p("b"), p("c"), p("d")}
	decided := map[Pair]map[analyses.Step]Decision{
		{"1", "a"}: {analyses.StepVerificator: {Decision: Positive}, analyses.StepReviewer: {Decision: Negative}},
		{"1", "b"}: {analyses.StepVerificator: {Decision: Positive}, analyses.StepReviewer: {Decision: Positive}},
		{"1", "d"}: {analyses.StepVerificator: {Decision: Inconclusive}, analyses.StepReviewer: {Decision: Negative}},
	}
	for _, step := range []analyses.Step{analyses.StepVerificator, analyses.StepReviewer} {
		if got := stepScope(all, step, decided); len(got) != 4 {
			t.Errorf("%s scope = %+v, want every pair", step, got)
		}
	}
	got := stepScope(all, analyses.StepInconsistence, decided)
	if len(got) != 2 || got[0].pair.FeatureB != "a" || got[1].pair.FeatureB != "d" {
		t.Errorf("INCONSISTENCE scope = %+v, want the disputed pairs a, d", got)
	}
}

// TestCropPNG: a box is cut from a PNG or JPEG frame and returned as PNG, clipped to the frame; a
// box off the frame is no image; data that is not an image is an error.
func TestCropPNG(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 10, 8))
	red := color.RGBA{255, 0, 0, 255}
	frame.Set(3, 2, red)
	var pngData, jpegData bytes.Buffer
	if err := png.Encode(&pngData, frame); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegData, frame, nil); err != nil {
		t.Fatal(err)
	}
	decode := func(data []byte) image.Image {
		t.Helper()
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("the crop is not a PNG: %v", err)
		}
		return img
	}

	crop, err := cropPNG(pngData.Bytes(), image.Rect(3, 2, 6, 5))
	if err != nil {
		t.Fatal(err)
	}
	img := decode(crop)
	if img.Bounds() != image.Rect(0, 0, 3, 3) || color.RGBAModel.Convert(img.At(0, 0)) != red {
		t.Errorf("crop = %v with %v at its corner; want 3x3 starting at the red pixel", img.Bounds(), img.At(0, 0))
	}

	if crop, err = cropPNG(pngData.Bytes(), image.Rect(8, 6, 20, 20)); err != nil {
		t.Fatal(err)
	}
	if b := decode(crop).Bounds(); b != image.Rect(0, 0, 2, 2) {
		t.Errorf("a box past the edge cropped to %v, want 2x2", b)
	}

	if crop, err = cropPNG(jpegData.Bytes(), image.Rect(0, 0, 4, 4)); err != nil {
		t.Fatal(err)
	}
	if b := decode(crop).Bounds(); b != image.Rect(0, 0, 4, 4) {
		t.Errorf("a JPEG frame cropped to %v, want 4x4", b)
	}

	if crop, err = cropPNG(pngData.Bytes(), image.Rect(20, 20, 30, 30)); err != nil || crop != nil {
		t.Errorf("a box off the frame = %d bytes, %v; want nil, nil", len(crop), err)
	}
	if _, err := cropPNG([]byte("not an image"), image.Rect(0, 0, 1, 1)); err == nil {
		t.Error("cropping data that is not an image: no error")
	}
}
