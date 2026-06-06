package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/4gly/neul/internal/agent"
)

func runAgentEnroll(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent enroll", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pairCode := flags.String("pair", "", "pairing code")
	serverURL := flags.String("server", "", "server URL")
	configDir := flags.String("config-dir", defaultConfigDir(), "config directory")
	configPath := flags.String("config", "", "config path")
	connectOnce := flags.Bool("connect-once", false, "run one agent tick after enrollment")
	force := flags.Bool("force", false, "overwrite existing local config")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *pairCode == "" || *serverURL == "" {
		return errors.New("agent enroll requires --pair and --server")
	}
	path := *configPath
	if path == "" {
		path = filepath.Join(*configDir, configFileName)
	}
	if !*force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check config: %w", err)
		}
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
	if err := writeConfig(path, config); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Machine enrolled: %s\nConfig: %s\n", claim.MachineID, path); err != nil {
		return err
	}
	if !*connectOnce {
		return nil
	}
	if _, err := fmt.Fprintln(stdout, "Connecting"); err != nil {
		return err
	}
	agentConfig := agent.DefaultConfig()
	agentConfig.ServerURL = config.ServerURL
	agentConfig.MachineID = config.MachineID
	agentConfig.MachineToken = config.MachineToken
	if err := agent.New(agentConfig).Tick(context.Background()); err != nil {
		return fmt.Errorf("connect once: %w", err)
	}
	_, err = fmt.Fprintln(stdout, "Connected")
	return err
}
