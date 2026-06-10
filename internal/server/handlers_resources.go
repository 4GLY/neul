package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/4gly/neul/internal/domain"
)

func handleListResources(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resources, err := queryResources(r, db)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resources_query_failed", "Could not read resources.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"resources": resources})
	})
}

func handleCreatePackageResource(db *sql.DB, clock func() time.Time) http.Handler {
	type requestBody struct {
		Name           string `json:"name"`
		SourceKind     string `json:"sourceKind"`
		DesiredVersion string `json:"desiredVersion"`
		TargetSegment  string `json:"targetSegment"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if body.Name == "" || body.DesiredVersion == "" || body.TargetSegment == "" || !validPackageSource(body.SourceKind) {
			writeJSONError(w, http.StatusBadRequest, "package_invalid", "Package name, source kind, version, and segment are required.")
			return
		}
		spec := map[string]string{
			"name":           body.Name,
			"sourceKind":     body.SourceKind,
			"desiredVersion": body.DesiredVersion,
			"targetSegment":  body.TargetSegment,
		}
		resource, err := insertResource(r, db, clock, body.TargetSegment, string(domain.ResourceKindPackage), body.Name, spec)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_create_failed", "Could not create package resource.")
			return
		}
		if err := markResourcePendingForMachines(r, db, clock, resource); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_pending_failed", "Could not mark resource pending.")
			return
		}
		writeJSON(w, http.StatusCreated, resource)
	})
}

func handleCreateDotfileResource(db *sql.DB, clock func() time.Time, homeDir string) http.Handler {
	type requestBody struct {
		Path          string `json:"path"`
		Content       string `json:"content"`
		Mode          string `json:"mode"`
		ApplyMode     string `json:"applyMode"`
		TargetSegment string `json:"targetSegment"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		normalizedPath, err := normalizeAllowedDotfilePath(homeDir, body.Path)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "path_not_allowed", "Dotfile path is not allowed.")
			return
		}
		if body.Mode == "" || body.ApplyMode == "" || body.TargetSegment == "" {
			writeJSONError(w, http.StatusBadRequest, "dotfile_invalid", "Dotfile mode, apply mode, and segment are required.")
			return
		}
		spec := map[string]string{
			"path":          normalizedPath,
			"content":       body.Content,
			"mode":          body.Mode,
			"applyMode":     body.ApplyMode,
			"targetSegment": body.TargetSegment,
		}
		resource, err := insertResource(r, db, clock, body.TargetSegment, string(domain.ResourceKindDotfile), normalizedPath, spec)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_create_failed", "Could not create dotfile resource.")
			return
		}
		if err := markResourcePendingForMachines(r, db, clock, resource); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_pending_failed", "Could not mark resource pending.")
			return
		}
		writeJSON(w, http.StatusCreated, resource)
	})
}

func handlePatchResource(db *sql.DB, clock func() time.Time, homeDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceId")
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		current, err := queryResourceByID(r, db, resourceID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, http.StatusInternalServerError, "resource_query_failed", "Could not read resource.")
				return
			}
			writeJSONError(w, http.StatusNotFound, "resource_not_found", "Resource was not found.")
			return
		}
		nextSpec, nextName, err := mergeResourcePatch(homeDir, current, patch)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "resource_patch_invalid", "Resource patch is invalid.")
			return
		}
		specJSON, err := json.Marshal(nextSpec)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "resource_patch_invalid", "Resource patch is invalid.")
			return
		}
		now := clock().UTC().Format(time.RFC3339Nano)
		result, err := db.ExecContext(
			r.Context(),
			`UPDATE resources SET name = ?, spec_json = ?, desired_version = desired_version + 1, updated_at = ? WHERE id = ?`,
			nextName,
			string(specJSON),
			now,
			resourceID,
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_update_failed", "Could not update resource.")
			return
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			writeJSONError(w, http.StatusNotFound, "resource_not_found", "Resource was not found.")
			return
		}
		resource, err := queryResourceByID(r, db, resourceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_query_failed", "Could not read resource.")
			return
		}
		if err := markResourcePendingForMachines(r, db, clock, resource); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_pending_failed", "Could not mark resource pending.")
			return
		}
		writeJSON(w, http.StatusOK, resource)
	})
}

func handleDeleteResource(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := db.ExecContext(r.Context(), `DELETE FROM resources WHERE id = ?`, r.PathValue("resourceId"))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resource_delete_failed", "Could not delete resource.")
			return
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			writeJSONError(w, http.StatusNotFound, "resource_not_found", "Resource was not found.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
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
