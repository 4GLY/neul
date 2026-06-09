package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/4gly/neul/internal/domain"
)

type repairCommandResponse struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
}

func handleRepairDrift(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machineID := r.PathValue("machineId")
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			writeJSONError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required.")
			return
		}
		existing, err := queryCommandByIdempotencyKey(r, db, idempotencyKey)
		if err == nil {
			writeJSON(w, http.StatusAccepted, existing)
			return
		}
		if err != sql.ErrNoRows {
			writeJSONError(w, http.StatusInternalServerError, "command_query_failed", "Could not read repair command.")
			return
		}
		resourceIDs, err := queryDriftedResourceIDs(r, db, machineID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "drift_query_failed", "Could not read drifted resources.")
			return
		}
		commandID := "command_" + hashSecret(machineID + idempotencyKey)[:16]
		payload, err := json.Marshal(map[string]interface{}{"resourceIds": resourceIDs})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "payload_encode_failed", "Could not create repair command.")
			return
		}
		now := clock().UTC().Format(time.RFC3339Nano)
		_, err = db.ExecContext(
			r.Context(),
			`INSERT INTO agent_commands (id, machine_id, command_type, payload_json, idempotency_key, status, created_at) VALUES (?, ?, 'repair_drift', ?, ?, 'queued', ?)`,
			commandID,
			machineID,
			string(payload),
			idempotencyKey,
			now,
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "command_create_failed", "Could not create repair command.")
			return
		}
		writeJSON(w, http.StatusAccepted, repairCommandResponse{CommandID: commandID, Status: "queued"})
	})
}

func queryCommandByIdempotencyKey(r *http.Request, db *sql.DB, idempotencyKey string) (repairCommandResponse, error) {
	var response repairCommandResponse
	err := db.QueryRowContext(
		r.Context(),
		`SELECT id, status FROM agent_commands WHERE idempotency_key = ?`,
		idempotencyKey,
	).Scan(&response.CommandID, &response.Status)
	return response, err
}

func queryDriftedResourceIDs(r *http.Request, db *sql.DB, machineID string) ([]string, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT e.resource_id, e.status
		 FROM reconcile_events e
		 JOIN reconcile_runs rr ON rr.id = e.run_id
		 JOIN resources r ON r.id = e.resource_id
		 WHERE rr.machine_id = ? AND rr.status IN ('reported', 'finished')
		 ORDER BY e.resource_id ASC, unixepoch(e.created_at) DESC, e.created_at DESC, e.rowid DESC`,
		machineID,
	)
	if err != nil {
		return nil, fmt.Errorf("query drifted resources: %w", err)
	}
	defer rows.Close()

	resourceIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var resourceID string
		var status string
		if err := rows.Scan(&resourceID, &status); err != nil {
			return nil, fmt.Errorf("scan drifted resource: %w", err)
		}
		if _, ok := seen[resourceID]; ok {
			continue
		}
		seen[resourceID] = struct{}{}
		if status != string(domain.ResourceStateDrifted) {
			continue
		}
		resourceIDs = append(resourceIDs, resourceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate drifted resources: %w", err)
	}
	return resourceIDs, nil
}
