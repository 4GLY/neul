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
	"time"
)

var agentServiceGOOS = runtime.GOOS
var cliHTTPClient = &http.Client{Timeout: 30 * time.Second}

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout)
	case "login":
		return runLogin(args[1:], stdout)
	case "up":
		return runUp(args[1:], stdout)
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
	case "start":
		return runAgentStart(args[1:], stdout)
	case "reset":
		return runAgentReset(args[1:], stdout)
	case "uninstall":
		return runAgentUninstall(args[1:], stdout)
	case "status":
		return runAgentStatus(args[1:], stdout)
	case "logs":
		return runAgentLogs(args[1:], stdout)
	default:
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

func runAgentInstall(args []string, stdout io.Writer) error {
	return runAgentInstallCommand(args, stdout)
}

func runAgentStart(args []string, _ io.Writer) error {
	flags := flag.NewFlagSet("agent start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := readConfig(*configPath); err != nil {
		return err
	}
	if agentServiceGOOS != "darwin" {
		return fmt.Errorf("agent start is unsupported on %s", agentServiceGOOS)
	}
	return errors.New("agent start is not implemented")
}

func runAgentStatus(args []string, stdout io.Writer) error {
	return runAgentStatusCommand(args, stdout)
}

func runAgentLogs(args []string, stdout io.Writer) error {
	return runAgentLogsCommand(args, stdout)
}

type claimResponse struct {
	MachineID    string `json:"machineId"`
	MachineToken string `json:"machineToken"`
}

type machineMetadata struct {
	Name         string `json:"name"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	AgentVersion string `json:"agentVersion"`
}

func claimPairingCode(serverURL string, pairCode string) (claimResponse, error) {
	machine := currentMachineMetadata()
	requestBody := struct {
		Code    string          `json:"code"`
		Machine machineMetadata `json:"machine"`
	}{
		Code:    pairCode,
		Machine: machine,
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return claimResponse{}, fmt.Errorf("encode claim: %w", err)
	}
	url := strings.TrimRight(serverURL, "/") + "/api/pair/claim"
	response, err := cliHTTPClient.Post(url, "application/json", bytes.NewReader(encoded))
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

func currentMachineMetadata() machineMetadata {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	return machineMetadata{
		Name:         hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: "0.1.0",
	}
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

type apiError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *apiError) Error() string {
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

func decodeAPIError(body io.Reader, statusCode int) error {
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return &apiError{Code: "server_polling_failed", StatusCode: statusCode}
	}
	if response.Error.Code == "" {
		response.Error.Code = "server_polling_failed"
	}
	return &apiError{Code: response.Error.Code, Message: response.Error.Message, StatusCode: statusCode}
}
