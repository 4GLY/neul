package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentReport_FinishedInSyncCommandEventUpdatesCommandAndDashboard(t *testing.T) {
	// Given
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_finished_sync", "mtn_finished_sync")
	seedResource(t, db, "resource_finished_sync", "package", "kubectl", 1)
	seedAgentCommand(t, db, "command_finished_sync", "machine_finished_sync", now)

	// When
	postAgentCommandReport(t, router, "mtn_finished_sync", "command-finished-sync-1", `{
		"commandId":"command_finished_sync",
		"status":"finished",
		"events":[{"resourceId":"resource_finished_sync","status":"in_sync","message":"kubectl applied","desiredVersion":1,"appliedVersion":1}]
	}`)

	// Then
	assertCommandStatus(t, db, "command_finished_sync", "finished")
	assertDashboardMachineState(t, router, cookie, "machine_finished_sync", "healthy", 0, 0, 0, 1, 1)
}

func TestAgentReport_FinishedBlockedCommandEventUpdatesDashboard(t *testing.T) {
	// Given
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_finished_blocked", "mtn_finished_blocked")
	seedResource(t, db, "resource_finished_blocked", "package", "kubectl", 1)
	seedAgentCommand(t, db, "command_finished_blocked", "machine_finished_blocked", now)

	// When
	postAgentCommandReport(t, router, "mtn_finished_blocked", "command-finished-blocked-1", `{
		"commandId":"command_finished_blocked",
		"status":"finished",
		"events":[{"resourceId":"resource_finished_blocked","status":"blocked","message":"brew_apply_failed: exit 1","desiredVersion":1,"appliedVersion":0}]
	}`)

	// Then
	assertCommandStatus(t, db, "command_finished_blocked", "finished")
	assertDashboardMachineState(t, router, cookie, "machine_finished_blocked", "blocked", 0, 0, 1, 1, 0)
}

func TestDashboard_FinishedPackageOnlyCommandDoesNotDoubleCountDotfilesAsBlocked(t *testing.T) {
	// Given
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_finished_package_only", "mtn_finished_package_only")
	seedResource(t, db, "resource_finished_package", "package", "kubectl", 1)
	seedResource(t, db, "resource_finished_dotfile", "dotfile", "~/.zshrc", 1)
	seedReconcileEventAt(t, db, "machine_finished_package_only", "resource_finished_dotfile", "blocked", 1, 0, now.Add(-2*time.Minute))
	seedAgentCommand(t, db, "command_finished_package_only", "machine_finished_package_only", now)

	// When
	postAgentCommandReport(t, router, "mtn_finished_package_only", "command-finished-package-only-1", `{
		"commandId":"command_finished_package_only",
		"status":"finished",
		"events":[{"resourceId":"resource_finished_package","status":"in_sync","message":"kubectl applied","desiredVersion":1,"appliedVersion":1}]
	}`)

	// Then
	assertDashboardMachineState(t, router, cookie, "machine_finished_package_only", "blocked", 0, 0, 1, 2, 1)
}

func TestAgentReport_FinishedCommandScopedAuditEventAcceptsEmptyResourceID(t *testing.T) {
	// Given
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_finished_audit", "mtn_finished_audit")
	seedAgentCommand(t, db, "command_finished_audit", "machine_finished_audit", now)

	// When
	postAgentCommandReport(t, router, "mtn_finished_audit", "command-finished-audit-1", `{
		"commandId":"command_finished_audit",
		"status":"finished",
		"events":[{"resourceId":"","status":"blocked","message":"resource_not_found:resource_missing","desiredVersion":0,"appliedVersion":0}]
	}`)

	// Then
	var resourceID sql.NullString
	var status string
	var message string
	err := db.QueryRowContext(
		context.Background(),
		`SELECT resource_id, status, message FROM reconcile_events WHERE message = 'resource_not_found:resource_missing'`,
	).Scan(&resourceID, &status, &message)
	if err != nil {
		t.Fatalf("query command-scoped event error = %v", err)
	}
	if resourceID.Valid {
		t.Fatalf("resource_id = %q, want NULL for command-scoped audit event", resourceID.String)
	}
	if status != "blocked" || message != "resource_not_found:resource_missing" {
		t.Fatalf("event = status:%s message:%s, want blocked resource_not_found audit", status, message)
	}

	report := storedAgentReport(t, db, "command-finished-audit-1")
	if len(report.Events) != 1 || report.Events[0].ResourceID != "" {
		t.Fatalf("stored report events = %+v, want one event with empty resourceId", report.Events)
	}
	machine := dashboardMachineByID(t, router, cookie, "machine_finished_audit")
	if machine.ResourceCount != 0 || machine.BlockedCount != 0 || machine.AppliedCount != 0 {
		t.Fatalf("dashboard counts = resource:%d blocked:%d applied:%d, want zero per-resource counts", machine.ResourceCount, machine.BlockedCount, machine.AppliedCount)
	}
}

