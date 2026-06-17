package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStatusFlag_whenOmitted_defaultsToConfigDirectoryStatusJSON(t *testing.T) {
	configDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer server.Close()
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"serverURL":"`+server.URL+`","machineId":"machine_default_status","machineToken":"mtn_secret"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestNeulAgentMainHelper", "--", "--once", "--config", configPath)
	cmd.Env = append(os.Environ(), "NEUL_AGENT_TEST_HELPER=1")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("neul-agent helper failed: %v\n%s", err, string(output))
	}
	statusPath := filepath.Join(configDir, "status.json")
	body, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var status struct {
		MachineID string `json:"machineId"`
		LastError *struct {
			Message string `json:"message"`
		} `json:"lastError"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if status.MachineID != "machine_default_status" || status.LastError != nil {
		t.Fatalf("status = %+v, want successful default receipt", status)
	}
}

func TestNeulAgentMainHelper(t *testing.T) {
	if os.Getenv("NEUL_AGENT_TEST_HELPER") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"neul-agent"}, os.Args[index+1:]...)
			main()
			return
		}
	}
	t.Fatal("missing helper argument separator")
}
