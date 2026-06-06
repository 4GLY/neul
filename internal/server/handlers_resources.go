package server

import (
	"database/sql"
	"encoding/json"
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
		writeJSON(w, http.StatusCreated, resource)
	})
}

func handlePatchResource(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceId")
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		specJSON, err := json.Marshal(patch)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "resource_patch_invalid", "Resource patch is invalid.")
			return
		}
		now := clock().UTC().Format(time.RFC3339Nano)
		result, err := db.ExecContext(
			r.Context(),
			`UPDATE resources SET spec_json = ?, desired_version = desired_version + 1, updated_at = ? WHERE id = ?`,
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

type resourceResponse struct {
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	Name           string                 `json:"name"`
	DesiredVersion int                    `json:"desiredVersion"`
	AgentSupport   string                 `json:"agentSupport"`
	Spec           map[string]interface{} `json:"spec"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

func insertResource(r *http.Request, db *sql.DB, clock func() time.Time, segmentName string, kind string, name string, spec map[string]string) (resourceResponse, error) {
	segmentID, err := segmentIDByName(r, db, segmentName)
	if err != nil {
		return resourceResponse{}, err
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return resourceResponse{}, fmt.Errorf("marshal spec: %w", err)
	}
	now := clock().UTC().Format(time.RFC3339Nano)
	resourceID := "resource_" + hashSecret(kind + name + now)[:16]
	_, err = db.ExecContext(
		r.Context(),
		`INSERT INTO resources (id, segment_id, kind, name, spec_json, desired_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		resourceID,
		segmentID,
		kind,
		name,
		string(specJSON),
		now,
		now,
	)
	if err != nil {
		return resourceResponse{}, fmt.Errorf("insert resource: %w", err)
	}
	if kind == string(domain.ResourceKindDotfile) {
		_, err = db.ExecContext(
			r.Context(),
			`INSERT INTO file_versions (id, resource_id, content_hash, content, created_at) VALUES (?, ?, ?, ?, ?)`,
			"file_version_"+hashSecret(resourceID + spec["content"])[:16],
			resourceID,
			hashSecret(spec["content"]),
			spec["content"],
			now,
		)
		if err != nil {
			return resourceResponse{}, fmt.Errorf("insert file version: %w", err)
		}
	}
	return queryResourceByID(r, db, resourceID)
}

func queryResources(r *http.Request, db *sql.DB) ([]resourceResponse, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT id, kind, name, spec_json, desired_version, created_at, updated_at FROM resources WHERE kind != 'secret' ORDER BY created_at DESC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query resources: %w", err)
	}
	defer rows.Close()

	resources := make([]resourceResponse, 0)
	for rows.Next() {
		resource, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resources: %w", err)
	}
	return resources, nil
}

func queryResourceByID(r *http.Request, db *sql.DB, resourceID string) (resourceResponse, error) {
	row := db.QueryRowContext(
		r.Context(),
		`SELECT id, kind, name, spec_json, desired_version, created_at, updated_at FROM resources WHERE id = ?`,
		resourceID,
	)
	return scanResource(row)
}

type resourceScanner interface {
	Scan(dest ...interface{}) error
}

func scanResource(scanner resourceScanner) (resourceResponse, error) {
	var response resourceResponse
	var specJSON string
	if err := scanner.Scan(&response.ID, &response.Kind, &response.Name, &specJSON, &response.DesiredVersion, &response.CreatedAt, &response.UpdatedAt); err != nil {
		return resourceResponse{}, fmt.Errorf("scan resource: %w", err)
	}
	if err := json.Unmarshal([]byte(specJSON), &response.Spec); err != nil {
		return resourceResponse{}, fmt.Errorf("decode resource spec: %w", err)
	}
	response.AgentSupport = agentSupport(response)
	return response, nil
}

func segmentIDByName(r *http.Request, db *sql.DB, segmentName string) (string, error) {
	var segmentID string
	err := db.QueryRowContext(r.Context(), `SELECT id FROM segments WHERE name = ? ORDER BY priority LIMIT 1`, segmentName).Scan(&segmentID)
	if err != nil {
		return "", fmt.Errorf("query segment: %w", err)
	}
	return segmentID, nil
}

func validPackageSource(sourceKind string) bool {
	return sourceKind == "brew" || sourceKind == "apt" || sourceKind == "mise"
}

func agentSupport(resource resourceResponse) string {
	if resource.Kind != string(domain.ResourceKindPackage) {
		return "supported"
	}
	sourceKind, _ := resource.Spec["sourceKind"].(string)
	if sourceKind == "brew" {
		return "supported"
	}
	return "unsupported"
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
