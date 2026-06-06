package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/4gly/neul/internal/agent"
)

func main() {
	once := flag.Bool("once", false, "run one agent tick")
	configPath := flag.String("config", "", "agent config path")
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
	client := agent.New(config)
	ctx := context.Background()
	if *once {
		if err := client.Tick(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_, _ = fmt.Fprintln(os.Stdout, "agent tick completed")
		return
	}
	ticker := time.NewTicker(config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		if err := client.Tick(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		<-ticker.C
	}
}
