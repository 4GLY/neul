package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUp_whenConfigExists_doesNotClaimPairOrOverwriteConfig(t *testing.T) {
	var pairClaims atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pair/claim" {
			pairClaims.Add(1)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	plan := newDarwinInstallPlan(t)
	config := Config{ServerURL: server.URL, MachineID: "machine_1", MachineToken: "mtn_secret"}
	if err := writeConfig(plan.configPath, config); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	before, err := os.ReadFile(plan.configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	restoreLaunchctl := stubLaunchctlCommand(t, launchctlNotLoadedThenOK)
	defer restoreLaunchctl()
	writeTestFile(t, plan.statusPath, `{"mode":"run_loop","lastHeartbeatAt":"2026-06-19T08:00:01Z","lastError":null}`)
	var stdout strings.Builder

	err = Run(append([]string{"up"}, plan.upArgs()...), &stdout, &stdout)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if pairClaims.Load() != 0 {
		t.Fatalf("pair claims = %d, want 0", pairClaims.Load())
	}
	after, err := os.ReadFile(plan.configPath)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("config changed from %q to %q", before, after)
	}
}

func TestUp_whenFreshConnectionFailureAtDeadline_returnsServerUnreachable(t *testing.T) {
	assertUpFreshErrorMapsTo(t, "connection_failure", "server_unreachable")
}

func TestUp_whenFreshServerFailureAtDeadline_returnsServerError(t *testing.T) {
	assertUpFreshErrorMapsTo(t, "server_failure", "server_error")
}

func TestUp_whenFreshRateLimitedAtDeadline_returnsRateLimited(t *testing.T) {
	assertUpFreshErrorMapsTo(t, "rate_limited", "rate_limited")
}

func assertUpFreshErrorMapsTo(t *testing.T, kind string, want string) {
	t.Helper()
	plan := newDarwinInstallPlan(t)
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	restoreLaunchctl := stubLaunchctlCommand(t, launchctlNotLoadedThenOK)
	defer restoreLaunchctl()
	writeTestFile(t, plan.statusPath, `{"mode":"run_loop","lastHeartbeatAttempt":"2026-06-19T08:00:01Z","lastError":{"kind":"`+kind+`","message":"fresh"}}`)
	var stdout strings.Builder

	err := Run(append([]string{"up"}, plan.upArgs()...), &stdout, &stdout)

	if err == nil {
		t.Fatalf("Run() error = nil, want %s", want)
	}
	if !strings.Contains(err.Error(), want) || !strings.Contains(stdout.String(), want) {
		t.Fatalf("error = %v stdout = %q, want %s", err, stdout.String(), want)
	}
}
