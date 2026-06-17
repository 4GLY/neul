package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentStatus_printsContractWithUnknownLaunchAgent_whenLaunchctlUnavailable(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	statusPath := filepath.Join(filepath.Dir(configPath), "status.json")
	writeTestFile(t, statusPath, `{"lastHeartbeatAt":"2026-06-05T13:00:00Z","lastError":"disk full"}`)
	restore := overrideAgentStatusRuntime(t, "darwin", func(string) (launchAgentState, error) {
		return launchAgentStateUnknown, errLaunchAgentStateUnknown
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "status", "--config", configPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := strings.Join([]string{
		"Machine: machine_1",
		"Config: " + configPath,
		"LaunchAgent: unknown",
		"Heartbeat: 2026-06-05T13:00:00Z",
		"Last error: disk full",
		"",
	}, "\n")
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestAgentStatus_printsUnloadedAndNone_whenLaunchAgentIsUnloaded(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	restore := overrideAgentStatusRuntime(t, "darwin", func(string) (launchAgentState, error) {
		return launchAgentStateUnloaded, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "status", "--config", configPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Machine: machine_1\n",
		"Config: " + configPath + "\n",
		"LaunchAgent: unloaded\n",
		"Heartbeat: unknown\n",
		"Last error: none\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestAgentStatus_readsSelectedStatusPath_whenStatusFlagIsProvided(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	defaultStatusPath := filepath.Join(filepath.Dir(configPath), "status.json")
	customStatusPath := filepath.Join(t.TempDir(), "custom-status.json")
	writeTestFile(t, defaultStatusPath, `{"lastHeartbeatAt":"2026-06-05T13:00:00Z"}`)
	writeTestFile(t, customStatusPath, `{"lastHeartbeatAt":"2026-06-05T14:00:00Z"}`)
	restore := overrideAgentStatusRuntime(t, "linux", func(string) (launchAgentState, error) {
		t.Fatal("launch agent probe should not run on non-darwin")
		return launchAgentStateLoaded, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "status", "--config", configPath, "--status", customStatusPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Heartbeat: 2026-06-05T14:00:00Z\n") {
		t.Fatalf("stdout = %q, want custom status heartbeat", stdout.String())
	}
}

func TestAgentStatus_returnsError_whenConfigMissingOrInvalid(t *testing.T) {
	tests := []struct {
		name       string
		configBody string
		want       string
	}{
		{name: "missing config", want: "read config"},
		{name: "invalid config", configBody: "{", want: "decode config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			configPath := filepath.Join(t.TempDir(), "config.json")
			if tt.configBody != "" {
				writeTestFile(t, configPath, tt.configBody)
			}
			var stdout strings.Builder

			// When
			err := Run([]string{"agent", "status", "--config", configPath}, &stdout, &stdout)

			// Then
			if err == nil {
				t.Fatal("Run() error = nil, want config failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if stdout.String() != "" {
				t.Fatalf("stdout = %q, want empty output on failure", stdout.String())
			}
		})
	}
}

func TestAgentStatus_printsUnknownLaunchAgent_whenNotDarwin(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	restore := overrideAgentStatusRuntime(t, "linux", func(string) (launchAgentState, error) {
		t.Fatal("launch agent probe should not run on non-darwin")
		return launchAgentStateLoaded, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "status", "--config", configPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "LaunchAgent: unknown\n") {
		t.Fatalf("stdout = %q, want unknown launch agent", stdout.String())
	}
}

func TestAgentServiceActions_returnUnsupported_whenNotDarwin(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	previousGOOS := agentServiceGOOS
	agentServiceGOOS = "linux"
	defer func() {
		agentServiceGOOS = previousGOOS
	}()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "install",
			args: []string{"agent", "install", "--config", configPath},
			want: "agent install is unsupported on linux",
		},
		{
			name: "start",
			args: []string{"agent", "start", "--config", configPath},
			want: "agent start is unsupported on linux",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout strings.Builder

			// When
			err := Run(tt.args, &stdout, &stdout)

			// Then
			if err == nil {
				t.Fatal("Run() error = nil, want unsupported failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func overrideAgentStatusRuntime(t *testing.T, goos string, probe func(string) (launchAgentState, error)) func() {
	t.Helper()
	previousGOOS := agentStatusGOOS
	previousProbe := probeLaunchAgentState
	agentStatusGOOS = goos
	probeLaunchAgentState = probe
	return func() {
		agentStatusGOOS = previousGOOS
		probeLaunchAgentState = previousProbe
	}
}
