package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type StatusError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type StatusReceipt struct {
	Mode                 string       `json:"mode"`
	MachineID            string       `json:"machineId"`
	ServerURL            string       `json:"serverURL"`
	LastHeartbeatAttempt string       `json:"lastHeartbeatAttempt"`
	LastSuccess          string       `json:"lastSuccess"`
	LastHeartbeatAt      string       `json:"lastHeartbeatAt"`
	LastError            *StatusError `json:"lastError"`
}

const (
	statusModeRunLoop     = "run_loop"
	statusModeConnectOnce = "connect_once"
)

type statusWriteOptions struct {
	Path                 string
	Mode                 string
	MachineID            string
	ServerURL            string
	LastHeartbeatAttempt time.Time
	LastSuccess          time.Time
	LastHeartbeatAt      time.Time
	LastError            *StatusError
}

func (c *Client) TickWithStatus(ctx context.Context, statusPath string) error {
	attemptedAt := time.Now().UTC()
	err := c.Tick(ctx)
	if err != nil {
		if writeErr := c.writeStatus(statusWriteOptions{
			Path:                 statusPath,
			Mode:                 statusModeConnectOnce,
			LastHeartbeatAttempt: attemptedAt,
			LastError:            &StatusError{Kind: classifyTickError(err), Message: err.Error()},
		}); writeErr != nil {
			return writeErr
		}
		return err
	}
	if writeErr := c.writeStatus(statusWriteOptions{
		Path:                 statusPath,
		Mode:                 statusModeConnectOnce,
		LastHeartbeatAttempt: attemptedAt,
		LastSuccess:          attemptedAt,
		LastHeartbeatAt:      attemptedAt,
	}); writeErr != nil {
		return writeErr
	}
	return nil
}

func (c *Client) writeStatus(options statusWriteOptions) error {
	options.MachineID = c.config.MachineID
	options.ServerURL = c.config.ServerURL
	return writeStatus(options)
}

func writeStatus(options statusWriteOptions) error {
	if options.Path == "" {
		return nil
	}
	receipt := StatusReceipt{
		Mode:                 options.Mode,
		MachineID:            options.MachineID,
		ServerURL:            options.ServerURL,
		LastHeartbeatAttempt: options.LastHeartbeatAttempt.Format(time.RFC3339Nano),
		LastError:            options.LastError,
	}
	if !options.LastSuccess.IsZero() {
		receipt.LastSuccess = options.LastSuccess.Format(time.RFC3339Nano)
	}
	if !options.LastHeartbeatAt.IsZero() {
		receipt.LastHeartbeatAt = options.LastHeartbeatAt.Format(time.RFC3339Nano)
		if receipt.LastSuccess == "" {
			receipt.LastSuccess = receipt.LastHeartbeatAt
		}
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(options.Path), 0o700); err != nil {
		return fmt.Errorf("create status dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(options.Path), ".status-*.json")
	if err != nil {
		return fmt.Errorf("create status temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write status temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close status temp file: %w", err)
	}
	if err := os.Rename(tmpPath, options.Path); err != nil {
		return fmt.Errorf("replace status file: %w", err)
	}
	return nil
}
