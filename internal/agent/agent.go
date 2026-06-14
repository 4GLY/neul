package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerURL             string        `json:"serverURL"`
	MachineID             string        `json:"machineId"`
	MachineToken          string        `json:"machineToken"`
	HomeDir               string        `json:"homeDir,omitempty"`
	HeartbeatInterval     time.Duration `json:"-"`
	EnablePackageAdapters bool          `json:"-"`
}

type Client struct {
	config Config
	http   *http.Client
	brew   PackageAdapter
}

type HTTPStatusError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s %s failed with status %d", e.Method, e.Path, e.StatusCode)
}

func DefaultConfig() Config {
	return Config{HeartbeatInterval: 30 * time.Second}
}

func LoadConfig(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	config := DefaultConfig()
	if err := json.Unmarshal(body, &config); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	config = normalizeConfig(config)
	if config.ServerURL == "" || config.MachineID == "" || config.MachineToken == "" {
		return Config{}, fmt.Errorf("config requires serverURL, machineId, and machineToken")
	}
	return config, nil
}

func New(config Config) *Client {
	brew := PackageAdapter(disabledBrewAdapter{})
	if config.EnablePackageAdapters {
		brew = NewHomebrewAdapter()
	}
	return NewWithAdapters(config, brew)
}

func NewWithAdapters(config Config, brew PackageAdapter) *Client {
	config = normalizeConfig(config)
	return &Client{config: config, http: &http.Client{Timeout: 10 * time.Second}, brew: brew}
}

func normalizeConfig(config Config) Config {
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.HomeDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			config.HomeDir = homeDir
		}
	}
	config.ServerURL = strings.TrimRight(config.ServerURL, "/")
	return config
}

func (c *Client) Endpoints() []string {
	return []string{
		c.config.ServerURL + "/api/agent/heartbeat",
		c.config.ServerURL + "/api/agent/desired-state",
		c.config.ServerURL + "/api/agent/commands",
		c.config.ServerURL + "/api/agent/reconcile-report",
		c.config.ServerURL + "/api/agent/drift-report",
	}
}

func (c *Client) Tick(ctx context.Context) error {
	if err := c.postJSON(ctx, "/api/agent/heartbeat", map[string]string{
		"machineId":    c.config.MachineID,
		"agentVersion": "0.1.0",
	}); err != nil {
		return err
	}
	var desiredState desiredStateResponse
	if err := c.getJSON(ctx, "/api/agent/desired-state", &desiredState); err != nil {
		return err
	}
	if len(desiredState.Resources) > 0 {
		events := make([]ResourceEvent, 0, len(desiredState.Resources))
		for _, resource := range desiredState.Resources {
			events = append(events, EvaluateResourceWithHome(ctx, c.brew, c.config.HomeDir, resource))
		}
		report := driftReport{MachineID: c.config.MachineID, Events: events}
		if err := c.postJSONWithIdempotency(ctx, "/api/agent/drift-report", report, driftIdempotencyKey(c.config.MachineID, desiredState.Resources, events)); err != nil {
			return err
		}
	}
	var commandResponse commandPollResponse
	if err := c.getJSON(ctx, "/api/agent/commands", &commandResponse); err != nil {
		return err
	}
	for _, command := range commandResponse.Commands {
		report := c.commandReportFor(ctx, command, desiredState.Resources)
		if err := c.postJSONWithIdempotency(ctx, "/api/agent/reconcile-report", report, "command-"+command.ID); err != nil {
			return err
		}
	}
	return nil
}

type desiredStateResponse struct {
	Resources []DesiredResource `json:"resources"`
}

type commandPollResponse struct {
	Commands []agentCommand `json:"commands"`
}

type agentCommand struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

type commandReport struct {
	MachineID string          `json:"machineId"`
	CommandID string          `json:"commandId"`
	Status    string          `json:"status"`
	Events    []ResourceEvent `json:"events"`
}

type driftReport struct {
	MachineID string          `json:"machineId"`
	Events    []ResourceEvent `json:"events"`
}

func driftIdempotencyKey(machineID string, resources []DesiredResource, events []ResourceEvent) string {
	fingerprints := make([]string, 0, len(resources))
	for _, resource := range resources {
		specJSON, err := json.Marshal(resource.Spec)
		if err != nil {
			specJSON = []byte("null")
		}
		fingerprints = append(fingerprints, strings.Join([]string{
			"resource",
			resource.ID,
			resource.Kind,
			resource.Name,
			strconv.Itoa(resource.DesiredVersion),
			string(specJSON),
		}, ":"))
	}
	for _, event := range events {
		fingerprints = append(fingerprints, strings.Join([]string{
			"event",
			event.ResourceID,
			event.Status,
			strconv.Itoa(event.DesiredVersion),
			strconv.Itoa(event.AppliedVersion),
		}, ":"))
	}
	sort.Strings(fingerprints)
	var fingerprint strings.Builder
	for _, value := range fingerprints {
		fingerprint.WriteString(strconv.Itoa(len(value)))
		fingerprint.WriteByte(':')
		fingerprint.WriteString(value)
		fingerprint.WriteByte(';')
	}
	sum := sha256.Sum256([]byte(fingerprint.String()))
	return fmt.Sprintf("drift-%s-%s", machineID, hex.EncodeToString(sum[:])[:32])
}

func (c *Client) getJSON(ctx context.Context, path string, dst interface{}) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.ServerURL+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &HTTPStatusError{Method: http.MethodGet, Path: path, StatusCode: response.StatusCode}
	}
	if dst == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, path string, body interface{}) error {
	return c.postJSONWithIdempotency(ctx, path, body, "")
}

func (c *Client) postJSONWithIdempotency(ctx context.Context, path string, body interface{}, idempotencyKey string) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.ServerURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &HTTPStatusError{Method: http.MethodPost, Path: path, StatusCode: response.StatusCode}
	}
	return nil
}

func (c *Client) authorize(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+c.config.MachineToken)
}
