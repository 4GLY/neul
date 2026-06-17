package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/4gly/neul/internal/domain/dotfiles"
)

func blockedDotfileEvent(resource DesiredResource, message string) ResourceEvent {
	return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: message, DesiredVersion: resource.DesiredVersion}
}

func parseFileMode(modeText string) (os.FileMode, error) {
	parsed, err := strconv.ParseUint(modeText, 8, 32)
	if err != nil || parsed > 0o777 {
		return 0, dotfiles.NewPolicyError(dotfiles.MessageInvalidSpec, fmt.Errorf("invalid mode %q", modeText))
	}
	return os.FileMode(parsed), nil
}

func safeManagedSegment(value string) bool {
	return dotfiles.SafeManagedSegment(value)
}

func hasMultipleLinks(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}

func managedDotfileDir(homeDir string, resourceID string, spec dotfileSpec) (string, error) {
	managedDir := filepath.Join(absPath(homeDir), ".local", "state", "neul", "dotfiles", spec.targetSegment, resourceID)
	if !dotfiles.PathInside(absPath(homeDir), managedDir) {
		return "", dotfiles.NewPolicyError(dotfiles.MessageInvalidSpec, errors.New("managed path escapes home"))
	}
	return managedDir, nil
}

func writeManagedDotfile(homeDir string, resourceID string, spec dotfileSpec) (string, string, error) {
	sum := sha256.Sum256([]byte(spec.content))
	contentHash := hex.EncodeToString(sum[:])
	managedDir, err := managedDotfileDir(homeDir, resourceID, spec)
	if err != nil {
		return "", "", err
	}
	managedPath := filepath.Join(managedDir, contentHash)
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o700); err != nil {
		return "", "", dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("mkdir managed path: %w", err))
	}
	if err := dotfiles.RejectExistingSymlinkEscape(absPath(homeDir), managedPath); err != nil {
		return "", "", err
	}
	if err := writeFileAtomically(homeDir, managedPath, []byte(spec.content), spec.mode); err != nil {
		return "", "", err
	}
	return managedDir, managedPath, nil
}

func conflictForExistingInfo(info os.FileInfo) error {
	switch {
	case info.IsDir():
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingDirectory, errors.New("target is a directory"))
	case info.Mode()&os.ModeSymlink != 0:
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingSymlink, errors.New("target is a symlink"))
	case info.Mode().IsRegular():
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingFile, errors.New("target is a regular file"))
	default:
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingNode, errors.New("target is not a regular file"))
	}
}

func syncDirectory(path string) (err error) {
	dir, err := os.Open(path)
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("open directory: %w", err))
	}
	defer func() {
		if closeErr := dir.Close(); closeErr != nil && err == nil {
			err = dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("close directory: %w", closeErr))
		}
	}()
	if err := dir.Sync(); err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("sync directory: %w", err))
	}
	return nil
}

func pruneManagedSiblings(managedDir string, keepPath string) error {
	entries, err := os.ReadDir(managedDir)
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("read managed directory: %w", err))
	}
	for _, entry := range entries {
		path := filepath.Join(managedDir, entry.Name())
		if path == keepPath {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("remove stale managed content: %w", err))
		}
	}
	return syncDirectory(managedDir)
}

func ensureSafeParent(homeDir string, targetPath string) error {
	absoluteHome := absPath(homeDir)
	parent := filepath.Dir(targetPath)
	relative, err := filepath.Rel(absoluteHome, parent)
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessagePathNotAllowed, fmt.Errorf("parent rel: %w", err))
	}
	current := absoluteHome
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if err := ensureSafeDirectory(absoluteHome, current); err != nil {
			return err
		}
	}
	return nil
}

func ensureSafeDirectory(homeDir string, path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o755); err != nil {
			return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("mkdir parent: %w", err))
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("lstat parent: %w", err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !dotfiles.PathInside(homeDir, resolved) {
			return dotfiles.NewPolicyError(dotfiles.MessageSymlinkEscape, errors.New("parent symlink escapes home"))
		}
		return nil
	}
	if !info.IsDir() {
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingNode, errors.New("parent is not a directory"))
	}
	return nil
}

func writeFileAtomically(homeDir string, targetPath string, body []byte, mode os.FileMode) (err error) {
	if err := dotfiles.RejectExistingSymlinkEscape(absPath(homeDir), targetPath); err != nil {
		return err
	}
	parent := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(parent, ".neul-dotfile-*")
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("create temp: %w", err))
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if err != nil && !renamed {
			removeErr := os.Remove(tmpPath)
			if removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err = tmp.Write(body); err != nil {
		err = dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("write temp: %w", err))
		return err
	}
	if err = tmp.Chmod(mode); err != nil {
		err = dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("chmod temp: %w", err))
		return err
	}
	if err = tmp.Sync(); err != nil {
		err = dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("sync temp: %w", err))
		return err
	}
	if err = tmp.Close(); err != nil {
		err = dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("close temp: %w", err))
		return err
	}
	if err = dotfiles.RejectExistingSymlinkEscape(absPath(homeDir), targetPath); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, targetPath); err != nil {
		err = dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("rename temp: %w", err))
		return err
	}
	renamed = true
	if err = syncDirectory(parent); err != nil {
		return err
	}
	return nil
}

func managedContentExists(managedDir string, body []byte) bool {
	entries, err := os.ReadDir(managedDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(managedDir, entry.Name())
		stored, err := os.ReadFile(path)
		if err == nil && string(stored) == string(body) {
			return true
		}
	}
	return false
}

func dotfileFingerprint(targetPath string) string {
	info, err := os.Lstat(targetPath)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "error"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(targetPath)
		if err != nil {
			return "symlink:error"
		}
		sum := sha256.Sum256([]byte(target))
		return "symlink:" + hex.EncodeToString(sum[:])
	}
	if !info.Mode().IsRegular() {
		return "node:" + info.Mode().Type().String()
	}
	body, err := os.ReadFile(targetPath)
	if err != nil {
		return "file:error"
	}
	sum := sha256.Sum256(body)
	return "file:" + hex.EncodeToString(sum[:])
}

func managedPathOwnedByResource(managedDir string, candidate string) bool {
	relative, err := filepath.Rel(managedDir, candidate)
	if err != nil {
		return false
	}
	return relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." && !strings.Contains(relative, string(filepath.Separator))
}

func replaceSymlink(homeDir string, targetPath string, managedPath string) error {
	if err := ensureSafeParent(homeDir, targetPath); err != nil {
		return err
	}
	parent := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(parent, ".neul-dotfile-link-*")
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("create temp symlink name: %w", err))
	}
	tmpLink := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpLink)
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("close temp symlink name: %w", err))
	}
	if err := os.Remove(tmpLink); err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("prepare temp symlink: %w", err))
	}
	if err := os.Symlink(managedPath, tmpLink); err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("create temp symlink: %w", err))
	}
	if err := os.Rename(tmpLink, targetPath); err != nil {
		_ = os.Remove(tmpLink)
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("replace target symlink: %w", err))
	}
	return syncDirectory(parent)
}

func absPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}
