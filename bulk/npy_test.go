package bulk

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// TestHalf: float32 -> float16 -> float32 is exact on values float16 holds, rounds to nearest
// even otherwise, and keeps subnormals, zeros and infinities.
func TestHalf(t *testing.T) {
	for _, c := range []struct {
		in   float32
		bits uint16
	}{
		{0, 0}, {float32(math.Copysign(0, -1)), 0x8000}, {1, 0x3c00}, {-2, 0xc000}, {0.5, 0x3800},
		{65504, 0x7bff}, {1e6, 0x7c00}, {float32(math.Pow(2, -24)), 0x0001}, {float32(math.Pow(2, -14)), 0x0400},
		{1 + 1.0/2048, 0x3c00}, // halfway, rounds to even (down)
		{1 + 3.0/2048, 0x3c02}, // halfway, rounds to even (up)
		{0.1, 0x2e66},          // nearest
		{float32(math.Inf(1)), 0x7c00},
	} {
		if got := float32ToHalf(c.in); got != c.bits {
			t.Errorf("float32ToHalf(%v) = %#04x, want %#04x", c.in, got, c.bits)
		}
	}
	for bits := uint16(0); bits < 0x7c00; bits++ {
		if back := float32ToHalf(halfToFloat32(bits)); back != bits {
			t.Fatalf("%#04x -> %v -> %#04x", bits, halfToFloat32(bits), back)
		}
	}
}

// TestNpyRoundTrip: what Go writes, Go reads back, in every dtype used.
func TestNpyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ids := []int64{1, -2, 1 << 40}
	if err := WriteInt64s(filepath.Join(dir, "ids.npy"), ids); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInt64s(filepath.Join(dir, "ids.npy"))
	if err != nil || len(got) != 3 || got[2] != 1<<40 || got[1] != -2 {
		t.Fatalf("ids = %v, %v", got, err)
	}
	rows := [][]float32{make([]float32, Dim), make([]float32, Dim)}
	rows[0][0], rows[1][Dim-1] = 0.25, -0.125
	for _, half := range []bool{true, false} {
		path := filepath.Join(dir, "m.npy")
		if err := WriteMatrix(path, rows, Dim, half); err != nil {
			t.Fatal(err)
		}
		m, err := ReadMatrix(path)
		if err != nil || len(m) != 2 || m[0][0] != 0.25 || m[1][Dim-1] != -0.125 {
			t.Fatalf("half %v: %v", half, err)
		}
	}
	if err := WriteMatrix(filepath.Join(dir, "bad.npy"), [][]float32{{1}}, Dim, true); err == nil {
		t.Fatal("short row accepted")
	}
	empty := filepath.Join(dir, "empty.npy")
	if err := WriteFloat32s(empty, nil); err != nil {
		t.Fatal(err)
	}
	if f, err := ReadFloat32s(empty); err != nil || len(f) != 0 {
		t.Fatalf("empty = %v, %v", f, err)
	}
}

// TestManifest: lines decode and validate, a bad line is reported with its number, and
// startLine skips.
func TestManifest(t *testing.T) {
	text := `{"personId":"p","documentType":"RG","documentNumber":"1","registerNumber":"r1","files":[{"fileType":"photo","uri":"file:///a.jpg"}]}

{"personId":"p","documentType":"RG","documentNumber":"1","registerNumber":"r2","files":[{"fileType":"photo","path":"rel.jpg"}]}
{"personId":"p","documentType":"RG","documentNumber":"1","registerNumber":"r3","files":[{"fileType":"photo","path":"/a.jpg","uri":"file:///a.jpg"}]}
not json
{"personId":"p","documentType":"RG","documentNumber":"1","registerNumber":"r4","files":[{"fileType":"photo","path":"/x/a.jpg"}]}
`
	out := make(chan ManifestLine, 10)
	if err := ReadManifest(strings.NewReader(text), 1, out); err != nil {
		t.Fatal(err)
	}
	var numbers []int
	var bad []int
	for l := range out {
		numbers = append(numbers, l.Number)
		if l.Err != nil {
			bad = append(bad, l.Number)
		}
	}
	if len(numbers) != 5 || len(bad) != 3 || bad[0] != 3 || bad[1] != 4 || bad[2] != 5 {
		t.Fatalf("lines %v, bad %v", numbers, bad)
	}
	out = make(chan ManifestLine, 10)
	if err := ReadManifest(strings.NewReader(text), 6, out); err != nil {
		t.Fatal(err)
	}
	l := <-out
	if l.Number != 6 || l.Err != nil {
		t.Fatalf("from line 6: %+v", l)
	}
	e := l.Enrollment.Enrollment([]string{"trackid/x"})
	if e.Parent1Gender != "U" || e.Files[0].StorageRef != "trackid/x" || e.Files[0].SourcePath != "/x/a.jpg" {
		t.Fatalf("enrollment = %+v", e)
	}
}
