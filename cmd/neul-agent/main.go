package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/4gly/neul/internal/agent"
)

func main() {
	once := flag.Bool("once", false, "run one agent tick")
	configPath := flag.String("config", "", "agent config path")
	statusPath := flag.String("status", "", "agent status receipt path")
	flag.Parse()
	if *configPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "--config is required")
		os.Exit(1)
	}
	config, err := agent.LoadConfig(*configPath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *statusPath == "" {
		*statusPath = filepath.Join(filepath.Dir(*configPath), "status.json")
	}
	config.EnablePackageAdapters = true
	client := agent.New(config)
	ctx := context.Background()
	if *once {
		if err := client.TickWithStatus(ctx, *statusPath); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_, _ = fmt.Fprintln(os.Stdout, "agent tick completed")
		return
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := client.Run(ctx, agent.RunOptions{
		Logger:     logger,
		StatusPath: *statusPath,
		ConfigReloader: func() (agent.Config, error) {
			config, err := agent.LoadConfig(*configPath)
			if err != nil {
				return agent.Config{}, err
			}
			config.EnablePackageAdapters = true
			return config, nil
		},
	}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
