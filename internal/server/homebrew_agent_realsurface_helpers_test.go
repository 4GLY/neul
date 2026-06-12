package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func installRealSurfaceFakeBrewWithScript(t *testing.T, script string) realSurfaceFakeBrew {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.Mkdir(stateDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	brewPath := filepath.Join(dir, "brew")
	if err := os.WriteFile(brewPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(brew) error = %v", err)
	}
	logPath := filepath.Join(dir, "argv.log")
	pathEnv := dir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", pathEnv)
	t.Setenv("BREW_ARGV_LOG", logPath)
	t.Setenv("BREW_FAKE_STATE_DIR", stateDir)
	lookedPath, err := exec.LookPath("brew")
	if err != nil {
		t.Fatalf("LookPath(brew) error = %v", err)
	}
	if lookedPath != brewPath {
		t.Fatalf("LookPath(brew) = %s, want fake brew first at %s", lookedPath, brewPath)
	}
	return realSurfaceFakeBrew{path: brewPath, logPath: logPath, lookedPath: lookedPath, pathEnv: pathEnv}
}
