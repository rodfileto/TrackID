package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSecret(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveFilesReadsTheValueWithoutTheTrailingNewline(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_SECRET_FILE", writeSecret(t, "s3cr3t\n"))
	if err := ResolveFiles(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("JWT_SECRET"); got != "s3cr3t" {
		t.Fatalf("JWT_SECRET = %q", got)
	}
}

func TestResolveFilesRefusesBothForms(t *testing.T) {
	t.Setenv("JWT_SECRET", "inline")
	t.Setenv("JWT_SECRET_FILE", writeSecret(t, "file"))
	if err := ResolveFiles(); err == nil || !strings.Contains(err.Error(), "both JWT_SECRET and JWT_SECRET_FILE") {
		t.Fatalf("expected an error naming both, got %v", err)
	}
}

func TestResolveFilesRefusesMissingAndEmptyFiles(t *testing.T) {
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("S3_SECRET_KEY_FILE", filepath.Join(t.TempDir(), "absent"))
	if err := ResolveFiles(); err == nil {
		t.Fatal("expected an error for a missing file")
	}
	t.Setenv("S3_SECRET_KEY_FILE", writeSecret(t, "\n"))
	if err := ResolveFiles(); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected an error for an empty file, got %v", err)
	}
}

func TestResolveFilesLeavesOtherFileVariablesAlone(t *testing.T) {
	t.Setenv("IMPORT_REPORT", "")
	t.Setenv("IMPORT_REPORT_FILE", writeSecret(t, "not a secret"))
	if err := ResolveFiles(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("IMPORT_REPORT"); got != "" {
		t.Fatalf("a variable outside Secrets was set: %q", got)
	}
}
