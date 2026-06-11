package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDashboard_returnsAllMachineStatesAndDocumentedMetricsFromSQLite(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	seedMachine(t, db, "machine_healthy", "healthy", now.Add(-time.Minute))
	seedMachine(t, db, "machine_pending", "pending", now.Add(-time.Minute))
	seedMachine(t, db, "machine_drifted", "drifted", now.Add(-time.Minute))
	seedMachine(t, db, "machine_blocked", "blocked", now.Add(-time.Minute))
	seedMachine(t, db, "machine_offline", "offline", now.Add(-6*time.Minute))
	seedMachine(t, db, "machine_unknown", "unknown", now.Add(-time.Minute))
	seedMachine(t, db, "machine_started", "started", now.Add(-time.Minute))
	seedMachine(t, db, "machine_tie", "tie", now.Add(-time.Minute))
	seedMachine(t, db, "machine_in_sync_mismatch", "in-sync-mismatch", now.Add(-time.Minute))
	seedMachine(t, db, "machine_pending_match", "pending-match", now.Add(-time.Minute))

	seedResource(t, db, "resource_healthy", "package", "healthy", 2)
	seedResource(t, db, "resource_pending", "package", "pending", 3)
	seedResource(t, db, "resource_drifted", "package", "drifted", 1)
	seedResource(t, db, "resource_blocked", "package", "blocked", 1)
	seedResource(t, db, "resource_offline", "package", "offline", 1)
	seedResource(t, db, "resource_started", "package", "started", 1)
	seedResource(t, db, "resource_in_sync_mismatch", "package", "in-sync-mismatch", 3)
	seedResource(t, db, "resource_pending_match", "package", "pending-match", 1)

	seedDashboardEvent(t, db, "machine_healthy", "resource_healthy", "pending", 2, 1, now.Add(-3*time.Minute))
	seedDashboardEvent(t, db, "machine_healthy", "resource_healthy", "in_sync", 2, 2, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_pending", "resource_pending", "pending", 3, 2, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_drifted", "resource_drifted", "drifted", 1, 0, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_blocked", "resource_blocked", "blocked", 1, 0, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_offline", "resource_offline", "drifted", 1, 0, now.Add(-2*time.Minute))
	seedDashboardEventWithRunStatus(t, db, "machine_started", "resource_started", "started", "blocked", 1, 0, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_tie", "resource_healthy", "pending", 2, 1, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_tie", "resource_healthy", "in_sync", 2, 2, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_in_sync_mismatch", "resource_in_sync_mismatch", "in_sync", 3, 2, now.Add(-2*time.Minute))
	seedDashboardEvent(t, db, "machine_pending_match", "resource_pending_match", "pending", 1, 1, now.Add(-2*time.Minute))

	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body dashboardStateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v; body=%s", err, recorder.Body.String())
	}
	wantMetrics := map[string]int{
		"total":   10,
		"healthy": 3,
		"pending": 2,
		"drifted": 1,
		"blocked": 1,
		"unknown": 2,
		"offline": 1,
	}
	for key, want := range wantMetrics {
		if body.Metrics[key] != want {
			t.Fatalf("metrics[%s] = %d, want %d; body=%s", key, body.Metrics[key], want, recorder.Body.String())
		}
	}
	if body.Metrics["total"] != len(body.Machines) {
		t.Fatalf("metrics total = %d, machine count = %d", body.Metrics["total"], len(body.Machines))
	}
	assertDashboardMachine(t, body, "machine_healthy", "healthy", 0, 0, 0, 1, 1)
	assertDashboardMachine(t, body, "machine_pending", "pending", 0, 1, 0, 1, 0)
	assertDashboardMachine(t, body, "machine_drifted", "drifted", 1, 0, 0, 1, 0)
	assertDashboardMachine(t, body, "machine_blocked", "blocked", 0, 0, 1, 1, 0)
	assertDashboardMachine(t, body, "machine_offline", "offline", 1, 0, 0, 1, 0)
	assertDashboardMachine(t, body, "machine_unknown", "unknown", 0, 0, 0, 0, 0)
	assertDashboardMachine(t, body, "machine_started", "unknown", 0, 0, 0, 0, 0)
	assertDashboardMachine(t, body, "machine_tie", "healthy", 0, 0, 0, 1, 1)
	assertDashboardMachine(t, body, "machine_in_sync_mismatch", "healthy", 0, 0, 0, 1, 1)
	assertDashboardMachine(t, body, "machine_pending_match", "pending", 0, 1, 0, 1, 0)
}

func TestDashboardCounts_ignoreEventsWithoutCurrentResource(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	seedMachine(t, db, "machine_orphan", "orphan", now.Add(-time.Minute))
	seedDashboardOrphanEvent(t, db, "machine_orphan", "resource_deleted", "in_sync", now.Add(-2*time.Minute))

	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", http.NoBody)
	countsByMachine, err := queryMachineCountsByMachine(request, db)
	if err != nil {
		t.Fatalf("queryMachineCountsByMachine() error = %v", err)
	}
	counts := countsByMachine["machine_orphan"]
	if counts.ResourceCount != 0 || counts.AppliedCount != 0 || counts.Pending != 0 {
		t.Fatalf("orphan counts = %+v, want zero counted resources", counts)
	}
}

