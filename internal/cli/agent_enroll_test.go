package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentEnroll_connectOnceRequiresFullAgentTick(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/pair/claim":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"machineId":"machine_1","machineToken":"mtn_secret"}`))
		case "/api/agent/heartbeat":
			requireBearer(t, r)
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			requireBearer(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resources":[]}`))
		case "/api/agent/commands":
			requireBearer(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	configDir := t.TempDir()
	var stdout strings.Builder

	err := Run(
		[]string{
			"agent",
			"enroll",
			"--server",
			server.URL,
			"--pair",
			"pair_abc",
			"--config-dir",
			configDir,
			"--connect-once",
		},
		&stdout,
		&stdout,
	)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"/api/pair/claim",
		"/api/agent/heartbeat",
		"/api/agent/desired-state",
		"/api/agent/commands",
	} {
		if !contains(paths, want) {
			t.Fatalf("paths = %v, want %s", paths, want)
		}
	}
	if !strings.Contains(stdout.String(), "Machine enrolled") ||
		!strings.Contains(stdout.String(), "Connecting") ||
		!strings.Contains(stdout.String(), "Connected") {
		t.Fatalf("stdout = %s, want friendly enroll and connected states", stdout.String())
	}
	if strings.Contains(stdout.String(), "mtn_secret") {
		t.Fatalf("stdout leaked token: %s", stdout.String())
	}
	assertConfig(t, filepath.Join(configDir, configFileName), "machine_1")
}

func TestAgentEnroll_existingConfigRequiresForce(t *testing.T) {
	configDir := t.TempDir()
	if err := writeConfig(filepath.Join(configDir, configFileName), Config{
		ServerURL:    "http://127.0.0.1:8080",
		MachineID:    "machine_old",
		MachineToken: "mtn_old",
	}); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called when config exists without force")
	}))
	defer server.Close()

	var stdout strings.Builder
	err := Run(
		[]string{"agent", "enroll", "--server", server.URL, "--pair", "pair_abc", "--config-dir", configDir},
		&stdout,
		&stdout,
	)

	if err == nil {
		t.Fatal("Run() error = nil, want existing config failure")
	}
	if !strings.Contains(err.Error(), "config already exists") {
		t.Fatalf("error = %v, want config already exists", err)
	}
	if strings.Contains(stdout.String(), "mtn_") {
		t.Fatalf("stdout leaked token: %s", stdout.String())
	}
}

func TestAgentEnroll_forceDoesNotDeletePriorServerMachine(t *testing.T) {
	var claimCount int
	var deleteCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount += 1
		}
		if r.URL.Path != "/api/pair/claim" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		claimCount += 1
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"machineId":"machine_` + string(rune('0'+claimCount)) + `","machineToken":"mtn_secret"}`))
	}))
	defer server.Close()
	configDir := t.TempDir()
	var stdout strings.Builder

	if err := Run([]string{"agent", "enroll", "--server", server.URL, "--pair", "pair_1", "--config-dir", configDir}, &stdout, &stdout); err != nil {
		t.Fatalf("first enroll error = %v", err)
	}
	if err := Run([]string{"agent", "enroll", "--server", server.URL, "--pair", "pair_2", "--config-dir", configDir, "--force"}, &stdout, &stdout); err != nil {
		t.Fatalf("force enroll error = %v", err)
	}

	if deleteCount != 0 {
		t.Fatalf("deleteCount = %d, want no server-side delete/revoke", deleteCount)
	}
	assertConfig(t, filepath.Join(configDir, configFileName), "machine_2")
}

func TestAgentEnroll_expiredCodeReportsClearFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":"pairing_code_expired","message":"Pairing code expired."}}`))
	}))
	defer server.Close()

	var stdout strings.Builder
	err := Run(
		[]string{"agent", "enroll", "--server", server.URL, "--pair", "pair_expired", "--config-dir", t.TempDir()},
		&stdout,
		&stdout,
	)

	if err == nil {
		t.Fatal("Run() error = nil, want expired failure")
	}
	if !strings.Contains(err.Error(), "pairing code expired") {
		t.Fatalf("error = %v, want pairing code expired", err)
	}
	if strings.Contains(stdout.String(), "pair_expired") {
		t.Fatalf("stdout leaked pair token: %s", stdout.String())
	}
}

func requireBearer(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer mtn_secret" {
		t.Fatalf("Authorization = %q, want bearer token", r.Header.Get("Authorization"))
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func assertConfig(t *testing.T, path string, machineID string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config stat error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 0600", info.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var config Config
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if config.MachineID != machineID || config.MachineToken != "mtn_secret" {
		t.Fatalf("config = %+v, want %s and token", config, machineID)
	}
}
