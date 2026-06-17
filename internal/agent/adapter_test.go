package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomebrew_fakeAdapterReportsInSyncDriftedAndApplyInSync(t *testing.T) {
	fake := &fakePackageAdapter{installed: map[string]string{"kubectl": "1.31.0"}}

	inSync := CheckPackage(context.Background(), fake, DesiredResource{
		ID: "resource_in_sync", Kind: "package", Spec: map[string]interface{}{"sourceKind": "brew", "name": "kubectl", "desiredVersion": "1.31.0"},
	})
	if inSync.Status != "in_sync" {
		t.Fatalf("inSync status = %s, want in_sync", inSync.Status)
	}

	drifted := CheckPackage(context.Background(), fake, DesiredResource{
		ID: "resource_drifted", Kind: "package", Spec: map[string]interface{}{"sourceKind": "brew", "name": "kubectl", "desiredVersion": "1.32.0"},
	})
	if drifted.Status != "drifted" {
		t.Fatalf("drifted status = %s, want drifted", drifted.Status)
	}

	applied := ApplyPackage(context.Background(), fake, DesiredResource{
		ID: "resource_apply", Kind: "package", Spec: map[string]interface{}{"sourceKind": "brew", "name": "helm", "desiredVersion": "latest"},
	})
	if applied.Status != "in_sync" {
		t.Fatalf("applied status = %s, want in_sync", applied.Status)
	}
}

func TestAgentReport_unsupportedMiseProducesBlockedUnsupportedMessage(t *testing.T) {
	event := EvaluateResource(context.Background(), nil, DesiredResource{
		ID: "resource_mise", Kind: "package", Spec: map[string]interface{}{"sourceKind": "mise", "name": "node", "desiredVersion": "22"},
	})
	if event.Status != "blocked" {
		t.Fatalf("status = %s, want blocked", event.Status)
	}
	if !strings.Contains(event.Message, "mise") || !strings.Contains(event.Message, "unsupported") {
		t.Fatalf("message = %s, want readable unsupported mise source message", event.Message)
	}
}

func TestDotfile_rejectsPathOutsideAllowlist(t *testing.T) {
	event := EvaluateResource(context.Background(), nil, DesiredResource{
		ID: "resource_dot", Kind: "dotfile", Spec: map[string]interface{}{"path": "/etc/hosts"},
	})
	if event.Status != "blocked" || !strings.Contains(event.Message, "path_not_allowed") {
		t.Fatalf("event = %+v, want blocked path_not_allowed", event)
	}
}

