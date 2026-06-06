package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type machineContextKey struct{}

func requireMachineToken(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || token == r.Header.Get("Authorization") {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Machine token is required.")
			return
		}
		var machineID string
		err := db.QueryRowContext(
			r.Context(),
			`SELECT machine_id FROM machine_tokens WHERE token_hash = ? AND revoked_at IS NULL`,
			hashSecret(token),
		).Scan(&machineID)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Machine token is required.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), machineContextKey{}, machineID)))
	})
}

func machineIDFromRequest(r *http.Request) string {
	machineID, _ := r.Context().Value(machineContextKey{}).(string)
	return machineID
}

func handleAgentHeartbeat(db *sql.DB, clock func() time.Time) http.Handler {
	type requestBody struct {
		AgentVersion string `json:"agentVersion"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		_, err := db.ExecContext(
			r.Context(),
			`UPDATE machines SET last_heartbeat_at = ?, agent_version = COALESCE(NULLIF(?, ''), agent_version) WHERE id = ?`,
			clock().UTC().Format(time.RFC3339Nano),
			body.AgentVersion,
			machineIDFromRequest(r),
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "heartbeat_failed", "Could not store heartbeat.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func handleAgentDesiredState(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resources, err := queryResources(r, db)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "desired_state_failed", "Could not read desired state.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"resources": resources})
	})
}

func handleAgentCommands(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(
			r.Context(),
			`SELECT id, command_type, payload_json FROM agent_commands WHERE machine_id = ? AND status = 'queued' ORDER BY created_at ASC`,
			machineIDFromRequest(r),
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "commands_query_failed", "Could not read commands.")
			return
		}
		defer rows.Close()
		commands := make([]map[string]interface{}, 0)
		for rows.Next() {
			var id string
			var commandType string
			var payloadJSON string
			if err := rows.Scan(&id, &commandType, &payloadJSON); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "commands_query_failed", "Could not read commands.")
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "command_payload_invalid", "Command payload is invalid.")
				return
			}
			commands = append(commands, map[string]interface{}{"id": id, "type": commandType, "payload": payload})
		}
		if err := rows.Err(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "commands_query_failed", "Could not read commands.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"commands": commands})
	})
}

func handleAgentReport(db *sql.DB, clock func() time.Time, reason string) http.Handler {
	type reportEvent struct {
		ResourceID     string `json:"resourceId"`
		Status         string `json:"status"`
		Message        string `json:"message"`
		DesiredVersion int    `json:"desiredVersion"`
		AppliedVersion int    `json:"appliedVersion"`
	}
	type requestBody struct {
		CommandID string        `json:"commandId"`
		Status    string        `json:"status"`
		Events    []reportEvent `json:"events"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			writeJSONError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required.")
			return
		}
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		runID := "run_" + hashSecret(machineIDFromRequest(r) + idempotencyKey)[:16]
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not store report.")
			return
		}
		defer func() {
			_ = tx.Rollback()
		}()
		var existingCount int
		if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM reconcile_runs WHERE idempotency_key = ?`, idempotencyKey).Scan(&existingCount); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "report_query_failed", "Could not store report.")
			return
		}
		if existingCount > 0 {
			if err := tx.Commit(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not store report.")
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
			return
		}
		now := clock().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(
			r.Context(),
			`INSERT INTO reconcile_runs (id, machine_id, reason, idempotency_key, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			runID,
			machineIDFromRequest(r),
			reason,
			idempotencyKey,
			reportStatus(body.Status),
			now,
		); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "run_insert_failed", "Could not store report.")
			return
		}
		if _, err := tx.ExecContext(
			r.Context(),
			`INSERT INTO agent_reports (id, machine_id, idempotency_key, report_json, created_at) VALUES (?, ?, ?, ?, ?)`,
			"agent_report_"+hashSecret(idempotencyKey)[:16],
			machineIDFromRequest(r),
			idempotencyKey,
			mustMarshalString(body),
			now,
		); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "agent_report_insert_failed", "Could not store report.")
			return
		}
		for index, event := range body.Events {
			if _, err := tx.ExecContext(
				r.Context(),
				`INSERT INTO reconcile_events (id, run_id, resource_id, status, message, desired_version, applied_version, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				fmt.Sprintf("event_%s_%d", hashSecret(idempotencyKey)[:16], index),
				runID,
				nullString(event.ResourceID),
				event.Status,
				event.Message,
				event.DesiredVersion,
				event.AppliedVersion,
				now,
			); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "event_insert_failed", "Could not store report.")
				return
			}
		}
		if body.CommandID != "" {
			if len(body.Events) == 0 {
				if _, err := tx.ExecContext(
					r.Context(),
					`INSERT INTO reconcile_events (id, run_id, resource_id, status, message, desired_version, applied_version, created_at) VALUES (?, ?, ?, ?, ?, 0, 0, ?)`,
					fmt.Sprintf("event_%s_command", hashSecret(idempotencyKey)[:16]),
					runID,
					sql.NullString{},
					reportStatus(body.Status),
					"repair command acknowledged",
					now,
				); err != nil {
					writeJSONError(w, http.StatusInternalServerError, "event_insert_failed", "Could not store report.")
					return
				}
			}
			if _, err := tx.ExecContext(r.Context(), `UPDATE agent_commands SET status = ?, acked_at = ? WHERE id = ? AND machine_id = ?`, reportStatus(body.Status), now, body.CommandID, machineIDFromRequest(r)); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "command_ack_failed", "Could not ack command.")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not store report.")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
	})
}

func reportStatus(status string) string {
	if status == "" {
		return "reported"
	}
	return status
}

func mustMarshalString(value interface{}) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
