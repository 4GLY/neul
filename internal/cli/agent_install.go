package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func runAgentInstallCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dryRun := flags.Bool("dry-run", false, "preview service install")
	configPath := flags.String("config", defaultConfigPath(), "config path")
	agentBinary := flags.String("agent-binary", defaultLaunchdAgentBinary(), "agent binary path")
	plistPath := flags.String("plist", "", "launch agent plist path")
	statusPath := flags.String("status", "", "status file path")
	logPath := flags.String("log", "", "log file path")
	if err := flags.Parse(args); err != nil {
		return err
	}

	plan, err := resolveLaunchdInstallPlan(*configPath, *agentBinary, *plistPath, *statusPath, *logPath)
	if err != nil {
		return err
	}
	if *dryRun {
		return printInstallDryRun(stdout, plan)
	}
	if agentServiceGOOS != "darwin" {
		return fmt.Errorf("agent install is unsupported on %s", agentServiceGOOS)
	}
	return installLaunchAgent(stdout, plan)
}

// resolveLaunchdInstallPlan fills empty install flags with the deterministic
// defaults derived from the selected config path, then validates the result so
// both dry-run preview and real install share the same resolved plan.
func resolveLaunchdInstallPlan(configPath, agentBinary, plistPath, statusPath, logPath string) (launchdAgentConfig, error) {
	if configPath == "" {
		return launchdAgentConfig{}, fmt.Errorf("config: %w", errLaunchdMissingPath)
	}
	configDir := filepath.Dir(configPath)

	resolvedPlist := plistPath
	if resolvedPlist == "" {
		defaultPlist, err := defaultLaunchdPlistPath()
		if err != nil {
			return launchdAgentConfig{}, err
		}
		resolvedPlist = defaultPlist
	}
	resolvedBinary := agentBinary
	if resolvedBinary == "" {
		resolvedBinary = defaultLaunchdAgentBinaryPath
	}
	resolvedStatus := statusPath
	if resolvedStatus == "" {
		resolvedStatus = filepath.Join(configDir, "status.json")
	}
	resolvedLog := logPath
	if resolvedLog == "" {
		resolvedLog = filepath.Join(configDir, "agent.log")
	}

	plan := launchdAgentConfig{
		Label:           launchdAgentLabel,
		PlistPath:       resolvedPlist,
		AgentBinaryPath: resolvedBinary,
		ConfigPath:      configPath,
		StatusPath:      resolvedStatus,
		LogPath:         resolvedLog,
	}
	if err := validateLaunchdAgentConfig(plan); err != nil {
		return launchdAgentConfig{}, err
	}
	return plan, nil
}

// printInstallDryRun emits a deterministic preview of the resolved install plan
// so callers can prove which plist/binary/config/status/log paths and launchctl
// commands would be used without touching the filesystem or launchd.
func printInstallDryRun(stdout io.Writer, plan launchdAgentConfig) error {
	target := defaultLaunchctlTarget()
	bootstrap, err := launchctlBootstrapCommand(target, plan.PlistPath)
	if err != nil {
		return err
	}
	kickstart, err := launchctlKickstartCommand(target, plan.Label)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout,
		"Dry run: would install LaunchAgent %s\n"+
			"  plist:   %s\n"+
			"  binary:  %s\n"+
			"  config:  %s\n"+
			"  status:  %s\n"+
			"  log:     %s\n"+
			"  bootstrap: %s\n"+
			"  kickstart: %s\n",
		plan.Label,
		plan.PlistPath,
		plan.AgentBinaryPath,
		plan.ConfigPath,
		plan.StatusPath,
		plan.LogPath,
		shellJoin(bootstrap),
		shellJoin(kickstart),
	)
	return err
}

// installLaunchAgent performs a deterministic, idempotent install: validate
// inputs, render the plist atomically, bootout any loaded job, bootstrap, and
// kickstart for install smoke. The plist is rolled back if bootstrap fails and
// no plist pre-existed.
func installLaunchAgent(stdout io.Writer, plan launchdAgentConfig) error {
	if err := ensureConfigFileExists(plan.ConfigPath); err != nil {
		return err
	}
	if err := ensureAgentBinaryExecutable(plan.AgentBinaryPath); err != nil {
		return err
	}
	body, err := renderLaunchdAgentPlist(plan)
	if err != nil {
		return err
	}

	_, statErr := os.Stat(plan.PlistPath)
	plistPreexisted := statErr == nil
	if err := writePlistAtomic(plan.PlistPath, body); err != nil {
		return err
	}

	target := defaultLaunchctlTarget()
	if launchAgentLoaded(target) {
		bootout, err := launchctlBootoutCommand(target, plan.Label)
		if err != nil {
			return err
		}
		if output, err := runLaunchctlCommand(bootout); err != nil && !isAlreadyUnloadedLaunchAgent(output) {
			return fmt.Errorf("bootout launch agent %s: %w", plan.Label, err)
		}
	}

	bootstrap, err := launchctlBootstrapCommand(target, plan.PlistPath)
	if err != nil {
		return err
	}
	if _, err := runLaunchctlCommand(bootstrap); err != nil {
		if !plistPreexisted {
			_ = removeIfExists(plan.PlistPath)
		}
		return fmt.Errorf("bootstrap launch agent %s: %w", plan.Label, err)
	}

	kickstart, err := launchctlKickstartCommand(target, plan.Label)
	if err != nil {
		return err
	}
	if _, err := runLaunchctlCommand(kickstart); err != nil {
		return fmt.Errorf("kickstart launch agent %s: %w", plan.Label, err)
	}

	_, err = fmt.Fprintf(stdout, "Installed LaunchAgent %s with plist %s\n", plan.Label, plan.PlistPath)
	return err
}

func launchAgentLoaded(target string) bool {
	command, err := launchctlPrintCommand(target, launchdAgentLabel)
	if err != nil {
		return false
	}
	if _, err := runLaunchctlCommand(command); err != nil {
		return false
	}
	return true
}

func ensureConfigFileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config %s does not exist; run neul agent enroll first", path)
		}
		return fmt.Errorf("stat config %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("config %s is a directory", path)
	}
	return nil
}

func ensureAgentBinaryExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("agent binary %s does not exist", path)
		}
		return fmt.Errorf("stat agent binary %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("agent binary %s is a directory", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("agent binary %s is not executable", path)
	}
	return nil
}

// writePlistAtomic renders the plist into a sibling temp file and renames it
// into place so a partially written plist is never observed by launchd.
func writePlistAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create plist directory %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".neul-agent-plist-*")
	if err != nil {
		return fmt.Errorf("create temp plist: %w", err)
	}
	tempPath := temp.Name()
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp plist: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp plist: %w", err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("chmod temp plist: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename temp plist into %s: %w", path, err)
	}
	return nil
}

func shellJoin(command []string) string {
	joined := ""
	for index, part := range command {
		if index > 0 {
			joined += " "
		}
		joined += part
	}
	return joined
}
