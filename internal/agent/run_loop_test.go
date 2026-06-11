package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunLoop_whenContextCancelsAfterThreeTicks_sendsHeartbeatsWithoutManualTicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var heartbeatCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			heartbeatCount.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[]}`))
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
			if heartbeatCount.Load() == 3 {
				cancel()
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(Config{
		ServerURL:         server.URL,
		MachineID:         "machine_loop",
		MachineToken:      "mtn_loop",
		HeartbeatInterval: time.Millisecond,
	})
	if err := client.Run(ctx, RunOptions{Delay: immediateDelay}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if heartbeatCount.Load() != 3 {
		t.Fatalf("heartbeatCount = %d, want 3", heartbeatCount.Load())
	}
}

func TestRunLoop_whenNetworkFails_retriesWithBackoffWithoutLogSpamOrConfigMutation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	configBody := []byte(`{"serverURL":"http://127.0.0.1:1","machineId":"machine_retry","machineToken":"mtn_retry"}`)
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delayCount int
	var observedDelays []time.Duration
	var logBuffer bytes.Buffer
	client := New(Config{
		ServerURL:         "http://127.0.0.1:1",
		MachineID:         "machine_retry",
		MachineToken:      "mtn_retry",
		HeartbeatInterval: time.Millisecond,
	})

	err := client.Run(ctx, RunOptions{
		Delay: func(ctx context.Context, delay time.Duration) error {
			observedDelays = append(observedDelays, delay)
			delayCount++
			if delayCount == 3 {
				cancel()
			}
			return immediateDelay(ctx, delay)
		},
		ConfigReloader: func() (Config, error) {
			return LoadConfig(configPath)
		},
		Logger: slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(observedDelays) != 3 {
		t.Fatalf("observedDelays = %v, want three retry delays", observedDelays)
	}
	if !(observedDelays[0] < observedDelays[1] && observedDelays[1] <= observedDelays[2]) {
		t.Fatalf("observedDelays = %v, want non-spamming backoff growth", observedDelays)
	}
	if strings.Count(logBuffer.String(), "agent tick failed") != 1 {
		t.Fatalf("logs = %s, want one repeated network failure log", logBuffer.String())
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(after) != string(configBody) {
		t.Fatalf("config file changed to %s, want original %s", string(after), string(configBody))
	}
}

func TestRunLoop_whenConfigReloadPointsAtRecoveredServer_usesReloadedConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var heartbeatCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			heartbeatCount.Add(1)
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
	var reloadCount int
	client := New(Config{
		ServerURL:         "http://127.0.0.1:1",
		MachineID:         "machine_reload",
		MachineToken:      "mtn_reload",
		HeartbeatInterval: time.Millisecond,
	})

	if err := client.Run(ctx, RunOptions{
		Delay: func(ctx context.Context, delay time.Duration) error {
			if heartbeatCount.Load() == 1 {
				cancel()
			}
			return immediateDelay(ctx, delay)
		},
		ConfigReloader: func() (Config, error) {
			reloadCount++
			if reloadCount == 1 {
				return Config{ServerURL: "http://127.0.0.1:1", MachineID: "machine_reload", MachineToken: "mtn_reload", HeartbeatInterval: time.Millisecond}, nil
			}
			return Config{ServerURL: server.URL, MachineID: "machine_reload", MachineToken: "mtn_reload", HeartbeatInterval: time.Millisecond}, nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if heartbeatCount.Load() != 1 {
		t.Fatalf("heartbeatCount = %d, want recovered heartbeat through reloaded config", heartbeatCount.Load())
	}
}

func TestRunLoop_whenConfigReloadFails_keepsExistingConfigAndLogsReloadFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var heartbeatCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			heartbeatCount.Add(1)
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
	var logBuffer bytes.Buffer
	client := New(Config{
		ServerURL:         server.URL,
		MachineID:         "machine_reload_failure",
		MachineToken:      "mtn_reload_failure",
		HeartbeatInterval: time.Millisecond,
	})

	if err := client.Run(ctx, RunOptions{
		Delay: func(ctx context.Context, delay time.Duration) error {
			if heartbeatCount.Load() == 1 {
				cancel()
			}
			return immediateDelay(ctx, delay)
		},
		ConfigReloader: func() (Config, error) {
			return Config{}, errors.New("permission denied")
		},
		Logger: slog.New(slog.NewTextHandler(&logBuffer, nil)),
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if heartbeatCount.Load() != 1 {
		t.Fatalf("heartbeatCount = %d, want existing config to keep ticking", heartbeatCount.Load())
	}
	if !strings.Contains(logBuffer.String(), "agent config reload failed") {
		t.Fatalf("logs = %s, want reload failure warning", logBuffer.String())
	}
}

func TestRunLoop_whenFailureKindChanges_logsEachKindAndRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var heartbeatCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			current := heartbeatCount.Add(1)
			if current == 1 {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			if current == 2 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
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
	}))
	defer server.Close()
	var logBuffer bytes.Buffer
	client := New(Config{
		ServerURL:         server.URL,
		MachineID:         "machine_kind_change",
		MachineToken:      "mtn_kind_change",
		HeartbeatInterval: time.Millisecond,
	})

	if err := client.Run(ctx, RunOptions{
		Delay: func(ctx context.Context, delay time.Duration) error {
			if heartbeatCount.Load() == 3 {
				cancel()
			}
			return immediateDelay(ctx, delay)
		},
		Logger: slog.New(slog.NewTextHandler(&logBuffer, nil)),
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	logs := logBuffer.String()
	if strings.Count(logs, "agent tick failed") != 2 {
		t.Fatalf("logs = %s, want one warning per failure kind", logs)
	}
	if !strings.Contains(logs, "kind=server_failure") || !strings.Contains(logs, "kind=auth_failure") {
		t.Fatalf("logs = %s, want server and auth failure kinds", logs)
	}
	if !strings.Contains(logs, "agent connection restored") || !strings.Contains(logs, "failures=2") {
		t.Fatalf("logs = %s, want recovery log with failure count", logs)
	}
}

func TestGrowBackoff_whenCurrentWouldOverflow_capsAtMaximum(t *testing.T) {
	maxBackoff := time.Hour
	if got := growBackoff(time.Duration(1<<62), maxBackoff); got != maxBackoff {
		t.Fatalf("growBackoff() = %s, want %s", got, maxBackoff)
	}
}

func immediateDelay(ctx context.Context, _ time.Duration) error {
	return ctx.Err()
}
