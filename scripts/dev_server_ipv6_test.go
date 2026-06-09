package scripts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevServerWrapper_exportsBracketedIPv6ListenAddr_whenHostContainsColon(t *testing.T) {
	requireTmux(t)
	tests := []struct {
		name   string
		suffix string
		host   string
		port   string
		want   string
	}{
		{name: "loopback", suffix: "loopback", host: "::1", port: "19096", want: "export NEUL_ADDR='[::1]:19096'"},
		{name: "broad bind", suffix: "broad-bind", host: "::", port: "19097", want: "export NEUL_ADDR='[::]:19097'"},
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
			assertContains(t, string(runner), tt.want)
		})
	}
}
