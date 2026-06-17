package cli

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchdRenderPlist_includesAgentContract_whenPathsAreProvided(t *testing.T) {
	// Given
	config := launchdAgentConfig{
		Label:           launchdAgentLabel,
		PlistPath:       "/Users/alice/Library/LaunchAgents/com.4gly.neul.agent.plist",
		AgentBinaryPath: "/opt/neul/bin/neul-agent",
		ConfigPath:      "/Users/alice/.config/neul/config.json",
		StatusPath:      "/Users/alice/.config/neul/status.json",
		LogPath:         "/Users/alice/.config/neul/agent.log",
	}

	// When
	body, err := renderLaunchdAgentPlist(config)

	// Then
	if err != nil {
		t.Fatalf("renderLaunchdAgentPlist() error = %v", err)
	}
	plist := parseLaunchdPlist(t, body)
	if plist["Label"] != launchdAgentLabel {
		t.Fatalf("Label = %v, want %s", plist["Label"], launchdAgentLabel)
	}
	if got := plist["ProgramArguments"]; !stringSliceEqual(got, []string{
		"/opt/neul/bin/neul-agent",
		"--config",
		"/Users/alice/.config/neul/config.json",
		"--status",
		"/Users/alice/.config/neul/status.json",
	}) {
		t.Fatalf("ProgramArguments = %#v, want agent config/status args", got)
	}
	if plist["RunAtLoad"] != true {
		t.Fatalf("RunAtLoad = %v, want true", plist["RunAtLoad"])
	}
	if plist["KeepAlive"] != true {
		t.Fatalf("KeepAlive = %v, want true", plist["KeepAlive"])
	}
	if plist["StandardOutPath"] != "/Users/alice/.config/neul/agent.log" {
		t.Fatalf("StandardOutPath = %v, want log path", plist["StandardOutPath"])
	}
	if plist["StandardErrorPath"] != "/Users/alice/.config/neul/agent.log" {
		t.Fatalf("StandardErrorPath = %v, want log path", plist["StandardErrorPath"])
	}
	if strings.Contains(string(body), "--log") {
		t.Fatalf("plist included --log: %s", string(body))
	}
	if strings.Contains(string(body), "machineToken") || strings.Contains(string(body), "mtn_") {
		t.Fatalf("plist leaked machine token material: %s", string(body))
	}
}

func TestLaunchdDefaultAgentConfig_usesExpectedPaths_whenEnvironmentSelectsConfigDir(t *testing.T) {
	// Given
	homeDir := t.TempDir()
	configDir := filepath.Join(t.TempDir(), "neul-config")
	t.Setenv("HOME", homeDir)
	t.Setenv("NEUL_CONFIG_DIR", configDir)

	// When
	config, err := defaultLaunchdAgentConfig()

	// Then
	if err != nil {
		t.Fatalf("defaultLaunchdAgentConfig() error = %v", err)
	}
	if config.Label != launchdAgentLabel {
		t.Fatalf("Label = %s, want %s", config.Label, launchdAgentLabel)
	}
	if config.PlistPath != filepath.Join(homeDir, "Library", "LaunchAgents", "com.4gly.neul.agent.plist") {
		t.Fatalf("PlistPath = %s, want LaunchAgents plist under HOME", config.PlistPath)
	}
	if config.AgentBinaryPath != defaultLaunchdAgentBinaryPath {
		t.Fatalf("AgentBinaryPath = %s, want default binary path", config.AgentBinaryPath)
	}
	if config.ConfigPath != filepath.Join(configDir, "config.json") {
		t.Fatalf("ConfigPath = %s, want selected config dir config.json", config.ConfigPath)
	}
	if config.StatusPath != filepath.Join(configDir, "status.json") {
		t.Fatalf("StatusPath = %s, want selected config dir status.json", config.StatusPath)
	}
	if config.LogPath != filepath.Join(configDir, "agent.log") {
		t.Fatalf("LogPath = %s, want selected config dir agent.log", config.LogPath)
	}
}

