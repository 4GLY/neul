package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevServerWrapper_statusAfterStopOmitsStaleSetupToken(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "stale")
	stateDir := t.TempDir()
	serverCommand := writeFakeServer(t, "setup_test_stale")
	t.Cleanup(func() { killTmuxSession(session) })

	start := runDevServer(t,
		"start",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", serverCommand,
	)
	if start.err != nil {
		t.Fatalf("start error = %v, output = %s", start.err, start.output)
	}
	stop := runDevServer(t, "stop", "--state-dir", stateDir, "--session", session)
	if stop.err != nil {
		t.Fatalf("stop error = %v, output = %s", stop.err, stop.output)
	}

	// When
	status := runDevServer(t, "status", "--state-dir", stateDir, "--session", session)

	// Then
	if status.err != nil {
		t.Fatalf("status error = %v, output = %s", status.err, status.output)
	}
	assertContains(t, status.output, "Status: stopped")
	if strings.Contains(status.output, "setup_test_stale") {
		t.Fatalf("status output leaked stale setup token after stop: %s", status.output)
	}
}

func TestDevServerWrapper_rejectsFlagNameAsValue(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "flag-value")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServer(t,
		"start",
		"--host", "--port",
		"19084",
		"--state-dir", stateDir,
		"--session", session,
	)

	// Then
	if start.err == nil {
		t.Fatalf("start error = nil, output = %s", start.output)
	}
	assertContains(t, start.output, "invalid value for --host")
	if hasTmuxSession(session) {
		t.Fatalf("tmux session %s exists after invalid flag value", session)
	}
}

func TestDevServerWrapper_logsWithoutLogFileFailsClearly(t *testing.T) {
	// Given
	stateDir := t.TempDir()

	// When
	logs := runDevServer(t, "logs", "--state-dir", stateDir, "--session", uniqueSessionName(t, "logs-missing"))

	// Then
	if logs.err == nil {
		t.Fatalf("logs error = nil, output = %s", logs.output)
	}
	assertContains(t, logs.output, "log file was not found")
}

func TestDevServerWrapper_stopWithoutRunningSessionIsIdempotent(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "stop-missing")
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	stop := runDevServer(t, "stop", "--state-dir", t.TempDir(), "--session", session)

	// Then
	if stop.err != nil {
		t.Fatalf("stop error = %v, output = %s", stop.err, stop.output)
	}
	assertContains(t, stop.output, "Status: stopped")
}

func TestDevServerWrapper_restartWithSameStatePrintsFreshSetupToken(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "fresh-token")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	first := runDevServer(t, "start", "--state-dir", stateDir, "--session", session, "--server-command", writeFakeServer(t, "setup_test_old"))
	if first.err != nil {
		t.Fatalf("first start error = %v, output = %s", first.err, first.output)
	}
	stop := runDevServer(t, "stop", "--state-dir", stateDir, "--session", session)
	if stop.err != nil {
		t.Fatalf("stop error = %v, output = %s", stop.err, stop.output)
	}

	// When
	second := runDevServer(t, "start", "--state-dir", stateDir, "--session", session, "--server-command", writeFakeServer(t, "setup_test_new"))

	// Then
	if second.err != nil {
		t.Fatalf("second start error = %v, output = %s", second.err, second.output)
	}
	assertContains(t, second.output, "Setup token: setup_test_new")
	if strings.Contains(second.output, "setup_test_old") {
		t.Fatalf("second start output used stale setup token: %s", second.output)
	}
}

func TestDevServerWrapper_startIgnoresPrefixMatchedTmuxSession(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "prefix")
	prefixNeighbor := session + "-neighbor"
	stateDir := t.TempDir()
	t.Cleanup(func() {
		killTmuxSession(session)
		killTmuxSession(prefixNeighbor)
	})
	if err := execCommand("tmux", "new-session", "-d", "-s", prefixNeighbor, "sleep 30").Run(); err != nil {
		t.Fatalf("create neighbor tmux session error = %v", err)
	}

	// When
	start := runDevServer(t, "start", "--state-dir", stateDir, "--session", session, "--server-command", writeFakeServer(t, "setup_test_prefix"))

	// Then
	if start.err != nil {
		t.Fatalf("start error = %v, output = %s", start.err, start.output)
	}
	assertContains(t, start.output, "Status: running")
	if !hasTmuxSession(prefixNeighbor) {
		t.Fatalf("neighbor session %s was incorrectly removed", prefixNeighbor)
	}
}