func TestDashboard_marksDesiredStateEditsPendingUntilCurrentReport(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_pending_desired", "mtn_pending_desired")
	resourceID := createPackageResource(t, router, cookie)

	assertDashboardMachineState(t, router, cookie, "machine_pending_desired", "pending", 1, 0, 0, 1, 0)
	postAgentReport(t, router, "mtn_pending_desired", "pending-in-sync-1", resourceID, "in_sync", 1, 1)
	assertDashboardMachineState(t, router, cookie, "machine_pending_desired", "healthy", 0, 0, 0, 1, 1)

	patch := httptest.NewRequest(
		http.MethodPatch,
		"/api/resources/"+resourceID,
		strings.NewReader(`{"desiredVersion":"1.2.3"}`),
	)
	patch.AddCookie(cookie)
	patchRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchRecorder, patch)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want %d; body=%s", patchRecorder.Code, http.StatusOK, patchRecorder.Body.String())
	}
	assertDashboardMachineState(t, router, cookie, "machine_pending_desired", "pending", 1, 0, 0, 1, 0)
	postAgentReport(t, router, "mtn_pending_desired", "pending-stale-1", resourceID, "in_sync", 1, 1)
	assertDashboardMachineState(t, router, cookie, "machine_pending_desired", "pending", 1, 0, 0, 1, 0)
	postAgentReport(t, router, "mtn_pending_desired", "pending-blocked-2", resourceID, "blocked", 2, 0)
	assertDashboardMachineState(t, router, cookie, "machine_pending_desired", "blocked", 0, 0, 1, 1, 0)
}

func TestDashboard_marksExistingDesiredStatePendingForNewlyEnrolledMachine(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	createPackageResource(t, router, cookie)
	claimed := claimMachineThroughPairing(t, router, cookie)
	heartbeat := httptest.NewRequest(http.MethodPost, "/api/agent/heartbeat", strings.NewReader(`{"agentVersion":"0.1.0"}`))
	heartbeat.Header.Set("Authorization", "Bearer "+claimed.machineToken)
	heartbeatRecorder := httptest.NewRecorder()
	router.ServeHTTP(heartbeatRecorder, heartbeat)
	if heartbeatRecorder.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status = %d, want %d; body=%s", heartbeatRecorder.Code, http.StatusNoContent, heartbeatRecorder.Body.String())
	}

	assertDashboardMachineState(t, router, cookie, claimed.machineID, "pending", 1, 0, 0, 1, 0)
}

type dashboardStateResponse struct {
	Metrics  map[string]int                `json:"metrics"`
	Machines []dashboardStateMachineRecord `json:"machines"`
}

type dashboardStateMachineRecord struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	LastReconcileAt string `json:"lastReconcileAt"`
	DriftCount      int    `json:"driftCount"`
	PendingCount    int    `json:"pendingCount"`
	BlockedCount    int    `json:"blockedCount"`
	ResourceCount   int    `json:"resourceCount"`
	AppliedCount    int    `json:"appliedCount"`
}

func assertDashboardMachine(
	t *testing.T,
	body dashboardStateResponse,
	id string,
	status string,
	driftCount int,
	pendingCount int,
	blockedCount int,
	resourceCount int,
	appliedCount int,
) {
	t.Helper()
	for _, machine := range body.Machines {
		if machine.ID != id {
			continue
		}
		if machine.Status != status {
			t.Fatalf("%s status = %s, want %s", id, machine.Status, status)
		}
		if machine.DriftCount != driftCount || machine.PendingCount != pendingCount || machine.BlockedCount != blockedCount {
			t.Fatalf("%s counts = drift:%d pending:%d blocked:%d, want drift:%d pending:%d blocked:%d", id, machine.DriftCount, machine.PendingCount, machine.BlockedCount, driftCount, pendingCount, blockedCount)
		}
		if machine.ResourceCount != resourceCount || machine.AppliedCount != appliedCount {
			t.Fatalf("%s progress = %d/%d, want %d/%d", id, machine.AppliedCount, machine.ResourceCount, appliedCount, resourceCount)
		}
		if resourceCount == 0 {
			if machine.LastReconcileAt != "" {
				t.Fatalf("%s LastReconcileAt = %s, want empty", id, machine.LastReconcileAt)
			}
			return
		}
		if _, err := time.Parse(time.RFC3339Nano, machine.LastReconcileAt); err != nil {
			t.Fatalf("%s LastReconcileAt = %s, want RFC3339Nano: %v", id, machine.LastReconcileAt, err)
		}
		return
	}
	t.Fatalf("machine %s not found in %+v", id, body.Machines)
}

