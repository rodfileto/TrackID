package identity

import (
	"strings"
	"testing"
)

func TestEnrollmentFile(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n rest of a png")
	key, f, err := enrollmentFile("R-1", "photo", 2, png)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "identity/R-1/photo/") || len(key) != len("identity/R-1/photo/")+64 {
		t.Fatalf("key %q", key)
	}
	if f.FileType != "photo" || f.Sequence != 2 || f.ContentType != "image/png" || f.SizeBytes != int64(len(png)) || f.StorageRef != "" {
		t.Fatalf("file input %+v", f)
	}
	again, _, _ := enrollmentFile("R-1", "photo", 2, png)
	if again != key {
		t.Fatalf("same bytes, different key: %q vs %q", again, key)
	}
	if _, f, _ := enrollmentFile("R-1", "nist", 1, []byte{0x31, 0x2e, 0x30, 0x30, 0x31}); f.ContentType == "" {
		t.Fatal("no content type for a nist file")
	}
	if _, _, err := enrollmentFile("R-1", "photo", 1, nil); err == nil {
		t.Fatal("empty file accepted")
	}
	if _, _, err := enrollmentFile("", "photo", 1, png); err == nil {
		t.Fatal("missing register accepted")
	}
}
