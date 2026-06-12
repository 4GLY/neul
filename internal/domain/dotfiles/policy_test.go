package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAllowedPath_returnsNormalizedHomePath_whenPathIsAllowlisted(t *testing.T) {
	homeDir := t.TempDir()

	got, err := NormalizeAllowedPath(homeDir, "~/.config/neul/config.toml")

	if err != nil {
		t.Fatalf("NormalizeAllowedPath() error = %v", err)
	}
	if got != "~/.config/neul/config.toml" {
		t.Fatalf("normalized path = %q, want ~/.config/neul/config.toml", got)
	}
}

func TestNormalizeAllowedPath_returnsTraversalMessage_whenPathContainsDotDot(t *testing.T) {
	homeDir := t.TempDir()

	_, err := NormalizeAllowedPath(homeDir, "~/.config/../.ssh/config")

	if MessageForError(err) != MessagePathTraversal {
		t.Fatalf("message = %q, want %q; err=%v", MessageForError(err), MessagePathTraversal, err)
	}
}

func TestNormalizeAllowedPath_returnsSymlinkEscapeMessage_whenExistingSegmentEscapesHome(t *testing.T) {
	homeDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(homeDir, ".config"), 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(homeDir, ".config", "escape")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := NormalizeAllowedPath(homeDir, "~/.config/escape/file")

	if MessageForError(err) != MessageSymlinkEscape {
		t.Fatalf("message = %q, want %q; err=%v", MessageForError(err), MessageSymlinkEscape, err)
	}
}

func TestNormalizeAllowedPathSyntax_ignoresFinalTargetSymlink_whenAgentNeedsConflictClassification(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.Symlink(filepath.Join(homeDir, "managed"), filepath.Join(homeDir, ".zshrc")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := NormalizeAllowedPathSyntax(homeDir, "~/.zshrc")

	if err != nil {
		t.Fatalf("NormalizeAllowedPathSyntax() error = %v", err)
	}
	if got != "~/.zshrc" {
		t.Fatalf("normalized path = %q, want ~/.zshrc", got)
	}
}

func TestValidateSpec_rejectsUnsafeTargetSegments(t *testing.T) {
	tests := []string{"", ".", "..", "../escape", "base/escape", "base..escape"}
	for _, targetSegment := range tests {
		t.Run(targetSegment, func(t *testing.T) {
			if err := ValidateSpec("0600", "copy", targetSegment); MessageForError(err) != MessageInvalidSpec {
				t.Fatalf("ValidateSpec() message = %q, want %q; err=%v", MessageForError(err), MessageInvalidSpec, err)
			}
		})
	}
}

func TestValidateSpec_acceptsCopyAndSymlinkModes(t *testing.T) {
	for _, applyMode := range []string{"copy", "symlink"} {
		t.Run(applyMode, func(t *testing.T) {
			if err := ValidateSpec("0644", applyMode, "base"); err != nil {
				t.Fatalf("ValidateSpec() error = %v", err)
			}
		})
	}
}

func TestAbsoluteTarget_staysInsideHome(t *testing.T) {
	homeDir := t.TempDir()

	target, err := AbsoluteTarget(homeDir, "~/.config/neul/config.toml")

	if err != nil {
		t.Fatalf("AbsoluteTarget() error = %v", err)
	}
	if target != filepath.Join(homeDir, ".config", "neul", "config.toml") {
		t.Fatalf("target = %q, want path inside home", target)
	}
}

func TestPathInside_rejectsSiblingDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	sibling := filepath.Join(filepath.Dir(root), "sibling")

	if PathInside(root, sibling) {
		t.Fatalf("PathInside(%q, %q) = true, want false", root, sibling)
	}
}
