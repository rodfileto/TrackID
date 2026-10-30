package imageproc

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"testing"
	"time"
)

// gradient is a colour image whose pixels all differ, with a bright red dot at (dx, dy).
func gradient(w, h, dx, dy int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(40 + x%100), uint8(60 + y%100), uint8(90 + (x+y)%60), 255})
		}
	}
	for y := dy - 2; y <= dy+2; y++ {
		for x := dx - 2; x <= dx+2; x++ {
			img.Set(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	return img
}

func renderFace(t *testing.T, o Original, r FaceRecipe) (FaceRecipe, FaceResult) {
	t.Helper()
	if err := r.Normalize(o.Width, o.Height); err != nil {
		t.Fatal(err)
	}
	res, err := RenderFace(o, r)
	if err != nil {
		t.Fatal(err)
	}
	return r, res
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestRenderFaceIdentityKeepsPixels(t *testing.T) {
	src := gradient(64, 48, 20, 20)
	o := original(t, src)
	r, res := renderFace(t, o, FaceRecipe{})
	if !r.Identity() {
		t.Fatal("an empty recipe should be the identity")
	}
	got := decodePNG(t, res.PNG)
	if got.Bounds().Dx() != 64 || got.Bounds().Dy() != 48 {
		t.Fatalf("size %v", got.Bounds())
	}
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			a, b := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA), src.NRGBAAt(x, y)
			if a != b {
				t.Fatalf("pixel (%d,%d) is %v, want %v", x, y, a, b)
			}
		}
	}
	if !strings.Contains(DescribeFace(FaceRecord{Original: o, Recipe: r, Result: res}), "Nenhum processamento") {
		t.Error("the record of an empty recipe should say nothing was done")
	}
}

func TestRenderFaceIsReproducible(t *testing.T) {
	o := original(t, gradient(80, 60, 30, 30))
	recipe := FaceRecipe{Rotate: 12, Mirror: true, Scale: 1.7, ColorBalance: &ColorBalance{1.1, 1, 0.9},
		BrightnessContrast: &BrightnessContrast{10, 20}, Curves: &Curves{Points: [][2]float64{{64, 50}, {192, 210}}}}
	_, a := renderFace(t, o, recipe)
	_, b := renderFace(t, o, recipe)
	if a.SHA256 != b.SHA256 {
		t.Error("the same recipe gave two different images")
	}
}

// TestRenderFaceTransform checks that the red dot lands where the transform says, through crop,
// rotation, mirror and scale, and that the inverse brings it back.
func TestRenderFaceTransform(t *testing.T) {
	o := original(t, gradient(120, 100, 70, 40))
	_, res := renderFace(t, o, FaceRecipe{Crop: &Rect{10, 5, 110, 95}, Rotate: -25, Mirror: true, Scale: 2})
	img := decodePNG(t, res.PNG)
	wx, wy := res.Transform.Apply(70.5, 40.5)
	r, _, _, _ := img.At(int(wx), int(wy)).RGBA()
	if r>>8 < 200 {
		t.Errorf("the dot is not at the transformed point (%.1f, %.1f): red %d", wx, wy, r>>8)
	}
	ft := FaceTransform(FaceRecipe{Crop: &Rect{10, 5, 110, 95}, Rotate: -25, Mirror: true, Scale: 2}, 120, 100)
	if fx, fy := ft.Apply(70.5, 40.5); math.Abs(fx-wx) > 1e-6 || math.Abs(fy-wy) > 1e-6 {
		t.Errorf("FaceTransform gives (%f, %f), the render (%f, %f)", fx, fy, wx, wy)
	}
	bx, by := res.Transform.Invert().Apply(wx, wy)
	if math.Abs(bx-70.5) > 1e-9 || math.Abs(by-40.5) > 1e-9 {
		t.Errorf("round trip gave (%f, %f)", bx, by)
	}
}

