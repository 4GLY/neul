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
	MachineID            string       `json:"machineId"`
	ServerURL            string       `json:"serverURL"`
	LastHeartbeatAttempt string       `json:"lastHeartbeatAttempt"`
	LastSuccess          string       `json:"lastSuccess"`
	LastHeartbeatAt      string       `json:"lastHeartbeatAt"`
	LastError            *StatusError `json:"lastError"`
}

func (c *Client) TickWithStatus(ctx context.Context, statusPath string) error {
	attemptedAt := time.Now().UTC()
	err := c.Tick(ctx)
	if err != nil {
		if writeErr := c.writeStatus(statusPath, attemptedAt, time.Time{}, err); writeErr != nil {
			return writeErr
		}
		return err
	}
	if writeErr := c.writeStatus(statusPath, attemptedAt, attemptedAt, nil); writeErr != nil {
		return writeErr
	}
	return nil
}

func (c *Client) writeStatus(statusPath string, attemptedAt time.Time, lastSuccess time.Time, tickErr error) error {
	if statusPath == "" {
		return nil
	}
	receipt := StatusReceipt{
		MachineID:            c.config.MachineID,
		ServerURL:            c.config.ServerURL,
		LastHeartbeatAttempt: attemptedAt.Format(time.RFC3339Nano),
	}
	if !lastSuccess.IsZero() {
		formatted := lastSuccess.Format(time.RFC3339Nano)
		receipt.LastSuccess = formatted
		receipt.LastHeartbeatAt = formatted
	}
	if tickErr != nil {
		receipt.LastError = &StatusError{Kind: classifyTickError(tickErr), Message: tickErr.Error()}
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o700); err != nil {
		return fmt.Errorf("create status dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(statusPath), ".status-*.json")
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
	if err := os.Rename(tmpPath, statusPath); err != nil {
		return fmt.Errorf("replace status file: %w", err)
	}
	return nil
}