func TestDotfileCopy_appliesFile_whenTargetIsAllowlisted(t *testing.T) {
	homeDir := t.TempDir()
	resource := DesiredResource{
		ID:             "resource_dot_zshrc",
		Kind:           "dotfile",
		DesiredVersion: 7,
		Spec: map[string]interface{}{
			"path":          "~/.zshrc",
			"content":       "export NEUL=v7\n",
			"mode":          "0600",
			"applyMode":     "copy",
			"targetSegment": "base",
		},
	}

	event := ApplyDotfile(context.Background(), homeDir, resource)

	requireDotfileEvent(t, event, "in_sync", "dotfile_applied", 7)
	targetPath := filepath.Join(homeDir, ".zshrc")
	body, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "export NEUL=v7\n" {
		t.Fatalf("content = %q, want desired content", string(body))
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestDotfileSymlink_appliesManagedSymlink_whenTargetIsAllowlisted(t *testing.T) {
	homeDir := t.TempDir()
	resource := DesiredResource{
		ID:             "resource_dot_gitconfig",
		Kind:           "dotfile",
		DesiredVersion: 3,
		Spec: map[string]interface{}{
			"path":          "~/.gitconfig",
			"content":       "[user]\n\tname = neul\n",
			"mode":          "0644",
			"applyMode":     "symlink",
			"targetSegment": "base",
		},
	}

	event := ApplyDotfile(context.Background(), homeDir, resource)

	requireDotfileEvent(t, event, "in_sync", "dotfile_applied", 3)
	targetPath := filepath.Join(homeDir, ".gitconfig")
	linkTarget, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if !strings.HasPrefix(linkTarget, filepath.Join(homeDir, ".local", "state", "neul", "dotfiles", "base", "resource_dot_gitconfig")+string(filepath.Separator)) {
		t.Fatalf("link target = %s, want managed state path under home", linkTarget)
	}
	body, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "[user]\n\tname = neul\n" {
		t.Fatalf("content = %q, want managed content", string(body))
	}
}

func TestDotfileSymlink_preservesExistingTempNameFile_whenReplacingLink(t *testing.T) {
	homeDir := t.TempDir()
	targetDir := filepath.Join(homeDir, ".config", "neul")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	sentinelPath := filepath.Join(targetDir, ".neul-dotfile-link")
	if err := os.WriteFile(sentinelPath, []byte("sentinel\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resource := DesiredResource{
		ID:             "resource_dot_nested",
		Kind:           "dotfile",
		DesiredVersion: 4,
		Spec:           dotfileResourceSpec("~/.config/neul/link.conf", "symlink"),
	}

	event := ApplyDotfile(context.Background(), homeDir, resource)

	requireDotfileEvent(t, event, "in_sync", "dotfile_applied", 4)
	linkTarget, err := os.Readlink(filepath.Join(targetDir, "link.conf"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if !strings.HasPrefix(linkTarget, filepath.Join(homeDir, ".local", "state", "neul", "dotfiles", "base", "resource_dot_nested")+string(filepath.Separator)) {
		t.Fatalf("link target = %s, want managed state path under home", linkTarget)
	}
	body, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("ReadFile() sentinel error = %v", err)
	}
	if string(body) != "sentinel\n" {
		t.Fatalf("sentinel content = %q, want unchanged", string(body))
	}
}

func TestDotfileCopy_updatesFile_whenExistingTargetWasManagedByAgent(t *testing.T) {
	homeDir := t.TempDir()
	resource := DesiredResource{
		ID:             "resource_dot_zshrc",
		Kind:           "dotfile",
		DesiredVersion: 1,
		Spec:           dotfileResourceSpec("~/.zshrc", "copy"),
	}
	first := ApplyDotfile(context.Background(), homeDir, resource)
	requireDotfileEvent(t, first, "in_sync", "dotfile_applied", 1)

	resource.DesiredVersion = 2
	resource.Spec["content"] = "desired v2\n"
	second := ApplyDotfile(context.Background(), homeDir, resource)

	requireDotfileEvent(t, second, "in_sync", "dotfile_applied", 2)
	body, err := os.ReadFile(filepath.Join(homeDir, ".zshrc"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "desired v2\n" {
		t.Fatalf("content = %q, want updated managed content", string(body))
	}
}

func TestDotfileSymlink_updatesLink_whenExistingTargetWasManagedByAgent(t *testing.T) {
	homeDir := t.TempDir()
	resource := DesiredResource{
		ID:             "resource_dot_gitconfig",
		Kind:           "dotfile",
		DesiredVersion: 1,
		Spec:           dotfileResourceSpec("~/.gitconfig", "symlink"),
	}
	first := ApplyDotfile(context.Background(), homeDir, resource)
	requireDotfileEvent(t, first, "in_sync", "dotfile_applied", 1)
	firstTarget, err := os.Readlink(filepath.Join(homeDir, ".gitconfig"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}

	resource.DesiredVersion = 2
	resource.Spec["content"] = "desired v2\n"
	second := ApplyDotfile(context.Background(), homeDir, resource)

	requireDotfileEvent(t, second, "in_sync", "dotfile_applied", 2)
	secondTarget, err := os.Readlink(filepath.Join(homeDir, ".gitconfig"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if secondTarget == firstTarget {
		t.Fatalf("link target did not change: %s", secondTarget)
	}
	body, err := os.ReadFile(filepath.Join(homeDir, ".gitconfig"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "desired v2\n" {
		t.Fatalf("content = %q, want updated managed content", string(body))
	}
}

func TestDotfile_reapplySameDesiredState_reportsInSync(t *testing.T) {
	for _, applyMode := range []string{"copy", "symlink"} {
		t.Run(applyMode, func(t *testing.T) {
			homeDir := t.TempDir()
			resource := DesiredResource{
				ID:             "resource_dot_reapply_" + applyMode,
				Kind:           "dotfile",
				DesiredVersion: 6,
				Spec:           dotfileResourceSpec("~/.config/neul/reapply.conf", applyMode),
			}

			first := ApplyDotfile(context.Background(), homeDir, resource)
			second := ApplyDotfile(context.Background(), homeDir, resource)

			requireDotfileEvent(t, first, "in_sync", "dotfile_applied", 6)
			requireDotfileEvent(t, second, "in_sync", "dotfile_applied", 6)
		})
	}
}

func TestDotfile_blocksWhenHomeIsUnavailable(t *testing.T) {
	event := ApplyDotfile(context.Background(), "", DesiredResource{
		ID:             "resource_dot_home",
		Kind:           "dotfile",
		DesiredVersion: 1,
		Spec:           dotfileResourceSpec("~/.zshrc", "copy"),
	})

	requireDotfileEvent(t, event, "blocked", "home_unavailable", 0)
}

func TestDotfile_blocksUnsafeResourceID_withoutWritingManagedPath(t *testing.T) {
	homeDir := t.TempDir()

	event := ApplyDotfile(context.Background(), homeDir, DesiredResource{
		ID:             "../escape",
		Kind:           "dotfile",
		DesiredVersion: 1,
		Spec:           dotfileResourceSpec("~/.zshrc", "symlink"),
	})

	requireDotfileEvent(t, event, "blocked", "invalid_spec", 0)
	if _, err := os.Stat(filepath.Join(homeDir, ".local", "state", "neul", "dotfiles")); !os.IsNotExist(err) {
		t.Fatalf("managed state stat error = %v, want absent", err)
	}
}

func TestDotfileSymlink_blocksUnmanagedSymlinkConflict_withoutWritingManagedPath(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.Symlink(filepath.Join(homeDir, "other"), filepath.Join(homeDir, ".gitconfig")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	event := ApplyDotfile(context.Background(), homeDir, DesiredResource{
		ID:             "resource_dot_gitconfig",
		Kind:           "dotfile",
		DesiredVersion: 5,
		Spec:           dotfileResourceSpec("~/.gitconfig", "symlink"),
	})

	requireDotfileEvent(t, event, "blocked", "conflict_existing_symlink", 0)
	if _, err := os.Stat(filepath.Join(homeDir, ".local", "state", "neul", "dotfiles", "base", "resource_dot_gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("managed resource dir stat error = %v, want absent", err)
	}
}

func TestDotfile_blocksInvalidInputs_withoutWriting(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]interface{}
		message string
	}{
		{
			name:    "absolute path",
			spec:    dotfileResourceSpec("/etc/hosts", "copy"),
			message: "path_not_allowed",
		},
		{
			name:    "usr path",
			spec:    dotfileResourceSpec("/usr/local/neul.conf", "copy"),
			message: "path_not_allowed",
		},
		{
			name:    "traversal path",
			spec:    dotfileResourceSpec("~/.config/../.ssh/config", "copy"),
			message: "path_traversal",
		},
		{
			name:    "nil spec",
			spec:    nil,
			message: "invalid_spec",
		},
		{
			name:    "empty spec",
			spec:    map[string]interface{}{},
			message: "invalid_spec",
		},
		{
			name:    "unknown apply mode",
			spec:    dotfileResourceSpec("~/.zshrc", "template"),
			message: "invalid_spec",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			outsideDir := t.TempDir()
			outsideMarker := filepath.Join(outsideDir, "hosts")

			event := ApplyDotfile(context.Background(), homeDir, DesiredResource{
				ID:             "resource_dot",
				Kind:           "dotfile",
				DesiredVersion: 9,
				Spec:           tt.spec,
			})

			requireDotfileEvent(t, event, "blocked", tt.message, 0)
			if _, err := os.Stat(filepath.Join(homeDir, ".zshrc")); !os.IsNotExist(err) {
				t.Fatalf(".zshrc stat error = %v, want absent", err)
			}
			if _, err := os.Stat(outsideMarker); !os.IsNotExist(err) {
				t.Fatalf("outside marker stat error = %v, want absent", err)
			}
		})
	}
}

func TestDotfile_blocksSymlinkEscape_withoutWritingOutsideHome(t *testing.T) {
	homeDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(homeDir, ".config"), 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(homeDir, ".config", "escape")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	event := ApplyDotfile(context.Background(), homeDir, DesiredResource{
		ID:             "resource_dot_escape",
		Kind:           "dotfile",
		DesiredVersion: 4,
		Spec:           dotfileResourceSpec("~/.config/escape/file", "copy"),
	})

	requireDotfileEvent(t, event, "blocked", "symlink_escape", 0)
	if _, err := os.Stat(filepath.Join(outsideDir, "file")); !os.IsNotExist(err) {
		t.Fatalf("outside file stat error = %v, want absent", err)
	}
}

func TestDotfileCopy_blocksConflicts_withoutChangingExistingTarget(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, homeDir string)
		message string
	}{
		{
			name: "regular file with different content",
			setup: func(t *testing.T, homeDir string) {
				if err := os.WriteFile(filepath.Join(homeDir, ".zshrc"), []byte("existing\n"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
			message: "conflict_existing_file",
		},
		{
			name: "directory target",
			setup: func(t *testing.T, homeDir string) {
				if err := os.Mkdir(filepath.Join(homeDir, ".zshrc"), 0o700); err != nil {
					t.Fatalf("Mkdir() error = %v", err)
				}
			},
			message: "conflict_existing_directory",
		},
		{
			name: "symlink target",
			setup: func(t *testing.T, homeDir string) {
				if err := os.Symlink(filepath.Join(homeDir, "managed"), filepath.Join(homeDir, ".zshrc")); err != nil {
					t.Fatalf("Symlink() error = %v", err)
				}
			},
			message: "conflict_existing_symlink",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			tt.setup(t, homeDir)

			event := ApplyDotfile(context.Background(), homeDir, DesiredResource{
				ID:             "resource_dot_zshrc",
				Kind:           "dotfile",
				DesiredVersion: 8,
				Spec:           dotfileResourceSpec("~/.zshrc", "copy"),
			})

			requireDotfileEvent(t, event, "blocked", tt.message, 0)
			if tt.message == "conflict_existing_file" {
				body, err := os.ReadFile(filepath.Join(homeDir, ".zshrc"))
				if err != nil {
					t.Fatalf("ReadFile() error = %v", err)
				}
				if string(body) != "existing\n" {
					t.Fatalf("content = %q, want unchanged existing content", string(body))
				}
			}
		})
	}
}

func TestDotfileSymlink_blocksRegularFileConflict_withoutChangingExistingTarget(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".gitconfig")
	if err := os.WriteFile(targetPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	event := ApplyDotfile(context.Background(), homeDir, DesiredResource{
		ID:             "resource_dot_gitconfig",
		Kind:           "dotfile",
		DesiredVersion: 5,
		Spec:           dotfileResourceSpec("~/.gitconfig", "symlink"),
	})

	requireDotfileEvent(t, event, "blocked", "conflict_existing_file", 0)
	body, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "existing\n" {
		t.Fatalf("content = %q, want unchanged existing content", string(body))
	}
}

func dotfileResourceSpec(path string, applyMode string) map[string]interface{} {
	return map[string]interface{}{
		"path":          path,
		"content":       "desired\n",
		"mode":          "0600",
		"applyMode":     applyMode,
		"targetSegment": "base",
	}
}

func requireDotfileEvent(t *testing.T, event ResourceEvent, status string, message string, appliedVersion int) {
	t.Helper()
	if event.Status != status {
		t.Fatalf("status = %s, want %s; event=%+v", event.Status, status, event)
	}
	if event.Message != message {
		t.Fatalf("message = %q, want %q; event=%+v", event.Message, message, event)
	}
	if event.AppliedVersion != appliedVersion {
		t.Fatalf("appliedVersion = %d, want %d; event=%+v", event.AppliedVersion, appliedVersion, event)
	}
}

type fakePackageAdapter struct {
	installed map[string]string
}

func (f *fakePackageAdapter) Check(_ context.Context, name string, desiredVersion string) (string, error) {
	if f.installed[name] == desiredVersion || desiredVersion == "latest" && f.installed[name] != "" {
		return "in_sync", nil
	}
	return "drifted", nil
}

func (f *fakePackageAdapter) Apply(_ context.Context, name string, desiredVersion string) (string, error) {
	f.installed[name] = desiredVersion
	return "in_sync", nil
}
