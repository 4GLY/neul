package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const defaultAgentLogLines = 200

func runAgentLogsCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent logs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", defaultConfigPath(), "config path")
	logPath := flags.String("log", "", "log path")
	allLines := flags.Bool("all", false, "print all log lines")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := readConfig(*configPath); err != nil {
		return err
	}
	selectedLogPath := *logPath
	if selectedLogPath == "" {
		selectedLogPath = filepath.Join(filepath.Dir(*configPath), "agent.log")
	}
	body, err := os.ReadFile(selectedLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("agent log file was not found")
		}
		return fmt.Errorf("read logs: %w", err)
	}
	if *allLines {
		_, err = stdout.Write(body)
		return err
	}
	_, err = io.WriteString(stdout, lastLogLines(string(body), defaultAgentLogLines))
	return err
}

func lastLogLines(body string, limit int) string {
	trimmed := strings.TrimSuffix(body, "\n")
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= limit {
		return body
	}
	return strings.Join(lines[len(lines)-limit:], "\n") + "\n"
}
