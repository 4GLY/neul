package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRepairDrift_queuesSelectedDriftedResource_withoutMutatingDesiredState(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachine(t, db, "machine_select", "select", now.Add(-time.Minute))
	seedResource(t, db, "resource_selected", "package", "brew:git", 3)
	seedResource(t, db, "resource_other", "package", "brew:rg", 5)
	seedReconcileEvent(t, db, "machine_select", "resource_selected", "drifted", 3, 2)
	seedReconcileEvent(t, db, "machine_select", "resource_other", "drifted", 5, 4)

	request := httptest.NewRequest(http.MethodPost, "/api/machines/machine_select/repair-drift", strings.NewReader(`{"resourceIds":["resource_selected"]}`))
	request.AddCookie(cookie)
	request.Header.Set("Idempotency-Key", "repair-selected-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	var payloadJSON string
	if err := db.QueryRowContext(context.Background(), `SELECT payload_json FROM agent_commands WHERE machine_id = ? AND command_type = 'repair_drift'`, "machine_select").Scan(&payloadJSON); err != nil {
		t.Fatalf("query command payload error = %v", err)
	}
	var payload struct {
		ResourceIDs []string `json:"resourceIds"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("payload JSON error = %v", err)
	}
	if len(payload.ResourceIDs) != 1 || payload.ResourceIDs[0] != "resource_selected" {
		t.Fatalf("payload resourceIds = %v, want selected drifted resource only", payload.ResourceIDs)
	}
	var desiredVersion int
	if err := db.QueryRowContext(context.Background(), `SELECT desired_version FROM resources WHERE id = ?`, "resource_selected").Scan(&desiredVersion); err != nil {
		t.Fatalf("query desired version error = %v", err)
	}
	if desiredVersion != 3 {
		t.Fatalf("desired version = %d, want unchanged 3", desiredVersion)
	}
}

func TestRepairDrift_rejectsSelectedResourceThatIsNoLongerDrifted(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachine(t, db, "machine_clean_select", "clean select", now.Add(-time.Minute))
	seedResource(t, db, "resource_clean", "package", "brew:git", 1)
	seedReconcileEvent(t, db, "machine_clean_select", "resource_clean", "in_sync", 1, 1)

	request := httptest.NewRequest(http.MethodPost, "/api/machines/machine_clean_select/repair-drift", strings.NewReader(`{"resourceIds":["resource_clean"]}`))
	request.AddCookie(cookie)
	request.Header.Set("Idempotency-Key", "repair-clean-select-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	var commandCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM agent_commands WHERE machine_id = ?`, "machine_clean_select").Scan(&commandCount); err != nil {
		t.Fatalf("query command count error = %v", err)
	}
	if commandCount != 0 {
		t.Fatalf("command count = %d, want 0", commandCount)
	}
}