func assertDashboardMachineState(
	t *testing.T,
	router http.Handler,
	cookie *http.Cookie,
	id string,
	status string,
	pendingCount int,
	driftCount int,
	blockedCount int,
	resourceCount int,
	appliedCount int,
) {
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
		t.Fatalf("Unmarshal() error = %v; body=%s", err, recorder.Body.String())
	}
	assertDashboardMachine(t, body, id, status, driftCount, pendingCount, blockedCount, resourceCount, appliedCount)
}

func postAgentReport(t *testing.T, router http.Handler, token string, idempotencyKey string, resourceID string, status string, desiredVersion int, appliedVersion int) {
	t.Helper()
	body := `{"events":[{"resourceId":"` + resourceID + `","status":"` + status + `","message":"brew check","desiredVersion":` + strconv.Itoa(desiredVersion) + `,"appliedVersion":` + strconv.Itoa(appliedVersion) + `}]}`
	request := httptest.NewRequest(http.MethodPost, "/api/agent/drift-report", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
}

type claimedMachine struct {
	machineID    string
	machineToken string
}

func claimMachineThroughPairing(t *testing.T, router http.Handler, cookie *http.Cookie) claimedMachine {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/pair/init", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("pair init status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var pair struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &pair); err != nil {
		t.Fatalf("pair response JSON error = %v", err)
	}
	claim := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/claim",
		strings.NewReader(`{"code":"`+pair.Code+`","machine":{"name":"machine_late","os":"darwin","arch":"arm64","agentVersion":"0.1.0"}}`),
	)
	claimRecorder := httptest.NewRecorder()
	router.ServeHTTP(claimRecorder, claim)
	if claimRecorder.Code != http.StatusCreated {
		t.Fatalf("pair claim status = %d, want %d; body=%s", claimRecorder.Code, http.StatusCreated, claimRecorder.Body.String())
	}
	var body struct {
		MachineID    string `json:"machineId"`
		MachineToken string `json:"machineToken"`
	}
	if err := json.Unmarshal(claimRecorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("claim response JSON error = %v", err)
	}
	if body.MachineID == "" || body.MachineToken == "" {
		t.Fatalf("claim body = %+v, want machine id/token", body)
	}
	return claimedMachine{machineID: body.MachineID, machineToken: body.MachineToken}
}

func seedDashboardEvent(t *testing.T, db *sql.DB, machineID string, resourceID string, status string, desiredVersion int, appliedVersion int, createdAt time.Time) {
	t.Helper()
	seedDashboardEventWithRunStatus(t, db, machineID, resourceID, "finished", status, desiredVersion, appliedVersion, createdAt)
}

func seedDashboardEventWithRunStatus(t *testing.T, db *sql.DB, machineID string, resourceID string, runStatus string, eventStatus string, desiredVersion int, appliedVersion int, createdAt time.Time) {
	t.Helper()
	runID := "run_" + machineID + "_" + resourceID + "_" + runStatus + "_" + eventStatus + "_" + strconv.Itoa(appliedVersion) + "_" + createdAt.Format("150405")
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO reconcile_runs (id, machine_id, reason, idempotency_key, status, created_at, finished_at) VALUES (?, ?, 'test', ?, ?, ?, ?)`,
		runID,
		machineID,
		runID,
		runStatus,
		createdAt.Format(time.RFC3339Nano),
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert run error = %v", err)
	}
	_, err = db.ExecContext(
		context.Background(),
		`INSERT INTO reconcile_events (id, run_id, resource_id, status, message, desired_version, applied_version, created_at) VALUES (?, ?, ?, ?, 'seed event', ?, ?, ?)`,
		"event_"+runID,
		runID,
		resourceID,
		eventStatus,
		desiredVersion,
		appliedVersion,
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert event error = %v", err)
	}
}

func seedDashboardOrphanEvent(t *testing.T, db *sql.DB, machineID string, resourceID string, eventStatus string, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn() error = %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys error = %v", err)
	}
	runID := "run_" + machineID + "_" + resourceID + "_" + eventStatus
	createdAtText := createdAt.Format(time.RFC3339Nano)
	_, err = conn.ExecContext(
		ctx,
		`INSERT INTO reconcile_runs (id, machine_id, reason, idempotency_key, status, created_at, finished_at) VALUES (?, ?, 'test', ?, 'finished', ?, ?)`,
		runID,
		machineID,
		runID,
		createdAtText,
		createdAtText,
	)
	if err != nil {
		t.Fatalf("insert orphan run error = %v", err)
	}
	_, err = conn.ExecContext(
		ctx,
		`INSERT INTO reconcile_events (id, run_id, resource_id, status, message, desired_version, applied_version, created_at) VALUES (?, ?, ?, ?, 'orphan event', 1, 1, ?)`,
		"event_"+runID,
		runID,
		resourceID,
		eventStatus,
		createdAtText,
	)
	if err != nil {
		t.Fatalf("insert orphan event error = %v", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys error = %v", err)
	}
}
