package dotfiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	MessageApplied                   = "dotfile_applied"
	MessageDrifted                   = "dotfile_drifted"
	MessagePathNotAllowed            = "path_not_allowed"
	MessagePathTraversal             = "path_traversal"
	MessageSymlinkEscape             = "symlink_escape"
	MessageConflictExistingFile      = "conflict_existing_file"
	MessageConflictExistingDirectory = "conflict_existing_directory"
	MessageConflictExistingSymlink   = "conflict_existing_symlink"
	MessageConflictExistingNode      = "conflict_existing_node"
	MessageWriteFailed               = "write_failed"
	MessageInvalidSpec               = "invalid_spec"
	MessageHomeUnavailable           = "home_unavailable"
)

type PolicyError struct {
	Message string
	Err     error
}

func (e *PolicyError) Error() string {
	return e.Message + ": " + e.Err.Error()
}

func (e *PolicyError) Unwrap() error {
	return e.Err
}

func NewPolicyError(message string, err error) *PolicyError {
	if err == nil {
		err = errors.New(message)
	}
	return &PolicyError{Message: message, Err: err}
}

func MessageForError(err error) string {
	var policyErr *PolicyError
	if errors.As(err, &policyErr) {
		return policyErr.Message
	}
	return MessageWriteFailed
}

func ValidateSpec(mode string, applyMode string, targetSegment string) error {
	if mode == "" || targetSegment == "" || !SafeManagedSegment(targetSegment) {
		return NewPolicyError(MessageInvalidSpec, errors.New("mode and target segment are required"))
	}
	if applyMode != "copy" && applyMode != "symlink" {
		return NewPolicyError(MessageInvalidSpec, fmt.Errorf("unsupported apply mode %q", applyMode))
	}
	parsedMode, err := strconv.ParseUint(mode, 8, 32)
	if err != nil || parsedMode > 0o777 {
		return NewPolicyError(MessageInvalidSpec, fmt.Errorf("invalid mode %q", mode))
	}
	return nil
}

func SafeManagedSegment(value string) bool {
	return value != "" && !strings.Contains(value, "/") && !strings.Contains(value, string(filepath.Separator)) && value != "." && value != ".." && !strings.Contains(value, "..")
}

func NormalizeAllowedPath(homeDir string, rawPath string) (string, error) {
	normalizedPath, absoluteHome, err := normalizeAllowedPathSyntax(homeDir, rawPath)
	if err != nil {
		return "", err
	}
	absoluteTarget := filepath.Join(absoluteHome, filepath.FromSlash(strings.TrimPrefix(normalizedPath, "~/")))
	if err := RejectExistingSymlinkEscape(absoluteHome, absoluteTarget); err != nil {
		return "", err
	}
	return normalizedPath, nil
}

func NormalizeAllowedPathSyntax(homeDir string, rawPath string) (string, error) {
	normalizedPath, _, err := normalizeAllowedPathSyntax(homeDir, rawPath)
	return normalizedPath, err
}

func normalizeAllowedPathSyntax(homeDir string, rawPath string) (string, string, error) {
	if homeDir == "" || rawPath == "" || filepath.IsAbs(rawPath) {
		return "", "", NewPolicyError(MessagePathNotAllowed, errors.New("path not allowed"))
	}
	if strings.Contains(rawPath, "..") {
		return "", "", NewPolicyError(MessagePathTraversal, errors.New("path traversal"))
	}
	if rawPath != "~/.zshrc" && rawPath != "~/.gitconfig" && !strings.HasPrefix(rawPath, "~/.config/") {
		return "", "", NewPolicyError(MessagePathNotAllowed, errors.New("path not allowlisted"))
	}
	relative := strings.TrimPrefix(rawPath, "~/")
	cleanRelative := filepath.Clean(relative)
	if cleanRelative == "." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) || cleanRelative == ".." {
		return "", "", NewPolicyError(MessagePathTraversal, errors.New("path traversal"))
	}
	if cleanRelative != ".zshrc" && cleanRelative != ".gitconfig" && !strings.HasPrefix(cleanRelative, ".config"+string(filepath.Separator)) {
		return "", "", NewPolicyError(MessagePathNotAllowed, errors.New("path not allowlisted"))
	}
	absoluteHome, err := filepath.Abs(homeDir)
	if err != nil {
		return "", "", NewPolicyError(MessagePathNotAllowed, fmt.Errorf("home path invalid: %w", err))
	}
	return "~/" + filepath.ToSlash(cleanRelative), absoluteHome, nil
}

func AbsoluteTarget(homeDir string, normalizedPath string) (string, error) {
	if !strings.HasPrefix(normalizedPath, "~/") {
		return "", NewPolicyError(MessagePathNotAllowed, errors.New("normalized path must start with ~/"))
	}
	absoluteHome, err := filepath.Abs(homeDir)
	if err != nil {
		return "", NewPolicyError(MessagePathNotAllowed, fmt.Errorf("home path invalid: %w", err))
	}
	target := filepath.Join(absoluteHome, filepath.FromSlash(strings.TrimPrefix(normalizedPath, "~/")))
	if !PathInside(absoluteHome, target) {
		return "", NewPolicyError(MessagePathTraversal, errors.New("target escapes home"))
	}
	return target, nil
}

func RejectExistingSymlinkEscape(homeDir string, targetPath string) error {
	current := homeDir
	relative, err := filepath.Rel(homeDir, targetPath)
	if err != nil {
		return NewPolicyError(MessagePathNotAllowed, fmt.Errorf("path rel: %w", err))
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return NewPolicyError(MessageWriteFailed, fmt.Errorf("lstat path: %w", err))
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil {
			return NewPolicyError(MessageSymlinkEscape, fmt.Errorf("eval symlink: %w", err))
		}
		if !PathInside(homeDir, resolved) {
			return NewPolicyError(MessageSymlinkEscape, errors.New("symlink escape"))
		}
	}
	return nil
}

func PathInside(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "..")
}
