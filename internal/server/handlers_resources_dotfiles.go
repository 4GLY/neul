package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/4gly/neul/internal/domain/dotfiles"
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
	normalizedPath, err := dotfiles.NormalizeAllowedPath(homeDir, rawPath)
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
	if err := dotfiles.ValidateSpec(mode, applyMode, targetSegment); err != nil {
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
