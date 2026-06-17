package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	launchdAgentLabel             = "com.4gly.neul.agent"
	defaultLaunchdAgentBinaryPath = "/usr/local/libexec/neul-agent"
)

var errLaunchdMissingPath = errors.New("launchd path is required")

type launchdAgentConfig struct {
	Label           string
	PlistPath       string
	AgentBinaryPath string
	ConfigPath      string
	StatusPath      string
	LogPath         string
}

func defaultLaunchdAgentConfig() (launchdAgentConfig, error) {
	plistPath, err := defaultLaunchdPlistPath()
	if err != nil {
		return launchdAgentConfig{}, err
	}
	configPath := defaultConfigPath()
	configDir := filepath.Dir(configPath)
	return launchdAgentConfig{
		Label:           launchdAgentLabel,
		PlistPath:       plistPath,
		AgentBinaryPath: defaultLaunchdAgentBinaryPath,
		ConfigPath:      configPath,
		StatusPath:      filepath.Join(configDir, "status.json"),
		LogPath:         filepath.Join(configDir, "agent.log"),
	}, nil
}

func defaultLaunchdPlistPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if homeDir == "" {
		return "", fmt.Errorf("home directory: %w", errLaunchdMissingPath)
	}
	return filepath.Join(homeDir, "Library", "LaunchAgents", launchdAgentLabel+".plist"), nil
}

func validateLaunchdAgentConfig(config launchdAgentConfig) error {
	requiredPaths := []struct {
		name string
		path string
	}{
		{name: "plist", path: config.PlistPath},
		{name: "binary", path: config.AgentBinaryPath},
		{name: "config", path: config.ConfigPath},
		{name: "status", path: config.StatusPath},
		{name: "log", path: config.LogPath},
	}
	for _, requiredPath := range requiredPaths {
		if requiredPath.path == "" {
			return fmt.Errorf("%s: %w", requiredPath.name, errLaunchdMissingPath)
		}
	}
	if config.Label != launchdAgentLabel {
		return fmt.Errorf("label %q: want %s", config.Label, launchdAgentLabel)
	}
	return nil
}

func renderLaunchdAgentPlist(config launchdAgentConfig) ([]byte, error) {
	if err := validateLaunchdAgentConfig(config); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	body.WriteString(`<plist version="1.0">` + "\n")
	body.WriteString("<dict>\n")
	writePlistString(&body, "Label", config.Label)
	writePlistStringArray(&body, "ProgramArguments", []string{
		config.AgentBinaryPath,
		"--config",
		config.ConfigPath,
		"--status",
		config.StatusPath,
	})
	writePlistBool(&body, "RunAtLoad")
	writePlistBool(&body, "KeepAlive")
	writePlistString(&body, "StandardOutPath", config.LogPath)
	writePlistString(&body, "StandardErrorPath", config.LogPath)
	body.WriteString("</dict>\n")
	body.WriteString("</plist>\n")
	return body.Bytes(), nil
}

func launchctlBootstrapCommand(target string, plistPath string) ([]string, error) {
	if target == "" {
		return nil, fmt.Errorf("target: %w", errLaunchdMissingPath)
	}
	if plistPath == "" {
		return nil, fmt.Errorf("plist: %w", errLaunchdMissingPath)
	}
	return []string{"launchctl", "bootstrap", target, plistPath}, nil
}

func launchctlBootoutCommand(target string, label string) ([]string, error) {
	if target == "" {
		return nil, fmt.Errorf("target: %w", errLaunchdMissingPath)
	}
	if label == "" {
		return nil, fmt.Errorf("label: %w", errLaunchdMissingPath)
	}
	return []string{"launchctl", "bootout", target + "/" + label}, nil
}

func launchctlPrintCommand(target string, label string) ([]string, error) {
	if target == "" {
		return nil, fmt.Errorf("target: %w", errLaunchdMissingPath)
	}
	if label == "" {
		return nil, fmt.Errorf("label: %w", errLaunchdMissingPath)
	}
	return []string{"launchctl", "print", target + "/" + label}, nil
}

func launchctlKickstartCommand(target string, label string) ([]string, error) {
	if target == "" {
		return nil, fmt.Errorf("target: %w", errLaunchdMissingPath)
	}
	if label == "" {
		return nil, fmt.Errorf("label: %w", errLaunchdMissingPath)
	}
	return []string{"launchctl", "kickstart", "-k", target + "/" + label}, nil
}

func defaultLaunchctlTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func writePlistString(body *bytes.Buffer, key string, value string) {
	body.WriteString("\t<key>")
	writeXMLEscaped(body, key)
	body.WriteString("</key>\n\t<string>")
	writeXMLEscaped(body, value)
	body.WriteString("</string>\n")
}

func writePlistStringArray(body *bytes.Buffer, key string, values []string) {
	body.WriteString("\t<key>")
	writeXMLEscaped(body, key)
	body.WriteString("</key>\n\t<array>\n")
	for _, value := range values {
		body.WriteString("\t\t<string>")
		writeXMLEscaped(body, value)
		body.WriteString("</string>\n")
	}
	body.WriteString("\t</array>\n")
}

func writePlistBool(body *bytes.Buffer, key string) {
	body.WriteString("\t<key>")
	writeXMLEscaped(body, key)
	body.WriteString("</key>\n\t<true/>\n")
}

func writeXMLEscaped(body *bytes.Buffer, value string) {
	body.WriteString(xmlEscaper.Replace(value))
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&#34;",
	"'", "&#39;",
)
