package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteStatus_whenRunLoopMode_writesModeAndAttempt(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)

	err := writeStatus(statusWriteOptions{
		Path:                 statusPath,
		Mode:                 "run_loop",
		LastHeartbeatAttempt: now,
		LastError:            &StatusError{Kind: "auth_failure", Message: "invalid token"},
	})

	if err != nil {
		t.Fatalf("writeStatus() error = %v", err)
	}
	got := readStatusTestReceipt(t, statusPath)
	if got.Mode != "run_loop" {
		t.Fatalf("Mode = %q, want run_loop", got.Mode)
	}
	if got.LastHeartbeatAttempt != now.Format(time.RFC3339Nano) {
		t.Fatalf("LastHeartbeatAttempt = %q, want %q", got.LastHeartbeatAttempt, now.Format(time.RFC3339Nano))
	}
	if got.LastError == nil || got.LastError.Kind != "auth_failure" {
		t.Fatalf("LastError = %+v, want auth_failure", got.LastError)
	}
}

func TestWriteStatus_whenConnectOnceMode_writesDiagnosticMode(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	now := time.Date(2026, 6, 19, 8, 1, 0, 0, time.UTC)

	err := writeStatus(statusWriteOptions{
		Path:                 statusPath,
		Mode:                 "connect_once",
		LastHeartbeatAttempt: now,
		LastHeartbeatAt:      now,
	})

	if err != nil {
		t.Fatalf("writeStatus() error = %v", err)
	}
	got := readStatusTestReceipt(t, statusPath)
	if got.Mode != "connect_once" {
		t.Fatalf("Mode = %q, want connect_once", got.Mode)
	}
	if got.LastHeartbeatAttempt != now.Format(time.RFC3339Nano) {
		t.Fatalf("LastHeartbeatAttempt = %q, want %q", got.LastHeartbeatAttempt, now.Format(time.RFC3339Nano))
	}
	if got.LastHeartbeatAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("LastHeartbeatAt = %q, want %q", got.LastHeartbeatAt, now.Format(time.RFC3339Nano))
	}
}

func TestStatusReceipt_whenTickSucceeds_writesMachineStatusWithoutToken(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")
	server := newStatusTestServer(t, nil)
	defer server.Close()
	client := New(Config{ServerURL: server.URL, MachineID: "machine_status", MachineToken: "mtn_secret"})

	err := client.TickWithStatus(context.Background(), statusPath)

	if err != nil {
		t.Fatalf("TickWithStatus() error = %v", err)
	}
	status := readStatusTestReceipt(t, statusPath)
	if status.MachineID != "machine_status" || status.ServerURL != server.URL {
		t.Fatalf("status = %+v, want machine/server metadata", status)
	}
	if status.LastHeartbeatAttempt == "" || status.LastSuccess == "" || status.LastHeartbeatAt == "" {
		t.Fatalf("status = %+v, want attempt and success timestamps", status)
	}
	if status.LastError != nil {
		t.Fatalf("lastError = %+v, want nil after success", status.LastError)
	}
	body := readStatusTestBody(t, statusPath)
	if strings.Contains(body, "machineToken") || strings.Contains(body, "mtn_secret") {
		t.Fatalf("status leaked token material: %s", body)
	}
}

func TestStatusReceipt_whenTickFails_writesClassifiedErrorWithoutMisleadingSuccess(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"lastSuccess":"2026-01-01T00:00:00Z","lastHeartbeatAt":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := newStatusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/heartbeat" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		t.Fatalf("unexpected path after failed heartbeat: %s", r.URL.Path)
	})
	defer server.Close()
	client := New(Config{ServerURL: server.URL, MachineID: "machine_status", MachineToken: "mtn_secret"})

	err := client.TickWithStatus(context.Background(), statusPath)

	if err == nil {
		t.Fatal("TickWithStatus() error = nil, want failure")
	}
	status := readStatusTestReceipt(t, statusPath)
	if status.LastSuccess != "" || status.LastHeartbeatAt != "" {
		t.Fatalf("status = %+v, want stale success cleared on failure", status)
	}
	if status.LastError == nil || status.LastError.Kind != "auth_failure" || !strings.Contains(status.LastError.Message, "status 401") {
		t.Fatalf("lastError = %+v, want classified auth failure", status.LastError)
	}
	body := readStatusTestBody(t, statusPath)
	if strings.Contains(body, "machineToken") || strings.Contains(body, "mtn_secret") {
		t.Fatalf("status leaked token material: %s", body)
	}
}

func TestRunLoopStatusReceipt_whenTicksRecover_updatesFailureThenSuccess(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var heartbeatCount atomic.Int64
	server := newStatusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			if heartbeatCount.Add(1) == 1 {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[]}`))
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	defer server.Close()
	client := New(Config{
		ServerURL:         server.URL,
		MachineID:         "machine_loop_status",
		MachineToken:      "mtn_loop_status",
		HeartbeatInterval: time.Millisecond,
	})

	err := client.Run(ctx, RunOptions{
		StatusPath: statusPath,
		Delay: func(ctx context.Context, delay time.Duration) error {
			if heartbeatCount.Load() == 2 {
				cancel()
			}
			return immediateDelay(ctx, delay)
		},
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	status := readStatusTestReceipt(t, statusPath)
	if status.Mode != "run_loop" {
		t.Fatalf("Mode = %q, want run_loop", status.Mode)
	}
	if status.LastSuccess == "" || status.LastError != nil {
		t.Fatalf("status = %+v, want recovery success with cleared error", status)
	}
}

type statusTestReceipt struct {
	Mode                 string           `json:"mode"`
	MachineID            string           `json:"machineId"`
	ServerURL            string           `json:"serverURL"`
	LastHeartbeatAttempt string           `json:"lastHeartbeatAttempt"`
	LastSuccess          string           `json:"lastSuccess"`
	LastHeartbeatAt      string           `json:"lastHeartbeatAt"`
	LastError            *statusTestError `json:"lastError"`
}

type statusTestError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func newStatusTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	if handler != nil {
		return httptest.NewServer(handler)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[]}`))
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func readStatusTestReceipt(t *testing.T, path string) statusTestReceipt {
	t.Helper()
	var status statusTestReceipt
	if err := json.Unmarshal([]byte(readStatusTestBody(t, path)), &status); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return status
}

func readStatusTestBody(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(body)
}