func TestLaunchdRenderPlist_rejectsMalformedInput_whenRequiredPathIsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		config launchdAgentConfig
	}{
		{
			name: "missing plist path",
			config: launchdAgentConfig{
				Label:           launchdAgentLabel,
				AgentBinaryPath: "/usr/local/libexec/neul-agent",
				ConfigPath:      "/tmp/config.json",
				StatusPath:      "/tmp/status.json",
				LogPath:         "/tmp/agent.log",
			},
		},
		{
			name: "missing status path",
			config: launchdAgentConfig{
				Label:           launchdAgentLabel,
				PlistPath:       "/tmp/com.4gly.neul.agent.plist",
				AgentBinaryPath: "/usr/local/libexec/neul-agent",
				ConfigPath:      "/tmp/config.json",
				LogPath:         "/tmp/agent.log",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := renderLaunchdAgentPlist(tt.config)

			// Then
			if !errors.Is(err, errLaunchdMissingPath) {
				t.Fatalf("renderLaunchdAgentPlist() error = %v, want errLaunchdMissingPath", err)
			}
		})
	}
}

func TestLaunchdLaunchctlCommands_constructExpectedArgv_whenInputsAreValid(t *testing.T) {
	// Given
	target := "gui/501"
	plistPath := "/Users/alice/Library/LaunchAgents/com.4gly.neul.agent.plist"

	// When
	bootstrap, bootstrapErr := launchctlBootstrapCommand(target, plistPath)
	bootout, bootoutErr := launchctlBootoutCommand(target, launchdAgentLabel)

	// Then
	if bootstrapErr != nil {
		t.Fatalf("launchctlBootstrapCommand() error = %v", bootstrapErr)
	}
	if !stringSliceEqual(bootstrap, []string{"launchctl", "bootstrap", target, plistPath}) {
		t.Fatalf("bootstrap command = %#v, want launchctl bootstrap argv", bootstrap)
	}
	if bootoutErr != nil {
		t.Fatalf("launchctlBootoutCommand() error = %v", bootoutErr)
	}
	if !stringSliceEqual(bootout, []string{"launchctl", "bootout", "gui/501/com.4gly.neul.agent"}) {
		t.Fatalf("bootout command = %#v, want launchctl bootout argv", bootout)
	}
}

func TestLaunchdManualQA_writesPlistArtifact_whenEnvRequests(t *testing.T) {
	// Given
	outputPath := os.Getenv("NEUL_LAUNCHD_MANUAL_QA_PLIST")
	if outputPath == "" {
		t.Skip("NEUL_LAUNCHD_MANUAL_QA_PLIST is not set")
	}
	config := launchdAgentConfig{
		Label:           launchdAgentLabel,
		PlistPath:       "/Users/alice/Library/LaunchAgents/com.4gly.neul.agent.plist",
		AgentBinaryPath: "/custom/bin/neul-agent",
		ConfigPath:      "/custom/neul/config.json",
		StatusPath:      "/custom/neul/status.json",
		LogPath:         "/custom/neul/agent.log",
	}

	// When
	body, err := renderLaunchdAgentPlist(config)

	// Then
	if err != nil {
		t.Fatalf("renderLaunchdAgentPlist() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(outputPath, body, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func parseLaunchdPlist(t *testing.T, body []byte) map[string]any {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(body))
	parsed := make(map[string]any)
	var currentKey string
	for {
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return parsed
			}
			t.Fatalf("parse plist XML: %v", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			currentKey = decodeElementText(t, decoder, start)
		case "string":
			parsed[currentKey] = decodeElementText(t, decoder, start)
		case "true":
			parsed[currentKey] = true
		case "array":
			parsed[currentKey] = decodeStringArray(t, decoder)
		}
	}
}

func decodeElementText(t *testing.T, decoder *xml.Decoder, start xml.StartElement) string {
	t.Helper()
	var text string
	if err := decoder.DecodeElement(&text, &start); err != nil {
		t.Fatalf("decode %s: %v", start.Name.Local, err)
	}
	return text
}

func decodeStringArray(t *testing.T, decoder *xml.Decoder) []string {
	t.Helper()
	var values []string
	for {
		token, err := decoder.Token()
		if err != nil {
			t.Fatalf("decode array: %v", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "string" {
				values = append(values, decodeElementText(t, decoder, typed))
			}
		case xml.EndElement:
			if typed.Name.Local == "array" {
				return values
			}
		}
	}
}

func stringSliceEqual(got any, want []string) bool {
	gotSlice, ok := got.([]string)
	if !ok {
		return false
	}
	if len(gotSlice) != len(want) {
		return false
	}
	for i := range gotSlice {
		if gotSlice[i] != want[i] {
			return false
		}
	}
	return true
}
