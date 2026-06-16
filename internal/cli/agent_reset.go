package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

var runLaunchctlCommand = defaultRunLaunchctlCommand

func runAgentReset(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	machineID := flags.String("machine-id", "", "machine id confirmation")
	plistPath := flags.String("plist", "", "launch agent plist path")
	statusPath := flags.String("status", "", "status file path")
	logPath := flags.String("log", "", "log file path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *machineID == "" {
		return errors.New("reset requires machine id confirmation")
	}
	if !isValidMachineIDConfirmation(*machineID) {
		return fmt.Errorf("machine id confirmation %q is malformed", *machineID)
	}
	if config, err := readConfig(*configPath); err == nil {
		if *machineID != config.MachineID {
			return fmt.Errorf("machine id confirmation %q does not match config machine %q", *machineID, config.MachineID)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	paths, err := resolveLaunchdRemovalPaths(*configPath, *plistPath, *statusPath, *logPath)
	if err != nil {
		return err
	}
	if err := uninstallLaunchAgent(paths.plist); err != nil {
		return err
	}
	if err := removeLocalAgentState(*configPath, paths.status, paths.log); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Reset local agent state for machine %s\n", *machineID)
	return err
}

func runAgentUninstall(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent uninstall", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	removeState := flags.Bool("remove-state", false, "also remove local state")
	plistPath := flags.String("plist", "", "launch agent plist path")
	statusPath := flags.String("status", "", "status file path")
	logPath := flags.String("log", "", "log file path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *removeState {
		return errors.New("uninstall --remove-state is not implemented; use agent reset with --machine-id")
	}
	paths, err := resolveLaunchdRemovalPaths(*configPath, *plistPath, *statusPath, *logPath)
	if err != nil {
		return err
	}
	if err := uninstallLaunchAgent(paths.plist); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Uninstalled LaunchAgent %s and removed plist %s\n", launchdAgentLabel, paths.plist)
	return err
}

// launchdRemovalPaths holds the resolved plist/status/log paths that uninstall
// and reset operate on. They mirror the deterministic defaults that install
// derives so a service installed with custom paths can be torn down by passing
// the same overrides.
type launchdRemovalPaths struct {
	plist  string
	status string
	log    string
}

// resolveLaunchdRemovalPaths fills empty overrides with the same deterministic
// defaults install uses: plist defaults to the user LaunchAgents location while
// status/log default to siblings of the selected config. This keeps uninstall
// and reset consistent with the install/status/log path rule.
func resolveLaunchdRemovalPaths(configPath, plistPath, statusPath, logPath string) (launchdRemovalPaths, error) {
	if configPath == "" {
		return launchdRemovalPaths{}, fmt.Errorf("config: %w", errLaunchdMissingPath)
	}
	configDir := filepath.Dir(configPath)

	resolvedPlist := plistPath
	if resolvedPlist == "" {
		defaultPlist, err := defaultLaunchdPlistPath()
		if err != nil {
			return launchdRemovalPaths{}, err
		}
		resolvedPlist = defaultPlist
	}
	resolvedStatus := statusPath
	if resolvedStatus == "" {
		resolvedStatus = filepath.Join(configDir, "status.json")
	}
	resolvedLog := logPath
	if resolvedLog == "" {
		resolvedLog = filepath.Join(configDir, "agent.log")
	}
	return launchdRemovalPaths{plist: resolvedPlist, status: resolvedStatus, log: resolvedLog}, nil
}

func removeLocalAgentState(configPath, statusPath, logPath string) error {
	for _, path := range []string{
		configPath,
		statusPath,
		logPath,
	} {
		if err := removeIfExists(path); err != nil {
			return err
		}
	}
	if filepath.Clean(configPath) == filepath.Clean(defaultConfigPath()) {
		if err := os.Remove(defaultConfigDir()); err != nil && !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTEMPTY) && !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("remove empty config directory: %w", err)
		}
	}
	return nil
}

func uninstallLaunchAgent(plistPath string) error {
	// launchd is macOS-only. On other platforms there is no launchctl job to
	// unload, so skip the bootout and still remove the selected plist/local
	// state so reset/uninstall remain usable everywhere.
	if agentServiceGOOS == "darwin" {
		command, err := launchctlBootoutCommand(defaultLaunchctlTarget(), launchdAgentLabel)
		if err != nil {
			return err
		}
		output, err := runLaunchctlCommand(command)
		if err != nil && !isAlreadyUnloadedLaunchAgent(output) {
			return fmt.Errorf("bootout launch agent %s: %w", launchdAgentLabel, err)
		}
	}
	if err := removeIfExists(plistPath); err != nil {
		return err
	}
	return nil
}

func defaultRunLaunchctlCommand(command []string) ([]byte, error) {
	if len(command) == 0 {
		return nil, errors.New("launchctl command is required")
	}
	output, err := exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s: %w", strings.Join(command, " "), err)
	}
	return output, nil
}

func isAlreadyUnloadedLaunchAgent(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "no such process") ||
		strings.Contains(text, "could not find service") ||
		strings.Contains(text, "not found")
}

func isValidMachineIDConfirmation(machineID string) bool {
	return strings.HasPrefix(machineID, "machine_") && strings.TrimSpace(machineID) == machineID && len(machineID) > len("machine_")
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
