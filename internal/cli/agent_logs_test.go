package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentLogs_printsLastTwoHundredLinesByDefault(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	logPath := filepath.Join(filepath.Dir(configPath), "agent.log")
	writeTestFile(t, logPath, numberedLines(250))
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "logs", "--config", configPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 200 {
		t.Fatalf("line count = %d, want 200", len(lines))
	}
	if lines[0] != "line 051" || lines[199] != "line 250" {
		t.Fatalf("lines[0], lines[199] = %q, %q; want line 051, line 250", lines[0], lines[199])
	}
}

func TestAgentLogs_printsAllLines_whenAllFlagIsSet(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	logPath := filepath.Join(filepath.Dir(configPath), "agent.log")
	writeTestFile(t, logPath, numberedLines(250))
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "logs", "--config", configPath, "--all"}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 250 {
		t.Fatalf("line count = %d, want 250", len(lines))
	}
	if lines[0] != "line 001" || lines[249] != "line 250" {
		t.Fatalf("lines[0], lines[249] = %q, %q; want line 001, line 250", lines[0], lines[249])
	}
}

func TestAgentLogs_returnsClearError_whenLogIsMissing(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "logs", "--config", configPath}, &stdout, &stdout)

	// Then
	if err == nil {
		t.Fatal("Run() error = nil, want missing log failure")
	}
	if !strings.Contains(err.Error(), "agent log file was not found") {
		t.Fatalf("error = %v, want missing log message", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty output on failure", stdout.String())
	}
}

func TestAgentLogs_usesLogFlagOnlyForReadingRedirectedLog(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	configDirLog := filepath.Join(filepath.Dir(configPath), "agent.log")
	overrideLog := filepath.Join(t.TempDir(), "launchd.log")
	writeTestFile(t, configDirLog, "default log\n")
	writeTestFile(t, overrideLog, "redirected log\n")
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "logs", "--config", configPath, "--log", overrideLog}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != "redirected log\n" {
		t.Fatalf("stdout = %q, want redirected log", stdout.String())
	}
}

func numberedLines(count int) string {
	var body strings.Builder
	for i := 1; i <= count; i++ {
		_, _ = fmt.Fprintf(&body, "line %03d\n", i)
	}
	return body.String()
}

func writeTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
