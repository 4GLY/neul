package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/4gly/neul/internal/domain"
)

type machineSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	AgentVersion    string `json:"agentVersion"`
	Status          string `json:"status"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	DriftCount      int    `json:"driftCount"`
	PendingCount    int    `json:"pendingCount"`
	BlockedCount    int    `json:"blockedCount"`
}

type machineCounts struct {
	Drifted int
	Pending int
	Blocked int
}

func handleDashboard(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machines, err := queryMachineSummaries(r, db, clock().UTC())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "dashboard_query_failed", "Could not read dashboard.")
			return
		}
		metrics := map[string]int{
			"total":   len(machines),
			"healthy": 0,
			"drifted": 0,
			"pending": 0,
			"offline": 0,
			"blocked": 0,
		}
		for _, machine := range machines {
			metrics[machine.Status]++
		}
		body := map[string]interface{}{
			"metrics":  metrics,
			"machines": machines,
			"activity": []interface{}{},
			"ledger":   []interface{}{},
		}
		if len(machines) == 0 {
			body["emptyState"] = map[string]string{
				"action": "create_pairing_code",
				"title":  "첫 머신을 등록하세요",
			}
		}
		writeJSON(w, http.StatusOK, body)
	})
}

func handleListMachines(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machines, err := queryMachineSummaries(r, db, clock().UTC())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "machines_query_failed", "Could not read machines.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"machines": machines})
	})
}

func handleGetMachine(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machineID := r.PathValue("machineId")
		machines, err := queryMachineSummaries(r, db, clock().UTC())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "machine_query_failed", "Could not read machine.")
			return
		}
		var selected machineSummary
		found := false
		for _, machine := range machines {
			if machine.ID == machineID {
				selected = machine
				found = true
				break
			}
		}
		if !found {
			writeJSONError(w, http.StatusNotFound, "machine_not_found", "Machine was not found.")
			return
		}
		events, err := queryMachineEvents(r, db, machineID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "events_query_failed", "Could not read machine events.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"machine":         selected,
			"events":          events,
			"latestReconcile": latestEvent(events),
			"driftSummary": map[string]int{
				"drifted": selected.DriftCount,
				"pending": selected.PendingCount,
				"blocked": selected.BlockedCount,
			},
		})
	})
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

func queryMachineSummaries(r *http.Request, db *sql.DB, now time.Time) ([]machineSummary, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT id, name, os, arch, COALESCE(last_heartbeat_at, ''), COALESCE(agent_version, '') FROM machines ORDER BY created_at DESC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query machines: %w", err)
	}
	defer rows.Close()

	machines := make([]machineSummary, 0)
	for rows.Next() {
		var machine machineSummary
		var lastHeartbeat string
		if err := rows.Scan(&machine.ID, &machine.Name, &machine.OS, &machine.Arch, &lastHeartbeat, &machine.AgentVersion); err != nil {
			return nil, fmt.Errorf("scan machine: %w", err)
		}
		counts, err := queryMachineCounts(r, db, machine.ID)
		if err != nil {
			return nil, err
		}
		machine.DriftCount = counts.Drifted
		machine.PendingCount = counts.Pending
		machine.BlockedCount = counts.Blocked
		snapshot := domain.MachineSnapshot{
			DriftCount:   counts.Drifted,
			PendingCount: counts.Pending,
			BlockedCount: counts.Blocked,
		}
		if lastHeartbeat != "" {
			parsed, err := time.Parse(time.RFC3339Nano, lastHeartbeat)
			if err != nil {
				return nil, fmt.Errorf("parse heartbeat: %w", err)
			}
			snapshot.LastHeartbeatAt = parsed
			machine.LastHeartbeatAt = lastHeartbeat
		}
		machine.Status = string(domain.ComputeMachineStatus(snapshot, now))
		machines = append(machines, machine)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machines: %w", err)
	}
	return machines, nil
}

func queryMachineCounts(r *http.Request, db *sql.DB, machineID string) (machineCounts, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT e.status, COALESCE(e.desired_version, r.desired_version), COALESCE(e.applied_version, 0)
		 FROM reconcile_events e
		 JOIN reconcile_runs rr ON rr.id = e.run_id
		 LEFT JOIN resources r ON r.id = e.resource_id
		 WHERE rr.machine_id = ?`,
		machineID,
	)
	if err != nil {
		return machineCounts{}, fmt.Errorf("query counts: %w", err)
	}
	defer rows.Close()

	var counts machineCounts
	for rows.Next() {
		var status string
		var desiredVersion int
		var appliedVersion int
		if err := rows.Scan(&status, &desiredVersion, &appliedVersion); err != nil {
			return machineCounts{}, fmt.Errorf("scan count: %w", err)
		}
		switch status {
		case string(domain.ResourceStateDrifted):
			counts.Drifted++
		case string(domain.ResourceStateBlocked):
			counts.Blocked++
		}
		if desiredVersion > appliedVersion {
			counts.Pending++
		}
	}
	if err := rows.Err(); err != nil {
		return machineCounts{}, fmt.Errorf("iterate counts: %w", err)
	}
	return counts, nil
}

func queryMachineEvents(r *http.Request, db *sql.DB, machineID string) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT e.id, COALESCE(e.resource_id, ''), e.status, COALESCE(e.message, ''), e.created_at
		 FROM reconcile_events e
		 JOIN reconcile_runs rr ON rr.id = e.run_id
		 WHERE rr.machine_id = ?
		 ORDER BY e.created_at DESC
		 LIMIT 25`,
		machineID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id string
		var resourceID string
		var status string
		var message string
		var createdAt string
		if err := rows.Scan(&id, &resourceID, &status, &message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, map[string]interface{}{
			"id":         id,
			"resourceId": resourceID,
			"status":     status,
			"message":    message,
			"createdAt":  createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func latestEvent(events []map[string]interface{}) interface{} {
	if len(events) == 0 {
		return nil
	}
	return events[0]
}

type repairCommandResponse struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
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
		`SELECT DISTINCT e.resource_id
		 FROM reconcile_events e
		 JOIN reconcile_runs rr ON rr.id = e.run_id
		 WHERE rr.machine_id = ? AND e.status = ? AND e.resource_id IS NOT NULL
		 ORDER BY e.resource_id`,
		machineID,
		string(domain.ResourceStateDrifted),
	)
	if err != nil {
		return nil, fmt.Errorf("query drifted resources: %w", err)
	}
	defer rows.Close()

	var resourceIDs []string
	for rows.Next() {
		var resourceID string
		if err := rows.Scan(&resourceID); err != nil {
			return nil, fmt.Errorf("scan drifted resource: %w", err)
		}
		resourceIDs = append(resourceIDs, resourceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate drifted resources: %w", err)
	}
	return resourceIDs, nil
}
