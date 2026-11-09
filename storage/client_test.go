package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDownloadFile: a file:// reference is read inside an allowed root and refused outside it,
// including through a symlink or a ".." that leads out.
func TestDownloadFile(t *testing.T) {
	ctx := context.Background()
	root, outside := t.TempDir(), t.TempDir()
	inside := filepath.Join(root, "a", "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("in"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("out"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}

	client := NewFileClient([]string{root})
	if got, err := client.Download(ctx, "file://"+inside); err != nil || string(got) != "in" {
		t.Fatalf("inside root = %q, %v", got, err)
	}
	for _, ref := range []string{
		"file://" + secret,
		"file://" + link,
		"file://" + filepath.Join(root, "..", filepath.Base(outside), "secret"),
		"file://relative/path",
		"trackid/some-key",
	} {
		if _, err := client.Download(ctx, ref); err == nil {
			t.Errorf("%s was read", ref)
		}
	}
	if _, err := NewFileClient(nil).Download(ctx, "file://"+inside); err == nil {
		t.Error("read with no roots allowed")
	}
}
