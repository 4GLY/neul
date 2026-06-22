package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = "config.json"

type Config struct {
	ServerURL    string `json:"serverURL"`
	MachineID    string `json:"machineId"`
	MachineToken string `json:"machineToken"`
}

func writeConfig(path string, config Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}

func readConfig(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var config Config
	if err := json.Unmarshal(body, &config); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	return config, nil
}

func configExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, fmt.Errorf("check config: %w", err)
	}
}

func defaultConfigPath() string {
	return filepath.Join(defaultConfigDir(), configFileName)
}

func defaultConfigDir() string {
	if configDir := os.Getenv("NEUL_CONFIG_DIR"); configDir != "" {
		return configDir
	}
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return ".neul"
	}
	return filepath.Join(userConfigDir, "neul")
}
