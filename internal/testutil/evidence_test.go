package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteEvidence_whenParentDirectoryIsMissing_writesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "artifact.txt")

	if err := WriteEvidence(path, "ok"); err != nil {
		t.Fatalf("WriteEvidence() error = %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("ReadFile() = %q, want %q", string(body), "ok")
	}
}
