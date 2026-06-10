package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	errDotfileInvalid        = errors.New("dotfile invalid")
	errDotfilePathNotAllowed = errors.New("dotfile path not allowed")
)

type fileVersionExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type resourceQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func normalizeDotfilePatch(r *http.Request, queryer resourceQueryer, resourceID string, patch map[string]interface{}, homeDir string) (map[string]interface{}, error) {
	current, err := queryResourceByID(r, queryer, resourceID)
	if err != nil {
		return nil, err
	}
	rawPath, err := patchString(patch, current.Spec, "path")
	if err != nil {
		return nil, err
	}
	normalizedPath, err := normalizeAllowedDotfilePath(homeDir, rawPath)
	if err != nil {
		return nil, errDotfilePathNotAllowed
	}
	content, err := patchString(patch, current.Spec, "content")
	if err != nil {
		return nil, err
	}
	mode, err := patchString(patch, current.Spec, "mode")
	if err != nil {
		return nil, err
	}
	applyMode, err := patchString(patch, current.Spec, "applyMode")
	if err != nil {
		return nil, err
	}
	targetSegment, err := patchString(patch, current.Spec, "targetSegment")
	if err != nil {
		return nil, err
	}
	next := map[string]interface{}{
		"path":          normalizedPath,
		"content":       content,
		"mode":          mode,
		"applyMode":     applyMode,
		"targetSegment": targetSegment,
	}
	if err := validateDotfileSpec(mode, applyMode, targetSegment); err != nil {
		return nil, err
	}
	return next, nil
}

func validateDotfileSpec(mode string, applyMode string, targetSegment string) error {
	if mode == "" || targetSegment == "" {
		return errDotfileInvalid
	}
	if applyMode != "copy" && applyMode != "symlink" {
		return errDotfileInvalid
	}
	parsedMode, err := strconv.ParseUint(mode, 8, 32)
	if err != nil || parsedMode > 0o777 {
		return errDotfileInvalid
	}
	return nil
}

func insertFileVersion(r *http.Request, exec fileVersionExecutor, resourceID string, content string, createdAt string) error {
	_, err := exec.ExecContext(
		r.Context(),
		`INSERT INTO file_versions (id, resource_id, content_hash, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		"file_version_"+hashSecret(resourceID + content + createdAt)[:16],
		resourceID,
		hashSecret(content),
		content,
		createdAt,
	)
	return err
}

func resourceKindByID(r *http.Request, queryer resourceQueryer, resourceID string) (string, error) {
	var kind string
	if err := queryer.QueryRowContext(r.Context(), `SELECT kind FROM resources WHERE id = ?`, resourceID).Scan(&kind); err != nil {
		return "", fmt.Errorf("query resource kind: %w", err)
	}
	return kind, nil
}

func patchString(primary map[string]interface{}, fallback map[string]interface{}, key string) (string, error) {
	value, exists := primary[key]
	if !exists {
		return stringSpecValue(fallback, key), nil
	}
	text, ok := value.(string)
	if !ok {
		return "", errDotfileInvalid
	}
	return text, nil
}

func stringSpecValue(spec map[string]interface{}, key string) string {
	value, _ := spec[key].(string)
	return value
}

func normalizeAllowedDotfilePath(homeDir string, rawPath string) (string, error) {
	if homeDir == "" || rawPath == "" || filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("path not allowed")
	}
	if strings.Contains(rawPath, "..") {
		return "", fmt.Errorf("path traversal")
	}
	if rawPath != "~/.zshrc" && rawPath != "~/.gitconfig" && !strings.HasPrefix(rawPath, "~/.config/") {
		return "", fmt.Errorf("path not allowlisted")
	}
	relative := strings.TrimPrefix(rawPath, "~/")
	cleanRelative := filepath.Clean(relative)
	if cleanRelative == "." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) || cleanRelative == ".." {
		return "", fmt.Errorf("path traversal")
	}
	if cleanRelative != ".zshrc" && cleanRelative != ".gitconfig" && !strings.HasPrefix(cleanRelative, ".config"+string(filepath.Separator)) {
		return "", fmt.Errorf("path not allowlisted")
	}
	absoluteHome, err := filepath.Abs(homeDir)
	if err != nil {
		return "", fmt.Errorf("home path invalid: %w", err)
	}
	absoluteTarget := filepath.Join(absoluteHome, cleanRelative)
	if err := rejectExistingSymlinkEscape(absoluteHome, absoluteTarget); err != nil {
		return "", err
	}
	return "~/" + filepath.ToSlash(cleanRelative), nil
}

func rejectExistingSymlinkEscape(homeDir string, targetPath string) error {
	current := homeDir
	relative, err := filepath.Rel(homeDir, targetPath)
	if err != nil {
		return fmt.Errorf("path rel: %w", err)
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
			return fmt.Errorf("lstat path: %w", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil {
			return fmt.Errorf("eval symlink: %w", err)
		}
		if !pathInside(homeDir, resolved) {
			return fmt.Errorf("symlink escape")
		}
	}
	return nil
}

func pathInside(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "..")
}
