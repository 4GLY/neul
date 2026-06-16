package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var errLaunchAgentStateUnknown = errors.New("launch agent state unknown")

type launchAgentState string

const (
	launchAgentStateLoaded   launchAgentState = "loaded"
	launchAgentStateUnloaded launchAgentState = "unloaded"
	launchAgentStateUnknown  launchAgentState = "unknown"
)

var (
	agentStatusGOOS       = runtime.GOOS
	probeLaunchAgentState = defaultProbeLaunchAgentState
)

type localAgentStatus struct {
	LastHeartbeatAt string          `json:"lastHeartbeatAt"`
	LastError       json.RawMessage `json:"lastError"`
}

func runAgentStatusCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readConfig(*configPath)
	if err != nil {
		return err
	}
	status := readLocalAgentStatus(filepath.Join(filepath.Dir(*configPath), "status.json"))
	launchState := launchAgentStateUnknown
	if agentStatusGOOS == "darwin" {
		probed, err := probeLaunchAgentState(*configPath)
		if err == nil {
			launchState = probed
		}
	}
	heartbeat := status.LastHeartbeatAt
	if heartbeat == "" {
		heartbeat = "unknown"
	}
	_, err = fmt.Fprintf(stdout,
		"Machine: %s\nConfig: %s\nLaunchAgent: %s\nHeartbeat: %s\nLast error: %s\n",
		config.MachineID,
		*configPath,
		launchState,
		heartbeat,
		status.lastError(),
	)
	return err
}

func readLocalAgentStatus(path string) localAgentStatus {
	body, err := os.ReadFile(path)
	if err != nil {
		return localAgentStatus{}
	}
	var status localAgentStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return localAgentStatus{}
	}
	return status
}

func defaultProbeLaunchAgentState(string) (launchAgentState, error) {
	if _, err := exec.LookPath("launchctl"); err != nil {
		return launchAgentStateUnknown, fmt.Errorf("find launchctl: %w", errLaunchAgentStateUnknown)
	}
	target := defaultLaunchctlTarget() + "/" + launchdAgentLabel
	if err := exec.Command("launchctl", "print", target).Run(); err != nil {
		return launchAgentStateUnloaded, nil
	}
	return launchAgentStateLoaded, nil
}

func (s localAgentStatus) lastError() string {
	if len(s.LastError) == 0 || string(s.LastError) == "null" {
		return "none"
	}
	var message string
	if err := json.Unmarshal(s.LastError, &message); err == nil {
		if strings.TrimSpace(message) == "" {
			return "none"
		}
		return message
	}
	var object struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(s.LastError, &object); err == nil && strings.TrimSpace(object.Message) != "" {
		return object.Message
	}
	return "none"
}
