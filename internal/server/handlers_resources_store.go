package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/4gly/neul/internal/domain"
)

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
		if err := insertFileVersion(r, db, resourceID, spec["content"], now); err != nil {
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

func queryResourceByID(r *http.Request, queryer resourceQueryer, resourceID string) (resourceResponse, error) {
	row := queryer.QueryRowContext(
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
