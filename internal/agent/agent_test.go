package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentTick_sendsHeartbeatDesiredStateCommandPollAndReport(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mtn_secret" {
			t.Fatalf("Authorization = %q, want bearer token", r.Header.Get("Authorization"))
		}
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[]}`))
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[{"id":"command_1","type":"repair_drift","payload":{}}]}`))
		case "/api/agent/reconcile-report":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	config := Config{ServerURL: server.URL, MachineID: "machine_1", MachineToken: "mtn_secret"}
	client := New(config)
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	joined := strings.Join(paths, "\n")
	for _, expected := range []string{
		"POST /api/agent/heartbeat",
		"GET /api/agent/desired-state",
		"GET /api/agent/commands",
		"POST /api/agent/reconcile-report",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("paths = %s, missing %s", joined, expected)
		}
	}
}

func TestCommandPolling_unknownCommandReportsUnsupportedAndDoesNotExecute(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "executed")
	var reportBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[]}`))
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[{"id":"command_shell","type":"shell","payload":{"touch":"` + markerPath + `"}}]}`))
		case "/api/agent/reconcile-report":
			body, err := readAllString(r)
			if err != nil {
				t.Fatalf("read report error = %v", err)
			}
			reportBody = body
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL, MachineID: "machine_1", MachineToken: "mtn_secret"})
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("marker stat err = %v, want absent", err)
	}
	if !strings.Contains(reportBody, "unsupported_command") {
		t.Fatalf("report body = %s, want unsupported_command", reportBody)
	}
}

func TestAgentTick_reportsDotfileDesiredState(t *testing.T) {
	var reportBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[{"id":"resource_dot_zshrc","kind":"dotfile","name":"~/.zshrc","desiredVersion":2,"spec":{"path":"~/.zshrc","content":"export NEUL=v2\n","mode":"0600","applyMode":"symlink","targetSegment":"base"}}]}`))
		case "/api/agent/drift-report":
			body, err := readAllString(r)
			if err != nil {
				t.Fatalf("read drift report error = %v", err)
			}
			reportBody = body
			w.WriteHeader(http.StatusAccepted)
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL, MachineID: "machine_dot", MachineToken: "mtn_secret"})
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	for _, expected := range []string{
		`"resourceId":"resource_dot_zshrc"`,
		`"status":"in_sync"`,
		`"message":"dotfile dry run"`,
		`"desiredVersion":2`,
		`"appliedVersion":2`,
	} {
		if !strings.Contains(reportBody, expected) {
			t.Fatalf("report body = %s, missing %s", reportBody, expected)
		}
	}
}

func TestAgentTick_defaultHeartbeatIntervalIsThirtySeconds(t *testing.T) {
	config := DefaultConfig()
	if config.HeartbeatInterval != 30*time.Second {
		t.Fatalf("HeartbeatInterval = %s, want 30s", config.HeartbeatInterval)
	}
}

func TestNoWebSocket_usesOnlyHttpRestPaths(t *testing.T) {
	config := Config{ServerURL: "http://127.0.0.1:8080", MachineID: "machine_1", MachineToken: "mtn_secret"}
	client := New(config)
	for _, endpoint := range client.Endpoints() {
		if strings.HasPrefix(endpoint, "ws://") || strings.HasPrefix(endpoint, "wss://") || strings.Contains(endpoint, "/ws") {
			t.Fatalf("endpoint = %s, want REST only", endpoint)
		}
	}
}

func TestAgentConfig_loadsCliPairingConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"serverURL":"http://127.0.0.1:8080","machineId":"machine_1","machineToken":"mtn_secret"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ServerURL == "" || config.MachineID == "" || config.MachineToken == "" {
		t.Fatalf("config = %+v, want loaded machine credentials", config)
	}
}

func readAllString(r *http.Request) (string, error) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
