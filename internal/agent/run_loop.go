package agent

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"
)

type DelayFunc func(context.Context, time.Duration) error

type ConfigReloader func() (Config, error)

type RunOptions struct {
	Delay          DelayFunc
	ConfigReloader ConfigReloader
	Logger         *slog.Logger
	MaxBackoff     time.Duration
}

func (c *Client) Run(ctx context.Context, options RunOptions) error {
	delay := options.Delay
	if delay == nil {
		delay = sleepDelay
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	maxBackoff := options.MaxBackoff
	if maxBackoff == 0 {
		maxBackoff = 5 * time.Minute
	}
	backoff := c.config.HeartbeatInterval
	lastFailureKind := ""
	consecutiveFailures := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if options.ConfigReloader != nil {
			if config, err := options.ConfigReloader(); err != nil {
				logger.Warn("agent config reload failed", "error", err)
			} else {
				c.applyConfig(config)
			}
		}
		if err := c.Tick(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			kind := classifyTickError(err)
			consecutiveFailures++
			if kind != lastFailureKind {
				logger.Warn("agent tick failed", "kind", kind, "error", err, "retry_in", backoff)
				lastFailureKind = kind
			}
			nextDelay := withJitter(backoff)
			backoff = growBackoff(backoff, maxBackoff)
			if err := delay(ctx, nextDelay); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			continue
		}
		if consecutiveFailures > 0 {
			logger.Info("agent connection restored", "failures", consecutiveFailures)
		}
		consecutiveFailures = 0
		lastFailureKind = ""
		backoff = c.config.HeartbeatInterval
		if err := delay(ctx, c.config.HeartbeatInterval); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

func (c *Client) applyConfig(config Config) {
	c.config = normalizeConfig(config)
}

func sleepDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func growBackoff(current time.Duration, maxBackoff time.Duration) time.Duration {
	if current >= maxBackoff {
		return maxBackoff
	}
	if current >= maxBackoff/2 {
		return maxBackoff
	}
	next := current * 2
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}

func withJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}
	window := delay / 10
	if window <= 0 {
		return delay
	}
	offsetRange := int64(window * 2)
	offset := time.Duration(time.Now().UnixNano()%offsetRange) - window
	return delay + offset
}

func classifyTickError(err error) string {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		if statusErr.StatusCode == 401 || statusErr.StatusCode == 403 {
			return "auth_failure"
		}
		if statusErr.StatusCode == 429 {
			return "rate_limited"
		}
		return "server_failure"
	}
	return "connection_failure"
}