func TestRenderFaceGrey(t *testing.T) {
	o := original(t, gradient(40, 30, 10, 10))
	r, res := renderFace(t, o, FaceRecipe{Grey: ChannelLuminance, Levels: &Levels{InBlack: 10, InWhite: 240, Gamma: 1.2, OutWhite: 255}})
	if _, ok := decodePNG(t, res.PNG).(*image.Gray); !ok || !res.Grey {
		t.Error("a grey recipe should give a grey PNG")
	}
	rec := DescribeFace(FaceRecord{Filename: "q.png", Original: o, Recipe: r, Result: res, RenderedAt: time.Now()})
	for _, want := range []string{"luminância", "Níveis", "sem perdas", "imagem inteira", res.SHA256} {
		if !strings.Contains(rec, want) {
			t.Errorf("record lacks %q:\n%s", want, rec)
		}
	}
}

func TestFaceRecipeRefusesWhatFISWGRulesOut(t *testing.T) {
	for name, r := range map[string]FaceRecipe{
		"scale":     {Scale: 20},
		"channel":   {Grey: "infrared"},
		"gains":     {ColorBalance: &ColorBalance{5, 1, 1}},
		"contrast":  {BrightnessContrast: &BrightnessContrast{0, 100}},
		"curve":     {Curves: &Curves{Points: [][2]float64{{10, 10}, {10.5, 50}}}},
		"too large": {Scale: 8},
	} {
		w, h := 100, 100
		if name == "too large" {
			w, h = 1000, 1000
		}
		if err := r.Normalize(w, h); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMonotoneCurve(t *testing.T) {
	c := monotoneCurve([][2]float64{{64, 40}, {128, 128}, {192, 230}})
	for _, p := range [][2]float64{{0, 0}, {64, 40}, {128, 128}, {192, 230}, {255, 255}} {
		if got := c(p[0]); math.Abs(got-p[1]) > 0.6 {
			t.Errorf("curve(%v) = %.2f, want %v", p[0], got, p[1])
		}
	}
	prev := -1.0
	for v := 0.0; v <= 255; v += 0.5 {
		if got := c(v); got < prev-1e-9 {
			t.Fatalf("curve decreases at %v", v)
		} else {
			prev = got
		}
	}
}

func TestReadExif(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, gradient(16, 16, 8, 8), nil); err != nil {
		t.Fatal(err)
	}
	// A little-endian TIFF: IFD0 with Make, Software and the EXIF pointer; the EXIF IFD with
	// DateTimeOriginal.
	le := binary.LittleEndian
	tiff := []byte("II*\x00\x08\x00\x00\x00")
	ascii := func(s string) []byte { return append([]byte(s), 0) }
	make_, software, date := ascii("Canon"), ascii("Adobe Photoshop 25.0"), ascii("2026:10:01 14:03:22")
	ifd0 := 8
	exif := ifd0 + 2 + 3*12 + 4
	data := exif + 2 + 12 + 4
	entry := func(tag, kind uint16, count, value uint32) []byte {
		e := make([]byte, 12)
		le.PutUint16(e, tag)
		le.PutUint16(e[2:], kind)
		le.PutUint32(e[4:], count)
		le.PutUint32(e[8:], value)
		return e
	}
	tiff = le.AppendUint16(tiff, 3)
	tiff = append(tiff, entry(0x010F, 2, uint32(len(make_)), uint32(data))...)
	tiff = append(tiff, entry(0x0131, 2, uint32(len(software)), uint32(data+len(make_)))...)
	tiff = append(tiff, entry(0x8769, 4, 1, uint32(exif))...)
	tiff = le.AppendUint32(tiff, 0)
	tiff = le.AppendUint16(tiff, 1)
	tiff = append(tiff, entry(0x9003, 2, uint32(len(date)), uint32(data+len(make_)+len(software)))...)
	tiff = le.AppendUint32(tiff, 0)
	tiff = append(append(append(tiff, make_...), software...), date...)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	jpg := append(append(append([]byte{}, buf.Bytes()[:2]...), append(seg, payload...)...), buf.Bytes()[2:]...)

	x := ReadExif(jpg)
	if x.Make != "Canon" || x.Software != "Adobe Photoshop 25.0" || x.DateTimeOriginal != "2026:10:01 14:03:22" {
		t.Errorf("got %+v", x)
	}
	if (ReadExif(buf.Bytes()) != Exif{}) {
		t.Error("a JPEG without EXIF should give nothing")
	}
}
