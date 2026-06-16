package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentReset_refusesMissingOrWrongMachineID(t *testing.T) {
	// Given
	configDir := t.TempDir()
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	mustWriteFile(t, plistPath, "plist")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing confirmation",
			args: []string{"agent", "reset", "--config", configPath, "--plist", plistPath},
			want: "machine id",
		},
		{
			name: "empty confirmation",
			args: []string{"agent", "reset", "--config", configPath, "--plist", plistPath, "--machine-id", ""},
			want: "machine id",
		},
		{
			name: "malformed confirmation",
			args: []string{"agent", "reset", "--config", configPath, "--plist", plistPath, "--machine-id", "not_machine"},
			want: "malformed",
		},
		{
			name: "wrong confirmation",
			args: []string{"agent", "reset", "--config", configPath, "--plist", plistPath, "--machine-id", "machine_other"},
			want: "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout strings.Builder

			// When
			err := Run(tt.args, &stdout, &stdout)

			// Then
			if err == nil {
				t.Fatal("Run() error = nil, want reset refusal")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			assertPathExists(t, configPath)
			assertPathExists(t, filepath.Join(configDir, "status.json"))
			assertPathExists(t, filepath.Join(configDir, "agent.log"))
			assertPathExists(t, plistPath)
		})
	}
}

func TestAgentReset_removesSelectedPlistStatusAndLog(t *testing.T) {
	// Given
	configDir := t.TempDir()
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	mustWriteFile(t, plistPath, "plist")
	keepPath := filepath.Join(configDir, "notes.txt")
	mustWriteFile(t, keepPath, "keep")
	var commands [][]string
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		commands = append(commands, append([]string(nil), command...))
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{
		"agent", "reset",
		"--config", configPath,
		"--plist", plistPath,
		"--status", filepath.Join(configDir, "status.json"),
		"--log", filepath.Join(configDir, "agent.log"),
		"--machine-id", "machine_1",
	}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertLaunchctlBootout(t, commands)
	for _, name := range []string{configFileName, "status.json", "agent.log", "neul-agent.plist"} {
		assertPathMissing(t, filepath.Join(configDir, name))
	}
	assertPathExists(t, keepPath)
	assertPathExists(t, configDir)
	if !strings.Contains(stdout.String(), "machine_1") {
		t.Fatalf("stdout = %s, want machine id receipt", stdout.String())
	}
}

func TestAgentReset_removesSelectedLocalState_whenNotDarwin(t *testing.T) {
	// Given a non-macOS host where launchctl is unavailable.
	previousGOOS := agentServiceGOOS
	agentServiceGOOS = "linux"
	defer func() {
		agentServiceGOOS = previousGOOS
	}()
	configDir := t.TempDir()
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	mustWriteFile(t, plistPath, "plist")
	keepPath := filepath.Join(configDir, "notes.txt")
	mustWriteFile(t, keepPath, "keep")
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		t.Fatalf("launchctl should not run on %s, got %#v", agentServiceGOOS, command)
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{
		"agent", "reset",
		"--config", configPath,
		"--plist", plistPath,
		"--status", filepath.Join(configDir, "status.json"),
		"--log", filepath.Join(configDir, "agent.log"),
		"--machine-id", "machine_1",
	}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, name := range []string{configFileName, "status.json", "agent.log", "neul-agent.plist"} {
		assertPathMissing(t, filepath.Join(configDir, name))
	}
	assertPathExists(t, keepPath)
	if !strings.Contains(stdout.String(), "machine_1") {
		t.Fatalf("stdout = %s, want machine id receipt", stdout.String())
	}
}

func TestAgentReset_removesCustomPathsAndKeepsExternalParentDirectories(t *testing.T) {
	// Given a service installed with custom plist/status/log outside the config dir.
	rootDir := t.TempDir()
	configDir := filepath.Join(rootDir, "config")
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistDir := filepath.Join(rootDir, "LaunchAgents")
	statusDir := filepath.Join(rootDir, "state")
	logDir := filepath.Join(rootDir, "logs")
	plistPath := filepath.Join(plistDir, "com.4gly.neul.agent.plist")
	statusPath := filepath.Join(statusDir, "status.json")
	logPath := filepath.Join(logDir, "agent.log")
	for _, path := range []string{plistPath, statusPath, logPath} {
		mustWriteFile(t, path, "custom")
	}
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{
		"agent", "reset",
		"--config", configPath,
		"--plist", plistPath,
		"--status", statusPath,
		"--log", logPath,
		"--machine-id", "machine_1",
	}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, path := range []string{configPath, plistPath, statusPath, logPath} {
		assertPathMissing(t, path)
	}
	for _, dir := range []string{plistDir, statusDir, logDir, configDir, rootDir} {
		assertPathExists(t, dir)
	}
}

func TestAgentReset_removesDefaultConfigDirOnlyWhenEmpty(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	defaultDir := filepath.Join(t.TempDir(), "neul")
	t.Setenv("NEUL_CONFIG_DIR", defaultDir)
	configPath := filepath.Join(defaultDir, configFileName)
	writeResetTestState(t, defaultDir, "machine_1")
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "reset", "--machine-id", "machine_1"}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertPathMissing(t, configPath)
	assertPathMissing(t, defaultDir)
}

func TestAgentReset_keepsDefaultConfigDirWhenNotEmpty(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	defaultDir := filepath.Join(t.TempDir(), "neul")
	t.Setenv("NEUL_CONFIG_DIR", defaultDir)
	configPath := filepath.Join(defaultDir, configFileName)
	keepPath := filepath.Join(defaultDir, "keep.txt")
	writeResetTestState(t, defaultDir, "machine_1")
	mustWriteFile(t, keepPath, "keep")
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "reset", "--machine-id", "machine_1"}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertPathMissing(t, configPath)
	assertPathExists(t, keepPath)
	assertPathExists(t, defaultDir)
}

func TestAgentReset_keepsExternalParentDirectories(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	parentDir := t.TempDir()
	configDir := filepath.Join(parentDir, "nested", "..", "external")
	configPath := filepath.Join(configDir, configFileName)
	writeResetTestState(t, configDir, "machine_1")
	keepPath := filepath.Join(parentDir, "parent-keep.txt")
	mustWriteFile(t, keepPath, "keep")
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "reset", "--config", configPath, "--machine-id", "machine_1"}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertPathExists(t, parentDir)
	assertPathExists(t, keepPath)
	assertPathExists(t, filepath.Dir(configPath))
}

func TestResetDoesNotCallServerDeleteOrRevoke(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, configFileName)
	if err := writeConfig(configPath, Config{
		ServerURL:    server.URL,
		MachineID:    "machine_1",
		MachineToken: "mtn_secret",
	}); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "reset", "--config", configPath, "--machine-id", "machine_1"}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("server paths = %v, want no reset network calls", paths)
	}
	t.Logf("server paths after reset: %v", paths)
}

func writeResetTestState(t *testing.T, configDir string, machineID string) string {
	t.Helper()
	configPath := filepath.Join(configDir, configFileName)
	if err := writeConfig(configPath, Config{
		ServerURL:    "http://127.0.0.1:8080",
		MachineID:    machineID,
		MachineToken: "mtn_secret",
	}); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	for _, name := range []string{"status.json", "agent.log"} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	return configPath
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%s) error = %v, want exists", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want missing", path, err)
	}
}
