package scripts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevServerWrapper_exportsBracketedIPv6ListenAddr_whenHostContainsColon(t *testing.T) {
	requireTmux(t)
	tests := []struct {
		name          string
		suffix        string
		host          string
		port          string
		wantRunner    string
		wantDashboard string
	}{
		{
			name:          "loopback",
			suffix:        "loopback",
			host:          "::1",
			port:          "19096",
			wantRunner:    "export NEUL_ADDR='[::1]:19096'",
			wantDashboard: "Dashboard URL: http://[::1]:19096/",
		},
		{
			name:          "broad bind",
			suffix:        "broad-bind",
			host:          "::",
			port:          "19097",
			wantRunner:    "export NEUL_ADDR='[::]:19097'",
			wantDashboard: "Dashboard URL: http://127.0.0.1:19097/",
		},
		{
			name:          "ula",
			suffix:        "ula",
			host:          "fd00::1",
			port:          "19098",
			wantRunner:    "export NEUL_ADDR='[fd00::1]:19098'",
			wantDashboard: "Dashboard URL: http://[fd00::1]:19098/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			session := uniqueSessionName(t, "ipv6-"+tt.suffix)
			stateDir := t.TempDir()
			t.Cleanup(func() { killTmuxSession(session) })

			// When
			start := runDevServer(t,
				"start",
				"--host", tt.host,
				"--port", tt.port,
				"--state-dir", stateDir,
				"--session", session,
				"--server-command", writeFakeServer(t, "setup_test_ipv6_"+tt.suffix),
			)

			// Then
			if start.err != nil {
				t.Fatalf("start error = %v, output = %s", start.err, start.output)
			}
			runner, err := os.ReadFile(filepath.Join(stateDir, "run-server.sh"))
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			assertContains(t, string(runner), tt.wantRunner)
			assertContains(t, start.output, tt.wantDashboard)
		})
	}
}

func TestDevServerWrapper_rejectsBracketedIPv6Host(t *testing.T) {
	requireTmux(t)
	// Given
	session := uniqueSessionName(t, "bracketed-ipv6")
	stateDir := t.TempDir()
	t.Cleanup(func() { killTmuxSession(session) })

	// When
	start := runDevServer(t,
		"start",
		"--host", "[::1]",
		"--port", "19099",
		"--state-dir", stateDir,
		"--session", session,
		"--server-command", writeFakeServer(t, "setup_test_bracketed_ipv6"),
	)

	// Then
	if start.err == nil {
		t.Fatalf("start error = nil, output = %s", start.output)
	}
	assertContains(t, start.output, "host must be unbracketed")
	if hasTmuxSession(session) {
		t.Fatalf("tmux session %s exists after bracketed host failure", session)
	}
}