func TestDevServerWrapper_withoutCommandPrintsUsage(t *testing.T) {
	// When
	usage := runDevServer(t)

	// Then
	if usage.err != nil {
		t.Fatalf("usage error = %v, output = %s", usage.err, usage.output)
	}
	assertContains(t, usage.output, "Usage: ./scripts/dev-server")
}

func TestDevServerWrapper_unknownCommandFailsWithUsage(t *testing.T) {
	// When
	unknown := runDevServer(t, "bogus")

	// Then
	if unknown.err == nil {
		t.Fatalf("unknown command error = nil, output = %s", unknown.output)
	}
	assertContains(t, unknown.output, "unknown command: bogus")
	assertContains(t, unknown.output, "Usage: ./scripts/dev-server")
}

func TestDevServerWrapper_rejectsQuotedServerCommandBeforeTmux(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "quoted-command")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServer(t, "start", "--state-dir", stateDir, "--session", session, "--server-command", `"/path with spaces/neul"`)

	// Then
	if start.err == nil {
		t.Fatalf("start error = nil, output = %s", start.output)
	}
	assertContains(t, start.output, "server command executable path cannot be quoted")
	if hasTmuxSession(session) {
		t.Fatalf("tmux session %s exists after quoted command rejection", session)
	}
}

func TestDevServerWrapper_startTimeoutCleansTmuxSession(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "timeout")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServerWithEnv(t,
		map[string]string{"NEUL_DEV_SERVER_STARTUP_ATTEMPTS": "1"},
		"start",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", writeSilentServer(t),
	)

	// Then
	if start.err == nil {
		t.Fatalf("start error = nil, output = %s", start.output)
	}
	assertContains(t, start.output, "did not become ready")
	if hasTmuxSession(session) {
		t.Fatalf("tmux session %s exists after startup timeout", session)
	}
}

func TestDevServerWrapper_startAcceptsHealthWithoutSetupToken(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "health")
	stateDir := t.TempDir()
	port := "19095"
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServer(t,
		"start",
		"--host", "127.0.0.1",
		"--port", port,
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", writeHealthServer(t),
	)

	// Then
	if start.err != nil {
		t.Fatalf("start error = %v, output = %s", start.err, start.output)
	}
	assertContains(t, start.output, "Status: running")
	assertContains(t, start.output, "Setup token: unavailable")
}

func TestDevServerWrapper_stopRemovesGeneratedRunnerFiles(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "runner-cleanup")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	start := runDevServer(t, "start", "--state-dir", stateDir, "--session", session, "--server-command", writeFakeServer(t, "setup_test_runner"))
	if start.err != nil {
		t.Fatalf("start error = %v, output = %s", start.err, start.output)
	}

	// When
	stop := runDevServer(t, "stop", "--state-dir", stateDir, "--session", session)

	// Then
	if stop.err != nil {
		t.Fatalf("stop error = %v, output = %s", stop.err, stop.output)
	}
	matches, err := filepath.Glob(filepath.Join(stateDir, "run-server-*.sh"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("runner files = %v, want none", matches)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "run-server.sh")); !os.IsNotExist(err) {
		t.Fatalf("run-server.sh exists after stop, stat error = %v", err)
	}
}

func writeSilentServer(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "silent-server")
	content := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeHealthServer(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "health-server.go")
	content := `package main

import (
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"ok\":true}\n"))
	})
	if err := http.ListenAndServe(os.Getenv("NEUL_ADDR"), mux); err != nil {
		panic(err)
	}
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return "go run " + path
}
