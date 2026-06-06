package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout)
	case "agent":
		return runAgent(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pairCode := flags.String("pair", "", "pairing code")
	serverURL := flags.String("server", "", "server URL")
	configDir := flags.String("config-dir", defaultConfigDir(), "config directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *pairCode == "" || *serverURL == "" {
		return errors.New("init requires --pair and --server")
	}

	claim, err := claimPairingCode(*serverURL, *pairCode)
	if err != nil {
		return err
	}
	config := Config{
		ServerURL:    strings.TrimRight(*serverURL, "/"),
		MachineID:    claim.MachineID,
		MachineToken: claim.MachineToken,
	}
	if err := writeConfig(filepath.Join(*configDir, configFileName), config); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Machine paired: %s\n", claim.MachineID)
	return err
}

func runAgent(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("agent subcommand is required")
	}
	switch args[0] {
	case "enroll":
		return runAgentEnroll(args[1:], stdout)
	case "install":
		return runAgentInstall(args[1:], stdout)
	case "status":
		return runAgentStatus(args[1:], stdout)
	case "logs":
		return runAgentLogs(args[1:], stdout)
	default:
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

func runAgentInstall(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dryRun := flags.Bool("dry-run", false, "preview service install")
	configPath := flags.String("config", defaultConfigPath(), "config path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*dryRun {
		return errors.New("real service installation is out of MVP; pass --dry-run")
	}
	if _, err := readConfig(*configPath); err != nil {
		return err
	}
	serviceKind := "systemd"
	if runtime.GOOS == "darwin" {
		serviceKind = "launchd"
	}
	_, err := fmt.Fprintf(stdout, "Dry run: would install neul-agent using %s with config %s\n", serviceKind, *configPath)
	return err
}

func runAgentStatus(args []string, stdout io.Writer) error {
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
	statusPath := filepath.Join(filepath.Dir(*configPath), "status.json")
	statusBody := "unknown"
	if body, err := os.ReadFile(statusPath); err == nil {
		statusBody = strings.TrimSpace(string(body))
	}
	_, err = fmt.Fprintf(stdout, "Machine: %s\nStatus: %s\n", config.MachineID, statusBody)
	return err
}

func runAgentLogs(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent logs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := readConfig(*configPath); err != nil {
		return err
	}
	logPath := filepath.Join(filepath.Dir(*configPath), "agent.log")
	body, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("agent log file was not found")
		}
		return fmt.Errorf("read logs: %w", err)
	}
	_, err = stdout.Write(body)
	return err
}

type claimResponse struct {
	MachineID    string `json:"machineId"`
	MachineToken string `json:"machineToken"`
}

func claimPairingCode(serverURL string, pairCode string) (claimResponse, error) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	requestBody := map[string]interface{}{
		"code": pairCode,
		"machine": map[string]string{
			"name":         hostname,
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"agentVersion": "0.1.0",
		},
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return claimResponse{}, fmt.Errorf("encode claim: %w", err)
	}
	url := strings.TrimRight(serverURL, "/") + "/api/pair/claim"
	response, err := http.Post(url, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return claimResponse{}, fmt.Errorf("claim pairing code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return claimResponse{}, decodeClaimError(response.Body)
	}
	var claim claimResponse
	if err := json.NewDecoder(response.Body).Decode(&claim); err != nil {
		return claimResponse{}, fmt.Errorf("decode claim response: %w", err)
	}
	if claim.MachineID == "" || claim.MachineToken == "" {
		return claimResponse{}, errors.New("pairing response did not include machine credentials")
	}
	return claim, nil
}

func decodeClaimError(body io.Reader) error {
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return fmt.Errorf("pairing claim failed")
	}
	switch response.Error.Code {
	case "pairing_code_expired":
		return errors.New("pairing code expired")
	case "code_used":
		return errors.New("pairing code already used")
	default:
		if response.Error.Message != "" {
			return fmt.Errorf("pairing claim failed: %s", response.Error.Message)
		}
		return fmt.Errorf("pairing claim failed: %s", response.Error.Code)
	}
}