func TestDashboard_FinishedLegacyUnsupportedAdapterEventsCountAsBlocked(t *testing.T) {
	// Given
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachine(t, db, "machine_finished_legacy", "finished-legacy", now.Add(-time.Minute))
	seedResource(t, db, "resource_finished_legacy", "package", "kubectl", 1)
	seedDashboardEvent(t, db, "machine_finished_legacy", "resource_finished_legacy", "unsupported_adapter", 1, 0, now.Add(-2*time.Minute))

	// When
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// Then
	if recorder.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body dashboardStateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v; body=%s", err, recorder.Body.String())
	}
	assertDashboardMachine(t, body, "machine_finished_legacy", "blocked", 0, 0, 1, 1, 0)
}

func TestAgentReport_FinishedCommandReportRepostDoesNotDuplicateEvents(t *testing.T) {
	// Given
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, _ := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_finished_idempotent", "mtn_finished_idempotent")
	seedResource(t, db, "resource_finished_idempotent", "package", "kubectl", 1)
	seedAgentCommand(t, db, "command_finished_idempotent", "machine_finished_idempotent", now)
	body := `{
		"commandId":"command_finished_idempotent",
		"status":"finished",
		"events":[{"resourceId":"resource_finished_idempotent","status":"in_sync","message":"kubectl applied","desiredVersion":1,"appliedVersion":1}]
	}`

	// When
	postAgentCommandReport(t, router, "mtn_finished_idempotent", "command-finished-idempotent-1", body)
	postAgentCommandReport(t, router, "mtn_finished_idempotent", "command-finished-idempotent-1", body)

	// Then
	var eventCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM reconcile_events WHERE resource_id = 'resource_finished_idempotent'`).Scan(&eventCount); err != nil {
		t.Fatalf("query event count error = %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("event count = %d, want 1 after idempotent repost", eventCount)
	}
	var runCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM reconcile_runs WHERE idempotency_key = 'command-finished-idempotent-1'`).Scan(&runCount); err != nil {
		t.Fatalf("query run count error = %v", err)
	}
	if runCount != 1 {
		t.Fatalf("run count = %d, want 1 after idempotent repost", runCount)
	}
	assertCommandStatus(t, db, "command_finished_idempotent", "finished")
}

type storedCommandReport struct {
	Events []struct {
		ResourceID string `json:"resourceId"`
	} `json:"events"`
}

func seedAgentCommand(t *testing.T, db *sql.DB, commandID string, machineID string, now time.Time) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO agent_commands (id, machine_id, command_type, payload_json, idempotency_key, status, created_at) VALUES (?, ?, 'repair_drift', '{"resourceIds":[]}', ?, 'queued', ?)`,
		commandID,
		machineID,
		"seed-"+commandID,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert command error = %v", err)
	}
}

func postAgentCommandReport(t *testing.T, router http.Handler, token string, idempotencyKey string, body string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/reconcile-report", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
}

func assertCommandStatus(t *testing.T, db *sql.DB, commandID string, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(), `SELECT status FROM agent_commands WHERE id = ?`, commandID).Scan(&got); err != nil {
		t.Fatalf("query command status error = %v", err)
	}
	if got != want {
		t.Fatalf("command status = %s, want %s", got, want)
	}
}

func storedAgentReport(t *testing.T, db *sql.DB, idempotencyKey string) storedCommandReport {
	t.Helper()
	var reportJSON string
	if err := db.QueryRowContext(context.Background(), `SELECT report_json FROM agent_reports WHERE idempotency_key = ?`, idempotencyKey).Scan(&reportJSON); err != nil {
		t.Fatalf("query stored agent report error = %v", err)
	}
	var report storedCommandReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		t.Fatalf("stored report JSON error = %v; report=%s", err, reportJSON)
	}
	return report
}

func dashboardMachineByID(t *testing.T, router http.Handler, cookie *http.Cookie, machineID string) dashboardStateMachineRecord {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body dashboardStateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("dashboard JSON error = %v; body=%s", err, recorder.Body.String())
	}
	for _, machine := range body.Machines {
		if machine.ID == machineID {
			return machine
		}
	}
	t.Fatalf("machine %s not found in %+v", machineID, body.Machines)
	return dashboardStateMachineRecord{}
}
