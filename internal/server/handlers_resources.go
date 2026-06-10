package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
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
		if err := validateDotfileSpec(body.Mode, body.ApplyMode, body.TargetSegment); err != nil {
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

func handlePatchResource(db *sql.DB, clock func() time.Time, homeDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceId")
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		now := clock().UTC().Format(time.RFC3339Nano)
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not update resource.")
			return
		}
		defer func() {
			_ = tx.Rollback()
		}()
		kind, err := resourceKindByID(r, tx, resourceID)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "resource_not_found", "Resource was not found.")
			return
		}
		if kind == string(domain.ResourceKindDotfile) {
			patch, err = normalizeDotfilePatch(r, tx, resourceID, patch, homeDir)
			if err != nil {
				if errors.Is(err, errDotfilePathNotAllowed) {
					writeJSONError(w, http.StatusBadRequest, "path_not_allowed", "Dotfile path is not allowed.")
					return
				}
				if errors.Is(err, errDotfileInvalid) {
					writeJSONError(w, http.StatusBadRequest, "dotfile_invalid", "Dotfile patch is invalid.")
					return
				}
				writeJSONError(w, http.StatusInternalServerError, "resource_query_failed", "Could not read resource.")
				return
			}
		}
		specJSON, err := json.Marshal(patch)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "resource_patch_invalid", "Resource patch is invalid.")
			return
		}
		result, err := tx.ExecContext(
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
		if kind == string(domain.ResourceKindDotfile) {
			if err := insertFileVersion(r, tx, resourceID, stringSpecValue(patch, "content"), now); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "resource_update_failed", "Could not update resource.")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not update resource.")
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

func validPackageSource(sourceKind string) bool {
	return sourceKind == "brew" || sourceKind == "apt" || sourceKind == "mise"
}
