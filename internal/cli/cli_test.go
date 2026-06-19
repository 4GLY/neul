package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitPair_claimsMachineAndStoresConfig0600(t *testing.T) {
	var claimBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pair/claim" {
			t.Fatalf("path = %s, want /api/pair/claim", r.URL.Path)
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error = %v", err)
		}
		claimBody = string(bodyBytes)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"machineId":"machine_1","machineToken":"mtn_secret"}`))
	}))
	defer server.Close()
	configDir := t.TempDir()
	var stdout strings.Builder
	err := Run([]string{"init", "--pair", "pair_abc", "--server", server.URL, "--config-dir", configDir}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(claimBody, `"code":"pair_abc"`) {
		t.Fatalf("claim body = %s, want pair code", claimBody)
	}
	if !strings.Contains(stdout.String(), "Machine paired") {
		t.Fatalf("stdout = %s, want Machine paired", stdout.String())
	}
	if strings.Contains(stdout.String(), "mtn_secret") {
		t.Fatalf("stdout leaked token: %s", stdout.String())
	}
	configPath := filepath.Join(configDir, "config.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config stat error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 0600", info.Mode().Perm())
	}
	var config Config
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if config.MachineID != "machine_1" || config.MachineToken != "mtn_secret" {
		t.Fatalf("config = %+v, want machine id and token", config)
	}
}

func TestInitPair_whenServerRejectsCodeReportsClearFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":"pairing_code_expired","message":"Pairing code expired."}}`))
	}))
	defer server.Close()
	var stderr strings.Builder
	err := Run([]string{"init", "--pair", "expired", "--server", server.URL, "--config-dir", t.TempDir()}, &stderr, &stderr)
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "pairing code expired") {
		t.Fatalf("error = %v, want pairing code expired", err)
	}
}

func TestAgentInstallDryRun_printsPreviewWithoutWritingService(t *testing.T) {
	configPath := writeTestConfig(t)
	var stdout strings.Builder
	err := Run([]string{"agent", "install", "--dry-run", "--config", configPath}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Dry run") || !strings.Contains(stdout.String(), "neul-agent") {
		t.Fatalf("stdout = %s, want install preview", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), "neul-agent.plist")); !os.IsNotExist(err) {
		t.Fatalf("service file stat err = %v, want absent", err)
	}
}

func TestAgentStatus_readsLocalConfigAndStatusFile(t *testing.T) {
	configPath := writeTestConfig(t)
	statusPath := filepath.Join(filepath.Dir(configPath), "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"state":"running","lastHeartbeatAt":"2026-06-05T13:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var stdout strings.Builder
	err := Run([]string{"agent", "status", "--config", configPath}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Machine: machine_1\n",
		"LaunchAgent: ",
		"Heartbeat: 2026-06-05T13:00:00Z\n",
		"Last error: none\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %s, want %q", stdout.String(), want)
		}
	}
}

func TestAgentLogs_readsLocalLogFile(t *testing.T) {
	configPath := writeTestConfig(t)
	logPath := filepath.Join(filepath.Dir(configPath), "agent.log")
	if err := os.WriteFile(logPath, []byte("heartbeat ok\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var stdout strings.Builder
	err := Run([]string{"agent", "logs", "--config", configPath}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "heartbeat ok") {
		t.Fatalf("stdout = %s, want log content", stdout.String())
	}
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	return writeTestConfigInDir(t, t.TempDir())
}

func writeTestConfigInDir(t *testing.T, dir string) string {
	t.Helper()
	configPath := filepath.Join(dir, "config.json")
	content := []byte(`{"serverURL":"http://127.0.0.1:8080","machineId":"machine_1","machineToken":"mtn_secret"}`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return configPath
}
