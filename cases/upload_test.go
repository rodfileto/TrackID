package cases

import (
	"errors"
	"testing"
)

func TestEvidenceFile(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0 rest of a jpeg")
	key, f, err := evidenceFile("C-1", jpeg, "frame.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if key != evidenceObjectKey("C-1", f.HashID) || len(f.HashID) != 64 {
		t.Fatalf("key %q, hash %q", key, f.HashID)
	}
	if f.ContentType != "image/jpeg" || f.SizeBytes != int64(len(jpeg)) || f.Filename != "frame.jpg" || f.StorageRef != "" {
		t.Fatalf("file input %+v", f)
	}
	var verr ValidationError
	if _, _, err := evidenceFile("C-1", []byte("plain text"), ""); !errors.As(err, &verr) {
		t.Fatalf("text accepted as evidence: %v", err)
	}
	if _, _, err := evidenceFile("C-1", nil, ""); !errors.As(err, &verr) {
		t.Fatalf("empty file: %v", err)
	}
}
