package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUp_whenNoConfig_printsLoginGuidance(t *testing.T) {
	var stdout strings.Builder

	err := Run([]string{"up", "--config", filepath.Join(t.TempDir(), "config.json")}, &stdout, &stdout)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"아직 Neul fleet에 연결되지 않았습니다", "neul login --server <origin>"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "mtn_") || strings.Contains(stdout.String(), "pair_") {
		t.Fatalf("stdout leaked token-like value: %q", stdout.String())
	}
}

func TestUp_whenLaunchAgentInstallFails_returnsAgentNotRunning(t *testing.T) {
	restoreRuntime := overrideAgentServiceGOOS(t, "darwin")
	defer restoreRuntime()
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	dir := t.TempDir()
	configPath := writeTestConfigInDir(t, dir)
	var stdout strings.Builder

	err := Run([]string{
		"up",
		"--config", configPath,
		"--agent-binary", filepath.Join(dir, "missing-neul-agent"),
		"--plist", filepath.Join(dir, "agent.plist"),
		"--status", filepath.Join(dir, "status.json"),
	}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want agent_not_running")
	}
	if !strings.Contains(err.Error(), "agent_not_running") || !strings.Contains(stdout.String(), "agent_not_running") {
		t.Fatalf("error = %v stdout = %q, want agent_not_running", err, stdout.String())
	}
}

func TestUp_whenRunLoopHeartbeatFresh_reportsConnected(t *testing.T) {
	plan := newDarwinInstallPlan(t)
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	restoreLaunchctl := stubLaunchctlCommand(t, launchctlNotLoadedThenOK)
	defer restoreLaunchctl()
	writeTestFile(t, plan.statusPath, `{"mode":"run_loop","lastHeartbeatAt":"2026-06-19T08:00:01Z","lastError":null}`)
	var stdout strings.Builder

	err := Run(append([]string{"up"}, plan.upArgs()...), &stdout, &stdout)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "connected") {
		t.Fatalf("stdout = %q, want connected", stdout.String())
	}
}

func TestUp_whenFreshAuthFailureAtDeadline_returnsAuthInvalid(t *testing.T) {
	plan := newDarwinInstallPlan(t)
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	restoreLaunchctl := stubLaunchctlCommand(t, launchctlNotLoadedThenOK)
	defer restoreLaunchctl()
	writeTestFile(t, plan.statusPath, `{"mode":"run_loop","lastHeartbeatAttempt":"2026-06-19T08:00:01Z","lastError":{"kind":"auth_failure","message":"invalid"}}`)
	var stdout strings.Builder

	err := Run(append([]string{"up"}, plan.upArgs()...), &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want auth_invalid")
	}
	if !strings.Contains(err.Error(), "auth_invalid") || !strings.Contains(stdout.String(), "auth_invalid") {
		t.Fatalf("error = %v stdout = %q, want auth_invalid", err, stdout.String())
	}
}

func TestUp_whenStaleErrorOnly_returnsLocalHeartbeatMissing(t *testing.T) {
	plan := newDarwinInstallPlan(t)
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	restoreLaunchctl := stubLaunchctlCommand(t, launchctlNotLoadedThenOK)
	defer restoreLaunchctl()
	writeTestFile(t, plan.statusPath, `{"mode":"run_loop","lastHeartbeatAttempt":"2026-06-19T07:59:59Z","lastError":{"kind":"auth_failure","message":"old"}}`)
	var stdout strings.Builder

	err := Run(append([]string{"up"}, plan.upArgs()...), &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want local_heartbeat_missing")
	}
	if !strings.Contains(err.Error(), "local_heartbeat_missing") || !strings.Contains(stdout.String(), "local_heartbeat_missing") {
		t.Fatalf("error = %v stdout = %q, want local_heartbeat_missing", err, stdout.String())
	}
}

func TestUp_whenConnectOnceReceiptFresh_doesNotReportConnected(t *testing.T) {
	plan := newDarwinInstallPlan(t)
	restoreWait := overrideUpWaitForTest(t, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC), 0)
	defer restoreWait()
	restoreLaunchctl := stubLaunchctlCommand(t, launchctlNotLoadedThenOK)
	defer restoreLaunchctl()
	writeTestFile(t, plan.statusPath, `{"mode":"connect_once","lastHeartbeatAt":"2026-06-19T08:00:01Z","lastError":null}`)
	var stdout strings.Builder

	err := Run(append([]string{"up"}, plan.upArgs()...), &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want local_heartbeat_missing")
	}
	if strings.Contains(stdout.String(), "connected") {
		t.Fatalf("stdout = %q, should not report connected", stdout.String())
	}
	if !strings.Contains(err.Error(), "local_heartbeat_missing") {
		t.Fatalf("error = %v, want local_heartbeat_missing", err)
	}
}

func (p darwinInstallPlan) upArgs() []string {
	return []string{
		"--config", p.configPath,
		"--agent-binary", p.agentBinary,
		"--plist", p.plistPath,
		"--status", p.statusPath,
		"--log", p.logPath,
	}
}

func launchctlNotLoadedThenOK(command []string) ([]byte, error) {
	if len(command) >= 2 && command[1] == "print" {
		return []byte("Could not find service"), errors.New("exit status 113")
	}
	return nil, nil
}

func overrideUpWaitForTest(t *testing.T, now time.Time, timeout time.Duration) func() {
	t.Helper()
	previousNow := upNow
	previousSleep := upSleep
	previousTimeout := upWaitTimeout
	upNow = func() time.Time {
		return now
	}
	upSleep = func(time.Duration) {}
	upWaitTimeout = timeout
	return func() {
		upNow = previousNow
		upSleep = previousSleep
		upWaitTimeout = previousTimeout
	}
}
