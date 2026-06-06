package testutil

import (
	"os"
	"path/filepath"
)

func WriteEvidence(path string, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
