package imageproc

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"testing"
	"time"
)

// ridges is a w x h image of straight ridges with the given period in pixels, at angle degrees,
// dark on light when dark is true.
func ridges(w, h int, period, angle float64, dark bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	th := angle * math.Pi / 180
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 0.5 + 0.4*math.Cos(2*math.Pi*(float64(x)*math.Cos(th)+float64(y)*math.Sin(th))/period)
			if dark {
				v = 1 - v
			}
			g := uint8(v * 255)
			img.Set(x, y, color.RGBA{g / 2, g, g / 3, 255})
		}
	}
	return img
}

func original(t *testing.T, img image.Image) Original {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	o, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func render(t *testing.T, o Original, r Recipe) Result {
	t.Helper()
	if err := r.Normalize(o.Width, o.Height); err != nil {
		t.Fatal(err)
	}
	res, err := Render(o, r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestEstimateRidgePeriod(t *testing.T) {
	for _, period := range []float64{8, 14.5, 27} {
		o := original(t, ridges(512, 400, period, 30, true))
		got, confidence := EstimateRidgePeriod(o, nil, ChannelGreen)
		if math.Abs(got-period)/period > 0.05 || confidence < 2 {
			t.Errorf("period %v: estimated %.2f (confidence %.1f)", period, got, confidence)
		}
	}
}

// A finger photo at about 1000 ppi (ridge period 18.9 px for 0.48 mm) estimated and resampled
// to 500 ppi has its ridges at about 9.4 px.
func TestEstimatedSourceResamplesToOutput(t *testing.T) {
	o := original(t, ridges(600, 600, 18.9, 75, false))
	period, _ := EstimateRidgePeriod(o, &Rect{100, 100, 500, 500}, ChannelGreen)
	r := Recipe{Source: Source{Method: PPIEstimated, RidgePeriodPX: period, RidgeDistanceMM: 0.48}, Crop: &Rect{100, 100, 500, 500}}
	res := render(t, o, r)
	if abs(float64(res.Width-200)) > 4 || res.Width != res.Height {
		t.Fatalf("size %dx%d, want about 200x200 (half of the 400 px crop)", res.Width, res.Height)
	}
	pl := &plane{res.Width, res.Height, make([]float32, res.Width*res.Height)}
	for i, v := range res.Image.Pix {
		pl.p[i] = float32(v)
	}
	if got, _ := ridgePeriod(pl, 4, 30); math.Abs(got-9.45) > 0.6 {
		t.Errorf("ridge period after resampling %.2f, want about 9.45", got)
	}
	if !strings.Contains(Describe(Record{Original: o, Recipe: r, Result: res}), "Estimada pelo período médio") {
		t.Error("the record does not say the resolution was estimated")
	}
}

func TestCalibration(t *testing.T) {
	r := Recipe{Source: Source{Method: PPICalibrated, Calibration: &Calibration{X1: 10, Y1: 10, X2: 410, Y2: 10, MM: 10}}}
	if err := r.Normalize(500, 500); err != nil {
		t.Fatal(err)
	}
	if math.Abs(r.Source.PPI-1016) > 0.01 {
		t.Errorf("ppi %.2f, want 1016 (400 px over 10 mm)", r.Source.PPI)
	}
	bad := Recipe{Source: Source{Method: PPICalibrated, Calibration: &Calibration{X2: 5, MM: 10}}}
	if err := bad.Normalize(500, 500); err == nil {
		t.Error("accepted a 5 pixel calibration line")
	}
}

func TestRenderIsReproducible(t *testing.T) {
	o := original(t, ridges(300, 260, 12, 10, false))
	r := FingerPhoto(Source{Method: PPIFile, PPI: 700}, &Rect{20, 30, 280, 240})
	r.Rotate = 12.5
	r.Levels = &Levels{InBlack: 20, InWhite: 230, Gamma: 1.2, OutWhite: 255}
	a, b := render(t, o, r), render(t, o, r)
	if a.SHA256 != b.SHA256 {
		t.Error("two renders of the same recipe differ")
	}
	if ppi := pngPPI(a.PNG); math.Abs(ppi-OutputPPI) > 0.1 {
		t.Errorf("the PNG states %.1f ppi, want %d", ppi, OutputPPI)
	}
	decoded, err := png.Decode(bytes.NewReader(a.PNG))
	if err != nil || decoded.Bounds().Dx() != a.Width {
		t.Fatalf("the PNG does not decode to the render: %v", err)
	}
	text := Describe(Record{Filename: "dedo.jpg", Original: o, Recipe: r, Result: a, Examiner: "perito", RenderedAt: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)})
	for _, want := range []string{"canal verde", "Rotação de 12,5° no sentido horário", "Espelhamento horizontal", "Reamostragem de 700 ppi para 500 ppi",
		"Níveis: entrada 20 a 230, gama 1,20", "CLAHE", "Gabor", "Inversão de tons", a.SHA256, o.SHA256, "única transformação geométrica", "dedo.jpg"} {
		if !strings.Contains(text, want) {
			t.Errorf("the record lacks %q", want)
		}
	}
}

// The transform maps original pixels onto the render: a dark dot ends where Transform says,
// mirrored and rotated.
func TestTransformFollowsThePixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 18; y < 22; y++ {
		for x := 38; x < 42; x++ {
			img.Set(x, y, color.Black)
		}
	}
	o := original(t, img)
	r := Recipe{Source: Source{Method: PPIFile, PPI: 500}, Rotate: 90, Mirror: true, Interpolation: InterpolationNearest}
	res := render(t, o, r)
	if res.Width != 100 || res.Height != 200 {
		t.Fatalf("size %dx%d, want 100x200 after 90°", res.Width, res.Height)
	}
	x, y := res.Transform.Apply(40, 20)
	if v := res.Image.GrayAt(int(x), int(y)).Y; v > 50 {
		t.Errorf("the dot maps to (%.1f, %.1f), where the render is %d, not dark", x, y, v)
	}
	bx, by := res.Transform.Invert().Apply(x, y)
	if math.Abs(bx-40) > 1e-9 || math.Abs(by-20) > 1e-9 {
		t.Errorf("round trip gives (%v, %v)", bx, by)
	}
}

func TestLevelsAndInvert(t *testing.T) {
	src := &plane{3, 1, []float32{20, 125, 230}}
	out := levels(src, Levels{InBlack: 20, InWhite: 230, Gamma: 1, OutBlack: 0, OutWhite: 255})
	if out.p[0] != 0 || out.p[2] != 255 || math.Abs(float64(out.p[1])-127.5) > 0.01 {
		t.Errorf("levels %v", out.p)
	}
	if inv := invert(out); inv.p[0] != 255 || inv.p[2] != 0 {
		t.Errorf("invert %v", inv.p)
	}
	bright := levels(src, Levels{InBlack: 20, InWhite: 230, Gamma: 2, OutWhite: 255})
	if bright.p[1] <= out.p[1] {
		t.Error("a gamma above 1 does not brighten the midtones")
	}
}

// Gabor keeps the ridge pattern and its polarity, and fills a flat background with paper.
func TestGaborEnhance(t *testing.T) {
	img := ridges(200, 200, 9, 40, true)
	for y := 0; y < 200; y++ {
		for x := 0; x < 60; x++ {
			img.Set(x, y, color.RGBA{200, 200, 200, 255}) // a flat band, no ridges
		}
	}
	o := original(t, img)
	res := render(t, o, Recipe{Source: Source{Method: PPIFile, PPI: 500}, Gabor: &Gabor{}})
	if math.Abs(res.GaborPeriodPX-9) > 0.7 || !res.GaborEstimated {
		t.Errorf("Gabor period %.2f (estimated %v), want about 9", res.GaborPeriodPX, res.GaborEstimated)
	}
	if v := res.Image.GrayAt(20, 100).Y; v != 255 {
		t.Errorf("the flat band is %d, want paper (255)", v)
	}
	// The source is dark where cos(...) peaks; the enhanced image must be dark there too.
	if dark, light := res.Image.GrayAt(150, 100).Y, res.Image.GrayAt(150+3, 100).Y; img.RGBAAt(150, 100).G < img.RGBAAt(153, 100).G && dark > light {
		t.Errorf("polarity flipped: %d at a dark ridge, %d beside it", dark, light)
	}
}

func TestOrient(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.Black) // top-left marker
	cases := map[int]image.Point{6: {1, 0}, 8: {0, 2}, 3: {2, 1}, 2: {2, 0}}
	for o, want := range cases {
		out := orient(img, o)
		if _, _, _, a := out.At(want.X, want.Y).RGBA(); a == 0 || out.At(want.X, want.Y) != (color.NRGBA{0, 0, 0, 255}) {
			t.Errorf("orientation %d: marker not at %v", o, want)
		}
	}
	if b := orient(img, 6).Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Errorf("orientation 6 keeps size %v", b)
	}
}

func TestNormalizeRejects(t *testing.T) {
	for name, r := range map[string]Recipe{
		"channel":  {Channel: "infrared"},
		"levels":   {Levels: &Levels{InBlack: 200, InWhite: 100, Gamma: 1, OutWhite: 255}},
		"gamma":    {Levels: &Levels{InWhite: 255, Gamma: 0, OutWhite: 255}},
		"crop":     {Crop: &Rect{10, 10, 12, 12}},
		"version":  {Version: 9},
		"estimate": {Source: Source{Method: PPIEstimated}},
	} {
		if err := r.Normalize(100, 100); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
