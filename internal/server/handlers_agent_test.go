package server

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDriftReport_persistsIdempotentlyAndUpdatesDashboard(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, ownerCookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_report", "mtn_report")
	seedResource(t, db, "resource_brew", "package", "kubectl", 1)

	body := `{"events":[{"resourceId":"resource_brew","status":"drifted","message":"kubectl missing","desiredVersion":1,"appliedVersion":0}]}`
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/agent/drift-report", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer mtn_report")
		request.Header.Set("Idempotency-Key", "drift-report-1")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("run %d status = %d, want %d; body=%s", i, recorder.Code, http.StatusAccepted, recorder.Body.String())
		}
	}

	var runCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM reconcile_runs WHERE idempotency_key = 'drift-report-1'`).Scan(&runCount); err != nil {
		t.Fatalf("query run count error = %v", err)
	}
	if runCount != 1 {
		t.Fatalf("run count = %d, want 1", runCount)
	}
	var eventCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM reconcile_events WHERE resource_id = 'resource_brew'`).Scan(&eventCount); err != nil {
		t.Fatalf("query event count error = %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("event count = %d, want 1", eventCount)
	}

	dashboard := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	dashboard.AddCookie(ownerCookie)
	dashboardRecorder := httptest.NewRecorder()
	router.ServeHTTP(dashboardRecorder, dashboard)
	if dashboardRecorder.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d; body=%s", dashboardRecorder.Code, http.StatusOK, dashboardRecorder.Body.String())
	}
	if !strings.Contains(dashboardRecorder.Body.String(), `"drifted":1`) {
		t.Fatalf("dashboard body = %s, want drifted metric 1", dashboardRecorder.Body.String())
	}
}

func TestAgentReport_heartbeatDesiredStateCommandsAndCommandAck(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, _ := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_agent", "mtn_agent")
	seedResource(t, db, "resource_brew", "package", "kubectl", 1)
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO agent_commands (id, machine_id, command_type, payload_json, idempotency_key, status, created_at) VALUES ('command_ack', 'machine_agent', 'repair_drift', '{}', 'ack-key', 'queued', ?)`,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert command error = %v", err)
	}

	heartbeat := httptest.NewRequest(http.MethodPost, "/api/agent/heartbeat", strings.NewReader(`{"agentVersion":"0.1.0"}`))
	heartbeat.Header.Set("Authorization", "Bearer mtn_agent")
	heartbeatRecorder := httptest.NewRecorder()
	router.ServeHTTP(heartbeatRecorder, heartbeat)
	if heartbeatRecorder.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status = %d, want %d; body=%s", heartbeatRecorder.Code, http.StatusNoContent, heartbeatRecorder.Body.String())
	}

	desired := httptest.NewRequest(http.MethodGet, "/api/agent/desired-state", http.NoBody)
	desired.Header.Set("Authorization", "Bearer mtn_agent")
	desiredRecorder := httptest.NewRecorder()
	router.ServeHTTP(desiredRecorder, desired)
	if desiredRecorder.Code != http.StatusOK || !strings.Contains(desiredRecorder.Body.String(), "resource_brew") {
		t.Fatalf("desired status = %d body=%s, want resource", desiredRecorder.Code, desiredRecorder.Body.String())
	}

	commands := httptest.NewRequest(http.MethodGet, "/api/agent/commands", http.NoBody)
	commands.Header.Set("Authorization", "Bearer mtn_agent")
	commandsRecorder := httptest.NewRecorder()
	router.ServeHTTP(commandsRecorder, commands)
	if commandsRecorder.Code != http.StatusOK || !strings.Contains(commandsRecorder.Body.String(), "command_ack") {
		t.Fatalf("commands status = %d body=%s, want queued command", commandsRecorder.Code, commandsRecorder.Body.String())
	}

	report := httptest.NewRequest(http.MethodPost, "/api/agent/reconcile-report", bytes.NewReader([]byte(`{"commandId":"command_ack","status":"dry_run_queued","events":[]}`)))
	report.Header.Set("Authorization", "Bearer mtn_agent")
	report.Header.Set("Idempotency-Key", "command-report-1")
	reportRecorder := httptest.NewRecorder()
	router.ServeHTTP(reportRecorder, report)
	if reportRecorder.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want %d; body=%s", reportRecorder.Code, http.StatusAccepted, reportRecorder.Body.String())
	}
	var commandStatus string
	if err := db.QueryRowContext(context.Background(), `SELECT status FROM agent_commands WHERE id = 'command_ack'`).Scan(&commandStatus); err != nil {
		t.Fatalf("query command status error = %v", err)
	}
	if commandStatus != "dry_run_queued" {
		t.Fatalf("command status = %s, want dry_run_queued", commandStatus)
	}
}

func seedMachineWithToken(t *testing.T, db *sql.DB, machineID string, token string) {
	t.Helper()
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO machines (id, name, os, arch, last_heartbeat_at, agent_version, created_at) VALUES (?, ?, 'darwin', 'arm64', ?, '0.1.0', ?)`,
		machineID,
		machineID,
		now,
		now,
	)
	if err != nil {
		t.Fatalf("insert machine error = %v", err)
	}
	_, err = db.ExecContext(
		context.Background(),
		`INSERT INTO machine_tokens (id, machine_id, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		"token_"+machineID,
		machineID,
		hashSecret(token),
		now,
	)
	if err != nil {
		t.Fatalf("insert token error = %v", err)
	}
}
