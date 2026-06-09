package scripts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevServerTailnetURL_usesTailscaleIPWhenDNSNameMissing(t *testing.T) {
	requirePython3(t)
	// Given
	binDir := writeFakeTailscaleWithBody(t, `{"Self":{"DNSName":"","TailscaleIPs":["100.64.0.8"]}}`)

	// When
	result := runTailnetHelper(t, binDir, "0.0.0.0", "19090")

	// Then
	if result.err != nil {
		t.Fatalf("tailnet helper error = %v, output = %s", result.err, result.output)
	}
	assertContains(t, result.output, "http://100.64.0.8:19090/")
}

func TestDevServerTailnetURL_withoutSelfPrintsNothing(t *testing.T) {
	requirePython3(t)
	// Given
	binDir := writeFakeTailscaleWithBody(t, `{}`)

	// When
	result := runTailnetHelper(t, binDir, "0.0.0.0", "19091")

	// Then
	if result.err != nil {
		t.Fatalf("tailnet helper error = %v, output = %s", result.err, result.output)
	}
	if result.output != "" {
		t.Fatalf("tailnet helper output = %q, want empty", result.output)
	}
}

func TestDevServerTailnetURL_withInvalidJSONPrintsNothing(t *testing.T) {
	requirePython3(t)
	// Given
	binDir := writeFakeTailscaleWithBody(t, `{not-json`)

	// When
	result := runTailnetHelper(t, binDir, "0.0.0.0", "19092")

	// Then
	if result.err != nil {
		t.Fatalf("tailnet helper error = %v, output = %s", result.err, result.output)
	}
	if result.output != "" {
		t.Fatalf("tailnet helper output = %q, want empty", result.output)
	}
}

func runTailnetHelper(t *testing.T, binDir string, host string, port string) commandResult {
	t.Helper()
	return runCommandWithEnv(t,
		map[string]string{"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH")},
		"/bin/sh",
		filepath.Join(scriptDir(t), "dev-server-tailnet-url"),
		host,
		port,
	)
}

func writeFakeTailscaleWithBody(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "status.json")
	if err := os.WriteFile(bodyPath, []byte(body+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	path := filepath.Join(dir, "tailscale")
	content := "#!/bin/sh\nif [ \"$1\" = \"status\" ]; then\n  cat " + bodyPath + "\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return dir
}
