package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentUninstall_removesSelectedPlistByDefault(t *testing.T) {
	// Given
	restoreGOOS := forceAgentServiceGOOS(t, "darwin")
	defer restoreGOOS()
	configDir := t.TempDir()
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	mustWriteFile(t, plistPath, "plist")
	var commands [][]string
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		commands = append(commands, append([]string(nil), command...))
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "uninstall", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertLaunchctlBootout(t, commands)
	assertPathMissing(t, plistPath)
	for _, name := range []string{configFileName, "status.json", "agent.log"} {
		assertPathExists(t, filepath.Join(configDir, name))
	}
	if !strings.Contains(stdout.String(), plistPath) {
		t.Fatalf("stdout = %s, want plist receipt", stdout.String())
	}
}

func TestAgentUninstall_removesSelectedCustomPlistAndKeepsExternalParent(t *testing.T) {
	// Given a service installed with a custom plist location.
	restoreGOOS := forceAgentServiceGOOS(t, "darwin")
	defer restoreGOOS()
	rootDir := t.TempDir()
	configDir := filepath.Join(rootDir, "config")
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistDir := filepath.Join(rootDir, "LaunchAgents")
	plistPath := filepath.Join(plistDir, "com.4gly.neul.agent.plist")
	mustWriteFile(t, plistPath, "custom plist")
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "uninstall", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertPathMissing(t, plistPath)
	assertPathExists(t, plistDir)
	for _, name := range []string{configFileName, "status.json", "agent.log"} {
		assertPathExists(t, filepath.Join(configDir, name))
	}
}

func TestAgentUninstall_removesSelectedPlistAndPreservesState_whenNotDarwin(t *testing.T) {
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
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		t.Fatalf("launchctl should not run on %s, got %#v", agentServiceGOOS, command)
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "uninstall", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertPathMissing(t, plistPath)
	for _, name := range []string{configFileName, "status.json", "agent.log"} {
		assertPathExists(t, filepath.Join(configDir, name))
	}
	if !strings.Contains(stdout.String(), plistPath) {
		t.Fatalf("stdout = %s, want plist receipt", stdout.String())
	}
}

func TestAgentUninstall_missingPlistDoesNotRemoveState(t *testing.T) {
	// Given a selected plist path that does not exist.
	restoreGOOS := forceAgentServiceGOOS(t, "darwin")
	defer restoreGOOS()
	configDir := t.TempDir()
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		return []byte("Boot-out failed: 3: No such process"), errors.New("exit status 3")
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "uninstall", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, name := range []string{configFileName, "status.json", "agent.log"} {
		assertPathExists(t, filepath.Join(configDir, name))
	}
}

func TestAgentUninstall_isIdempotent_whenRepeated(t *testing.T) {
	// Given
	restoreGOOS := forceAgentServiceGOOS(t, "darwin")
	defer restoreGOOS()
	configDir := t.TempDir()
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	mustWriteFile(t, plistPath, "plist")
	var calls int
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return []byte("Boot-out failed: 3: No such process"), errors.New("exit status 3")
	})
	defer restore()
	var stdout strings.Builder

	// When
	firstErr := Run([]string{"agent", "uninstall", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)
	secondErr := Run([]string{"agent", "uninstall", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)

	// Then
	if firstErr != nil {
		t.Fatalf("first Run() error = %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("second Run() error = %v", secondErr)
	}
	if calls != 2 {
		t.Fatalf("launchctl calls = %d, want 2", calls)
	}
	assertPathMissing(t, plistPath)
	for _, name := range []string{configFileName, "status.json", "agent.log"} {
		assertPathExists(t, filepath.Join(configDir, name))
	}
}

func TestAgentReset_isIdempotentAndDoesNotRemoveInstalledBinaries_whenRepeated(t *testing.T) {
	// Given
	restoreGOOS := forceAgentServiceGOOS(t, "darwin")
	defer restoreGOOS()
	rootDir := t.TempDir()
	configDir := filepath.Join(rootDir, "config")
	configPath := writeResetTestState(t, configDir, "machine_1")
	plistPath := filepath.Join(configDir, "neul-agent.plist")
	mustWriteFile(t, plistPath, "plist")
	binPath := filepath.Join(rootDir, "usr", "local", "bin", "neul")
	agentPath := filepath.Join(rootDir, "usr", "local", "libexec", "neul-agent")
	for _, path := range []string{binPath, agentPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte("installed"), 0o755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	var calls int
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return []byte("Boot-out failed: 3: No such process"), errors.New("exit status 3")
	})
	defer restore()
	var stdout strings.Builder

	// When
	firstErr := Run([]string{"agent", "reset", "--config", configPath, "--plist", plistPath, "--machine-id", "machine_1"}, &stdout, &stdout)
	secondErr := Run([]string{"agent", "reset", "--config", configPath, "--plist", plistPath, "--machine-id", "machine_1"}, &stdout, &stdout)

	// Then
	if firstErr != nil {
		t.Fatalf("first Run() error = %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("second Run() error = %v", secondErr)
	}
	if calls != 2 {
		t.Fatalf("launchctl calls = %d, want 2", calls)
	}
	for _, name := range []string{configFileName, "status.json", "agent.log", "neul-agent.plist"} {
		assertPathMissing(t, filepath.Join(configDir, name))
	}
	assertPathExists(t, binPath)
	assertPathExists(t, agentPath)
}

func stubLaunchctlCommand(t *testing.T, fn func([]string) ([]byte, error)) func() {
	t.Helper()
	previous := runLaunchctlCommand
	runLaunchctlCommand = fn
	return func() {
		runLaunchctlCommand = previous
	}
}

func forceAgentServiceGOOS(t *testing.T, goos string) func() {
	t.Helper()
	previous := agentServiceGOOS
	agentServiceGOOS = goos
	return func() {
		agentServiceGOOS = previous
	}
}

func assertLaunchctlBootout(t *testing.T, commands [][]string) {
	t.Helper()
	if len(commands) != 1 {
		t.Fatalf("launchctl commands = %#v, want one bootout command", commands)
	}
	want := []string{"launchctl", "bootout", defaultLaunchctlTarget() + "/" + launchdAgentLabel}
	if !stringSliceEqual(commands[0], want) {
		t.Fatalf("launchctl command = %#v, want %#v", commands[0], want)
	}
}
