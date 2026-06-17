package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDotfileDriftCheck_reportsDriftedWithoutChangingTarget(t *testing.T) {
	for _, applyMode := range []string{"copy", "symlink"} {
		t.Run(applyMode, func(t *testing.T) {
			homeDir := t.TempDir()
			resource := DesiredResource{
				ID:             "resource_dot_drift_" + applyMode,
				Kind:           "dotfile",
				DesiredVersion: 2,
				Spec:           dotfileResourceSpec("~/.config/neul/drift.conf", applyMode),
			}
			first := ApplyDotfile(context.Background(), homeDir, resource)
			requireDotfileEvent(t, first, "in_sync", "dotfile_applied", 2)

			targetPath := filepath.Join(homeDir, ".config", "neul", "drift.conf")
			if err := os.WriteFile(targetPath, []byte("manual drift\n"), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			event := EvaluateResourceWithHome(context.Background(), nil, homeDir, resource)

			requireDotfileEvent(t, event, "drifted", "dotfile_drifted", 0)
			body, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if string(body) != "manual drift\n" {
				t.Fatalf("content = %q, want unchanged drifted content", string(body))
			}
		})
	}
}
