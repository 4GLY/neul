package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDashboard_whenFleetEmpty_returnsFirstMachineCTA(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"action":"create_pairing_code"`) {
		t.Fatalf("body = %s, want create_pairing_code empty CTA", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"kind":"secret"`) {
		t.Fatalf("body contains secret resource: %s", recorder.Body.String())
	}
}

func TestDashboard_computesMachineStatusFromHeartbeatAndResourceState(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachine(t, db, "machine_fresh", "fresh", now.Add(-4*time.Minute-59*time.Second))
	seedMachine(t, db, "machine_old", "old", now.Add(-5*time.Minute-time.Second))
	seedResource(t, db, "resource_pkg", "package", "brew:git", 3)
	seedReconcileEvent(t, db, "machine_fresh", "resource_pkg", "drifted", 3, 2)

	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"drifted":1`) {
		t.Fatalf("body = %s, want drifted metric 1", body)
	}
	if !strings.Contains(body, `"offline":1`) {
		t.Fatalf("body = %s, want offline metric 1", body)
	}
	if !strings.Contains(body, `"id":"machine_fresh"`) || !strings.Contains(body, `"status":"drifted"`) {
		t.Fatalf("body = %s, want fresh machine drifted", body)
	}
	if strings.Contains(body, `"machine_fresh","status":"offline"`) {
		t.Fatalf("body = %s, fresh machine must not be offline at 4m59s", body)
	}
}

func TestListMachines_andGetMachineReturnServerOwnedStatusAndEvents(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachine(t, db, "machine_logs", "logs", now.Add(-time.Minute))
	seedResource(t, db, "resource_dot", "dotfile", "~/.zshrc", 1)
	seedReconcileEvent(t, db, "machine_logs", "resource_dot", "blocked", 1, 0)

	list := httptest.NewRequest(http.MethodGet, "/api/machines", http.NoBody)
	list.AddCookie(cookie)
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRecorder.Code, http.StatusOK, listRecorder.Body.String())
	}
	if !strings.Contains(listRecorder.Body.String(), `"status":"blocked"`) {
		t.Fatalf("list body = %s, want blocked status", listRecorder.Body.String())
	}

	detail := httptest.NewRequest(http.MethodGet, "/api/machines/machine_logs", http.NoBody)
	detail.AddCookie(cookie)
	detailRecorder := httptest.NewRecorder()
	router.ServeHTTP(detailRecorder, detail)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%s", detailRecorder.Code, http.StatusOK, detailRecorder.Body.String())
	}
	if !strings.Contains(detailRecorder.Body.String(), `"events":[`) || !strings.Contains(detailRecorder.Body.String(), `"status":"blocked"`) {
		t.Fatalf("detail body = %s, want events for Open logs", detailRecorder.Body.String())
	}
}

func TestRepairDrift_requiresOwnerCookieAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachine(t, db, "machine_repair", "repair", now.Add(-time.Minute))
	seedResource(t, db, "resource_pkg", "package", "brew:git", 1)
	seedReconcileEvent(t, db, "machine_repair", "resource_pkg", "drifted", 1, 0)

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/machines/machine_repair/repair-drift", http.NoBody)
	unauthorized.Header.Set("Idempotency-Key", "repair-test-1")
	unauthorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRecorder.Code, http.StatusUnauthorized)
	}

	first := httptest.NewRequest(http.MethodPost, "/api/machines/machine_repair/repair-drift", http.NoBody)
	first.AddCookie(cookie)
	first.Header.Set("Idempotency-Key", "repair-test-1")
	firstRecorder := httptest.NewRecorder()
	router.ServeHTTP(firstRecorder, first)
	if firstRecorder.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d; body=%s", firstRecorder.Code, http.StatusAccepted, firstRecorder.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/api/machines/machine_repair/repair-drift", http.NoBody)
	second.AddCookie(cookie)
	second.Header.Set("Idempotency-Key", "repair-test-1")
	secondRecorder := httptest.NewRecorder()
	router.ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, want %d; body=%s", secondRecorder.Code, http.StatusAccepted, secondRecorder.Body.String())
	}
	if firstRecorder.Body.String() != secondRecorder.Body.String() {
		t.Fatalf("idempotent bodies differ: first=%s second=%s", firstRecorder.Body.String(), secondRecorder.Body.String())
	}

	var commandCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM agent_commands WHERE machine_id = ? AND command_type = 'repair_drift'`, "machine_repair").Scan(&commandCount); err != nil {
		t.Fatalf("query command count error = %v", err)
	}
	if commandCount != 1 {
		t.Fatalf("command count = %d, want 1", commandCount)
	}
}

func seedMachine(t *testing.T, db *sql.DB, id string, name string, lastHeartbeat time.Time) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO machines (id, name, os, arch, last_heartbeat_at, agent_version, created_at) VALUES (?, ?, 'darwin', 'arm64', ?, '0.1.0', ?)`,
		id,
		name,
		lastHeartbeat.Format(time.RFC3339Nano),
		lastHeartbeat.Add(-time.Minute).Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert machine error = %v", err)
	}
}

func seedResource(t *testing.T, db *sql.DB, id string, kind string, name string, desiredVersion int) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO resources (id, kind, name, spec_json, desired_version, created_at, updated_at) VALUES (?, ?, ?, '{}', ?, ?, ?)`,
		id,
		kind,
		name,
		desiredVersion,
		time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert resource error = %v", err)
	}
}

func seedReconcileEvent(t *testing.T, db *sql.DB, machineID string, resourceID string, status string, desiredVersion int, appliedVersion int) {
	t.Helper()
	runID := "run_" + machineID + "_" + resourceID
	createdAt := time.Date(2026, 6, 5, 12, 30, 0, 0, time.UTC).Format(time.RFC3339Nano)
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO reconcile_runs (id, machine_id, reason, idempotency_key, status, created_at) VALUES (?, ?, 'test', ?, 'finished', ?)`,
		runID,
		machineID,
		runID,
		createdAt,
	)
	if err != nil {
		t.Fatalf("insert run error = %v", err)
	}
	_, err = db.ExecContext(
		context.Background(),
		`INSERT INTO reconcile_events (id, run_id, resource_id, status, message, desired_version, applied_version, created_at) VALUES (?, ?, ?, ?, 'seed event', ?, ?, ?)`,
		"event_"+machineID+"_"+resourceID,
		runID,
		resourceID,
		status,
		desiredVersion,
		appliedVersion,
		createdAt,
	)
	if err != nil {
		t.Fatalf("insert event error = %v", err)
	}
}
