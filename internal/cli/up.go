package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	upNow          = time.Now
	upSleep        = time.Sleep
	upWaitTimeout  = 60 * time.Second
	upPollInterval = 2 * time.Second
)

func runUp(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	agentBinary := flags.String("agent-binary", defaultLaunchdAgentBinaryPath, "agent binary path")
	plistPath := flags.String("plist", "", "launch agent plist path")
	statusPath := flags.String("status", "", "status file path")
	logPath := flags.String("log", "", "log file path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	exists, err := configExists(*configPath)
	if err != nil {
		return err
	}
	if !exists {
		_, err := fmt.Fprintln(stdout, "이 machine은 아직 Neul fleet에 연결되지 않았습니다.\n먼저 실행: neul login --server <origin>")
		return err
	}
	if _, err := readConfig(*configPath); err != nil {
		return err
	}
	plan, err := resolveLaunchdInstallPlan(*configPath, *agentBinary, *plistPath, *statusPath, *logPath)
	if err != nil {
		return err
	}
	upStartedAt := upNow().UTC()
	if agentServiceGOOS != "darwin" {
		return printUpFailure(stdout, "agent_not_running")
	}
	if err := installLaunchAgent(io.Discard, plan); err != nil {
		return printUpFailure(stdout, "agent_not_running")
	}
	return waitForRunLoopHeartbeat(stdout, plan.StatusPath, upStartedAt)
}

func waitForRunLoopHeartbeat(stdout io.Writer, statusPath string, upStartedAt time.Time) error {
	deadline := upStartedAt.Add(upWaitTimeout)
	latestFreshError := ""
	for {
		status := readLocalAgentStatus(statusPath)
		if status.connectedSince(upStartedAt) {
			_, err := fmt.Fprintln(stdout, "connected")
			return err
		}
		if kind := status.freshErrorKindSince(upStartedAt); kind != "" {
			latestFreshError = kind
		}
		if !upNow().UTC().Before(deadline) {
			if latestFreshError != "" {
				return printUpFailure(stdout, mapHeartbeatError(latestFreshError))
			}
			return printUpFailure(stdout, "local_heartbeat_missing")
		}
		upSleep(upPollInterval)
	}
}

func printUpFailure(stdout io.Writer, code string) error {
	_, _ = fmt.Fprintf(stdout, "%s\n", code)
	return errors.New(code)
}

func mapHeartbeatError(kind string) string {
	switch kind {
	case "auth_failure":
		return "auth_invalid"
	case "connection_failure":
		return "server_unreachable"
	case "server_failure":
		return "server_error"
	case "rate_limited":
		return "rate_limited"
	default:
		return "local_heartbeat_missing"
	}
}

func (s localAgentStatus) connectedSince(startedAt time.Time) bool {
	if s.Mode != "run_loop" || s.hasLastError() {
		return false
	}
	heartbeatAt, ok := parseStatusTime(s.LastHeartbeatAt)
	return ok && !heartbeatAt.Before(startedAt)
}

func (s localAgentStatus) freshErrorKindSince(startedAt time.Time) string {
	if s.Mode != "run_loop" {
		return ""
	}
	attemptedAt, ok := parseStatusTime(s.LastHeartbeatAttempt)
	if !ok || attemptedAt.Before(startedAt) || !s.hasLastError() {
		return ""
	}
	var object struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(s.LastError, &object); err != nil {
		return ""
	}
	return strings.TrimSpace(object.Kind)
}

func (s localAgentStatus) hasLastError() bool {
	if len(s.LastError) == 0 || string(s.LastError) == "null" {
		return false
	}
	var object struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(s.LastError, &object); err == nil {
		return strings.TrimSpace(object.Kind) != "" || strings.TrimSpace(object.Message) != ""
	}
	var message string
	if err := json.Unmarshal(s.LastError, &message); err == nil {
		return strings.TrimSpace(message) != ""
	}
	return true
}

func parseStatusTime(raw string) (time.Time, bool) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
