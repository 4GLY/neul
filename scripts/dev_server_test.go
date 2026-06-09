package scripts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevServerWrapper_startStatusLogsAndStopUsesTmux(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "start")
	stateDir := t.TempDir()
	serverCommand := writeFakeServer(t, "setup_test_start")
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServer(t,
		"start",
		"--host", "127.0.0.1",
		"--port", "19081",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", serverCommand,
	)

	// Then
	if start.err != nil {
		t.Fatalf("start error = %v, output = %s", start.err, start.output)
	}
	assertContains(t, start.output, "Setup token: setup_test_start")
	assertContains(t, start.output, "Dashboard URL: http://127.0.0.1:19081/")
	assertContains(t, start.output, "Stop command:")
	assertContains(t, start.output, "./scripts/dev-server stop")

	status := runDevServer(t, "status", "--state-dir", stateDir, "--session", session)
	if status.err != nil {
		t.Fatalf("status error = %v, output = %s", status.err, status.output)
	}
	assertContains(t, status.output, "Status: running")
	assertContains(t, status.output, "Dashboard URL: http://127.0.0.1:19081/")

	logs := runDevServer(t, "logs", "--state-dir", stateDir, "--session", session)
	if logs.err != nil {
		t.Fatalf("logs error = %v, output = %s", logs.err, logs.output)
	}
	assertContains(t, logs.output, "fake server started")
	assertContains(t, logs.output, "NEUL_ADDR=127.0.0.1:19081")
	assertContains(t, logs.output, "NEUL_DB="+filepath.Join(stateDir, "neul.sqlite"))
	assertContains(t, logs.output, "NEUL_STATIC_DIR="+filepath.Join(scriptDir(t), "..", "web/dist"))

	stop := runDevServer(t, "stop", "--state-dir", stateDir, "--session", session)
	if stop.err != nil {
		t.Fatalf("stop error = %v, output = %s", stop.err, stop.output)
	}
	assertContains(t, stop.output, "Stopped")
	if hasTmuxSession(session) {
		t.Fatalf("tmux session %s still exists after stop", session)
	}
}

func TestDevServerWrapper_repeatedStartReusesRunningSession(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "repeat")
	stateDir := t.TempDir()
	serverCommand := writeFakeServer(t, "setup_test_repeat")
	t.Cleanup(func() { killTmuxSession(session) })

	first := runDevServer(t,
		"start",
		"--host", "127.0.0.1",
		"--port", "19082",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", serverCommand,
	)
	if first.err != nil {
		t.Fatalf("first start error = %v, output = %s", first.err, first.output)
	}

	// When
	second := runDevServer(t,
		"start",
		"--host", "127.0.0.1",
		"--port", "19082",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", serverCommand,
	)

	// Then
	if second.err != nil {
		t.Fatalf("second start error = %v, output = %s", second.err, second.output)
	}
	assertContains(t, second.output, "Status: already running")
	assertContains(t, second.output, "Dashboard URL: http://127.0.0.1:19082/")
	assertContains(t, second.output, "Stop command:")
	if countTmuxSession(t, session) != 1 {
		t.Fatalf("tmux session count = %d, want 1", countTmuxSession(t, session))
	}
}

func TestDevServerWrapper_missingServerCommandFailsBeforeTmux(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "missing")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServer(t,
		"start",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", "definitely-missing-neul-server",
	)

	// Then
	if start.err == nil {
		t.Fatalf("start error = nil, output = %s", start.output)
	}
	assertContains(t, start.output, "server command not found")
	assertContains(t, start.output, "definitely-missing-neul-server")
	if hasTmuxSession(session) {
		t.Fatalf("tmux session %s exists after missing command failure", session)
	}
}

func TestDevServerWrapper_configurableHostPortAndTailnetURL(t *testing.T) {
	requireTmux(t)
	requirePython3(t)
	// Given
	session := uniqueSessionName(t, "tailnet")
	stateDir := t.TempDir()
	serverCommand := writeFakeServer(t, "setup_test_tailnet")
	tailnetBin := writeFakeTailscale(t)
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServerWithEnv(t,
		map[string]string{"PATH": tailnetBin + string(os.PathListSeparator) + os.Getenv("PATH")},
		"start",
		"--host", "0.0.0.0",
		"--port", "19083",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", serverCommand,
	)

	// Then
	if start.err != nil {
		t.Fatalf("start error = %v, output = %s", start.err, start.output)
	}
	assertContains(t, start.output, "Dashboard URL: http://127.0.0.1:19083/")
	assertContains(t, start.output, "Tailnet URL: http://neul.tail.ts.net:19083/")

	logs := runDevServer(t, "logs", "--state-dir", stateDir, "--session", session)
	if logs.err != nil {
		t.Fatalf("logs error = %v, output = %s", logs.err, logs.output)
	}
	assertContains(t, logs.output, "NEUL_ADDR=0.0.0.0:19083")
}

type commandResult struct {
	output string
	err    error
}

func runDevServer(t *testing.T, args ...string) commandResult {
	t.Helper()
	return runDevServerWithEnv(t, nil, args...)
}

func runDevServerWithEnv(t *testing.T, env map[string]string, args ...string) commandResult {
	t.Helper()
	return runCommandWithEnv(t, env, scriptPath(t), args...)
}

func runCommandWithEnv(t *testing.T, env map[string]string, name string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Env = mergedEnv(env)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return commandResult{output: string(output), err: ctx.Err()}
	}
	return commandResult{output: string(output), err: err}
}

func scriptPath(t *testing.T) string {
	return filepath.Join(scriptDir(t), "dev-server")
}

func scriptDir(t *testing.T) string {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	return workingDir
}

func writeFakeServer(t *testing.T, setupToken string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-neul-server")
	content := fmt.Sprintf(`#!/usr/bin/env sh
printf 'fake server started\n'
printf 'NEUL_ADDR=%%s\n' "$NEUL_ADDR"
printf 'NEUL_DB=%%s\n' "$NEUL_DB"
printf 'NEUL_STATIC_DIR=%%s\n' "$NEUL_STATIC_DIR"
printf 'neul setup token: %s\n'
trap 'printf "fake server stopped\n"; exit 0' INT TERM
while :; do sleep 1; done
`, setupToken)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeFakeTailscale(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tailscale")
	content := `#!/usr/bin/env sh
if [ "$1" = "status" ]; then
  printf '{"Self":{"DNSName":"neul.tail.ts.net.","TailscaleIPs":["100.64.0.7"]}}\n'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return dir
}

func uniqueSessionName(t *testing.T, suffix string) string {
	t.Helper()
	return fmt.Sprintf("neul-test-%s-%d", suffix, time.Now().UnixNano())
}

func hasTmuxSession(session string) bool {
	return exec.Command("tmux", "has-session", "-t", "="+session).Run() == nil
}

func killTmuxSession(session string) {
	_ = exec.Command("tmux", "kill-session", "-t", "="+session).Run()
}

func countTmuxSession(t *testing.T, session string) int {
	output, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == session {
			count += 1
		}
	}
	return count
}

func mergedEnv(overrides map[string]string) []string {
	env := os.Environ()
	for key, value := range overrides {
		prefix := key + "="
		replaced := false
		for index, item := range env {
			if strings.HasPrefix(item, prefix) {
				env[index] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}
	return env
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output = %q, want substring %q", haystack, needle)
	}
}
