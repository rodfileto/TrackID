package storagecheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rodfileto/trackid/storage"
)

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// The checks themselves, against real files through storage's file client.
func TestCheck(t *testing.T) {
	root := t.TempDir()
	good, other := []byte("evidence"), []byte("something else")
	path := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	store := storage.NewFileClient([]string{root})
	ctx := context.Background()
	ref := "file://" + path

	cases := []struct {
		name     string
		r        reference
		checked  bool
		verified bool
		kind     string
	}{
		{"hash matches", reference{ref: ref, sha256: sha(good), size: int64(len(good))}, true, true, ""},
		{"hash differs", reference{ref: ref, sha256: sha(other)}, true, false, HashMismatch},
		{"no hash, size matches", reference{ref: ref, size: int64(len(good))}, true, false, ""},
		{"no hash, size differs", reference{ref: ref, size: 1}, true, false, SizeMismatch},
		{"missing file", reference{ref: "file://" + filepath.Join(root, "gone.jpg")}, true, false, Missing},
		{"outside the roots", reference{ref: "file:///etc/hostname"}, true, false, ReadError},
		{"a placeholder", reference{ref: "sim://faces/1.jpg"}, false, false, ""},
	}
	for _, c := range cases {
		checked, verified, problem := check(ctx, store, c.r)
		kind := ""
		if problem != nil {
			kind = problem.Kind
		}
		if checked != c.checked || verified != c.verified || kind != c.kind {
			t.Errorf("%s: got checked=%v verified=%v kind=%q", c.name, checked, verified, kind)
		}
	}
}

// fakeStore answers from a map of reference to content.
type fakeStore map[string][]byte

func (f fakeStore) Digest(_ context.Context, ref string) (string, int64, error) {
	b, ok := f[ref]
	if !ok {
		return "", 0, fmt.Errorf("%w: %s", storage.ErrNotFound, ref)
	}
	return sha(b), int64(len(b)), nil
}

func TestRunOverTheDatabase(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	var caseRow int64
	if err := sqlDB.QueryRowContext(ctx, `INSERT INTO biometric_cases (case_id, case_type, modality, description)
		VALUES ('T-1', 'CRIMINAL', 'FACIAL', 'SIMULATED') RETURNING id`).Scan(&caseRow); err != nil {
		t.Fatal(err)
	}
	good, changed := []byte("good"), []byte("changed")
	store := fakeStore{"b/good": good, "b/changed": changed}
	for i, row := range []struct{ ref, hash string }{
		{"b/good", sha(good)},
		{"b/changed", sha([]byte("as received"))}, // stored otherwise than recorded
		{"b/gone", sha([]byte("gone"))},
		{"sim://placeholder", ""},
	} {
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO case_files (biometric_case_id, category, hash_id, storage_ref, filename)
			VALUES ($1, 'evidence', $2, $3, $4)`, caseRow, row.hash, row.ref, fmt.Sprintf("f%d", i)); err != nil {
			t.Fatal(err)
		}
	}

	report, err := Run(ctx, sqlDB, store, Options{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if report.Checked != 3 || report.Verified != 1 || report.Skipped != 1 || len(report.Problems) != 2 || report.OK() {
		t.Fatalf("unexpected report %+v", report)
	}
	kinds := map[string]bool{}
	for _, p := range report.Problems {
		kinds[p.Kind] = true
	}
	if !kinds[HashMismatch] || !kinds[Missing] {
		t.Fatalf("expected a mismatch and a missing file, got %+v", report.Problems)
	}

	if limited, err := Run(ctx, sqlDB, store, Options{Limit: 1}); err != nil || limited.Checked+limited.Skipped != 1 {
		t.Fatalf("Limit 1: %+v %v", limited, err)
	}
}
