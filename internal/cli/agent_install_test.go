package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentInstallDryRun_resolvesDefaultAgentBinary_whenFlagOmitted(t *testing.T) {
	// Given
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configPath := writeTestConfig(t)
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "install", "--dry-run", "--config", configPath}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	configDir := filepath.Dir(configPath)
	wantPlist := filepath.Join(homeDir, "Library", "LaunchAgents", "com.4gly.neul.agent.plist")
	for _, want := range []string{
		"Dry run: would install LaunchAgent com.4gly.neul.agent\n",
		"binary:  " + defaultLaunchdAgentBinaryPath + "\n",
		"plist:   " + wantPlist + "\n",
		"config:  " + configPath + "\n",
		"status:  " + filepath.Join(configDir, "status.json") + "\n",
		"log:     " + filepath.Join(configDir, "agent.log") + "\n",
		"bootstrap: launchctl bootstrap " + defaultLaunchctlTarget() + " " + wantPlist + "\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if defaultLaunchdAgentBinaryPath != "/usr/local/libexec/neul-agent" {
		t.Fatalf("default agent binary = %s, want /usr/local/libexec/neul-agent", defaultLaunchdAgentBinaryPath)
	}
	assertPathMissing(t, wantPlist)
}

func TestAgentInstallDryRun_reflectsOverrideFlags(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	overrideDir := t.TempDir()
	plistPath := filepath.Join(overrideDir, "com.4gly.neul.agent.plist")
	statusPath := filepath.Join(overrideDir, "status.json")
	logPath := filepath.Join(overrideDir, "agent.log")
	agentBinary := filepath.Join(overrideDir, "neul-agent")
	var stdout strings.Builder

	// When
	err := Run([]string{
		"agent", "install", "--dry-run",
		"--config", configPath,
		"--agent-binary", agentBinary,
		"--plist", plistPath,
		"--status", statusPath,
		"--log", logPath,
	}, &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"plist:   " + plistPath + "\n",
		"binary:  " + agentBinary + "\n",
		"status:  " + statusPath + "\n",
		"log:     " + logPath + "\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	assertPathMissing(t, plistPath)
}

func TestAgentInstall_returnsUnsupportedWithoutWritingPlist_whenNotDarwin(t *testing.T) {
	// Given
	configPath := writeTestConfig(t)
	plistPath := filepath.Join(t.TempDir(), "com.4gly.neul.agent.plist")
	restore := overrideAgentServiceGOOS(t, "linux")
	defer restore()
	var stdout strings.Builder

	// When
	err := Run([]string{"agent", "install", "--config", configPath, "--plist", plistPath}, &stdout, &stdout)

	// Then
	if err == nil {
		t.Fatal("Run() error = nil, want unsupported failure")
	}
	if !strings.Contains(err.Error(), "agent install is unsupported on linux") {
		t.Fatalf("error = %v, want unsupported on linux", err)
	}
	assertPathMissing(t, plistPath)
}

func TestAgentInstall_writesPlistAndBootstraps_whenAgentNotLoaded(t *testing.T) {
	// Given
	plan := newDarwinInstallPlan(t)
	var commands [][]string
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		commands = append(commands, append([]string(nil), command...))
		if len(command) >= 2 && command[1] == "print" {
			return []byte("Could not find service"), errors.New("exit status 113")
		}
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run(plan.args(), &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertPathExists(t, plan.plistPath)
	body, readErr := os.ReadFile(plan.plistPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if !strings.Contains(string(body), plan.agentBinary) {
		t.Fatalf("plist = %s, want agent binary %s", body, plan.agentBinary)
	}
	verbs := launchctlVerbs(commands)
	if !stringSliceEqual(verbs, []string{"print", "bootstrap", "kickstart"}) {
		t.Fatalf("launchctl verbs = %#v, want print/bootstrap/kickstart", verbs)
	}
	if !strings.Contains(stdout.String(), "Installed LaunchAgent com.4gly.neul.agent") {
		t.Fatalf("stdout = %q, want install receipt", stdout.String())
	}
}

func TestAgentInstall_bootoutThenBootstrap_whenAlreadyLoaded(t *testing.T) {
	// Given
	plan := newDarwinInstallPlan(t)
	var commands [][]string
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		commands = append(commands, append([]string(nil), command...))
		return nil, nil // print succeeds -> job is loaded
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run(plan.args(), &stdout, &stdout)

	// Then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	verbs := launchctlVerbs(commands)
	if !stringSliceEqual(verbs, []string{"print", "bootout", "bootstrap", "kickstart"}) {
		t.Fatalf("launchctl verbs = %#v, want print/bootout/bootstrap/kickstart", verbs)
	}
}

func TestAgentInstall_rollsBackPlist_whenBootstrapFailsWithoutPreexistingPlist(t *testing.T) {
	// Given
	plan := newDarwinInstallPlan(t)
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		if len(command) >= 2 && command[1] == "print" {
			return []byte("Could not find service"), errors.New("exit status 113")
		}
		if len(command) >= 2 && command[1] == "bootstrap" {
			return []byte("Bootstrap failed: 5: Input/output error"), errors.New("exit status 5")
		}
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run(plan.args(), &stdout, &stdout)

	// Then
	if err == nil {
		t.Fatal("Run() error = nil, want bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap launch agent") {
		t.Fatalf("error = %v, want bootstrap failure", err)
	}
	assertPathMissing(t, plan.plistPath)
}

func TestAgentInstall_keepsPreexistingPlist_whenBootstrapFails(t *testing.T) {
	// Given
	plan := newDarwinInstallPlan(t)
	if err := os.MkdirAll(filepath.Dir(plan.plistPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(plan.plistPath, []byte("preexisting"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	restore := stubLaunchctlCommand(t, func(command []string) ([]byte, error) {
		if len(command) >= 2 && command[1] == "print" {
			return nil, nil // loaded
		}
		if len(command) >= 2 && command[1] == "bootstrap" {
			return []byte("Bootstrap failed"), errors.New("exit status 5")
		}
		return nil, nil
	})
	defer restore()
	var stdout strings.Builder

	// When
	err := Run(plan.args(), &stdout, &stdout)

	// Then
	if err == nil {
		t.Fatal("Run() error = nil, want bootstrap failure")
	}
	assertPathExists(t, plan.plistPath)
}

func TestAgentInstall_validatesInputs_whenConfigOrBinaryMissing(t *testing.T) {
	// Given
	restore := overrideAgentServiceGOOS(t, "darwin")
	defer restore()
	noopLaunchctl := stubLaunchctlCommand(t, func([]string) ([]byte, error) {
		t.Fatal("launchctl should not run before validation passes")
		return nil, nil
	})
	defer noopLaunchctl()

	t.Run("missing config", func(t *testing.T) {
		dir := t.TempDir()
		plistPath := filepath.Join(dir, "com.4gly.neul.agent.plist")
		var stdout strings.Builder
		err := Run([]string{
			"agent", "install",
			"--config", filepath.Join(dir, "config.json"),
			"--agent-binary", writeExecutableBinary(t, dir),
			"--plist", plistPath,
		}, &stdout, &stdout)
		if err == nil || !strings.Contains(err.Error(), "config") {
			t.Fatalf("error = %v, want config failure", err)
		}
		assertPathMissing(t, plistPath)
	})

	t.Run("agent binary not executable", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.json")
		if err := os.WriteFile(configPath, []byte(`{"machineId":"machine_1"}`), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		nonExec := filepath.Join(dir, "neul-agent")
		if err := os.WriteFile(nonExec, []byte("binary"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		plistPath := filepath.Join(dir, "com.4gly.neul.agent.plist")
		var stdout strings.Builder
		err := Run([]string{
			"agent", "install",
			"--config", configPath,
			"--agent-binary", nonExec,
			"--plist", plistPath,
		}, &stdout, &stdout)
		if err == nil || !strings.Contains(err.Error(), "not executable") {
			t.Fatalf("error = %v, want not executable failure", err)
		}
		assertPathMissing(t, plistPath)
	})
}

type darwinInstallPlan struct {
	configPath  string
	agentBinary string
	plistPath   string
	statusPath  string
	logPath     string
}

func (p darwinInstallPlan) args() []string {
	return []string{
		"agent", "install",
		"--config", p.configPath,
		"--agent-binary", p.agentBinary,
		"--plist", p.plistPath,
		"--status", p.statusPath,
		"--log", p.logPath,
	}
}

func newDarwinInstallPlan(t *testing.T) darwinInstallPlan {
	t.Helper()
	restore := overrideAgentServiceGOOS(t, "darwin")
	t.Cleanup(restore)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"machineId":"machine_1"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return darwinInstallPlan{
		configPath:  configPath,
		agentBinary: writeExecutableBinary(t, dir),
		plistPath:   filepath.Join(dir, "LaunchAgents", "com.4gly.neul.agent.plist"),
		statusPath:  filepath.Join(dir, "status.json"),
		logPath:     filepath.Join(dir, "agent.log"),
	}
}

func writeExecutableBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "neul-agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func launchctlVerbs(commands [][]string) []string {
	verbs := make([]string, 0, len(commands))
	for _, command := range commands {
		if len(command) >= 2 {
			verbs = append(verbs, command[1])
		}
	}
	return verbs
}

func overrideAgentServiceGOOS(t *testing.T, goos string) func() {
	t.Helper()
	previous := agentServiceGOOS
	agentServiceGOOS = goos
	return func() {
		agentServiceGOOS = previous
	}
}
