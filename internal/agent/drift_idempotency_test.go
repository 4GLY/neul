package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
)

func TestAgentTick_changesDriftIdempotencyKeyWhenDesiredStateChanges(t *testing.T) {
	var mu sync.Mutex
	var driftKeys []string
	desiredVersion := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			mu.Lock()
			version := desiredVersion
			mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"resources":[{"id":"res_package","kind":"package","name":"kubectl","desiredVersion":%d,"spec":{"sourceKind":"brew","name":"kubectl","desiredVersion":"1.31.%d"}}]}`, version, version)
		case "/api/agent/drift-report":
			mu.Lock()
			driftKeys = append(driftKeys, r.Header.Get("Idempotency-Key"))
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewWithAdapters(
		Config{ServerURL: server.URL, MachineID: "machine_1", MachineToken: "mtn_secret"},
		&fakePackageAdapter{installed: map[string]string{"kubectl": "1.31.1"}},
	)

	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	mu.Lock()
	desiredVersion = 2
	mu.Unlock()
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}

	mu.Lock()
	keys := append([]string(nil), driftKeys...)
	mu.Unlock()
	if len(keys) != 2 {
		t.Fatalf("driftKeys = %v, want two drift reports", keys)
	}
	if keys[0] == "" || keys[1] == "" {
		t.Fatalf("driftKeys = %v, want non-empty idempotency keys", keys)
	}
	keyPattern := regexp.MustCompile(`^drift-machine_1-[0-9a-f]{32}$`)
	for _, key := range keys {
		if !keyPattern.MatchString(key) {
			t.Fatalf("drift key = %q, want drift-machine_1-<32 hex>", key)
		}
	}
	if keys[0] == keys[1] {
		t.Fatalf("driftKeys = %v, want different keys after desired state version changes", keys)
	}
}

func TestAgentTick_keepsDriftIdempotencyKeyStableForSameDesiredState(t *testing.T) {
	var mu sync.Mutex
	var driftKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[{"id":"res_package","kind":"package","name":"kubectl","desiredVersion":1,"spec":{"sourceKind":"brew","name":"kubectl","desiredVersion":"1.31.1"}}]}`))
		case "/api/agent/drift-report":
			mu.Lock()
			driftKeys = append(driftKeys, r.Header.Get("Idempotency-Key"))
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewWithAdapters(
		Config{ServerURL: server.URL, MachineID: "machine_1", MachineToken: "mtn_secret"},
		&fakePackageAdapter{installed: map[string]string{"kubectl": "1.31.1"}},
	)

	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}

	mu.Lock()
	keys := append([]string(nil), driftKeys...)
	mu.Unlock()
	if len(keys) != 2 {
		t.Fatalf("driftKeys = %v, want two drift reports", keys)
	}
	if keys[0] != keys[1] {
		t.Fatalf("driftKeys = %v, want stable key for identical desired state and report payload", keys)
	}
}

func TestDriftIdempotencyKey_stableForSameDesiredState(t *testing.T) {
	resources := []DesiredResource{
		{ID: "resource_b", Kind: "package", Name: "ripgrep", DesiredVersion: 2, Spec: map[string]interface{}{"desiredVersion": "14.1.0", "name": "ripgrep", "sourceKind": "brew"}},
		{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "1.31.1", "name": "kubectl", "sourceKind": "brew"}},
	}
	events := []ResourceEvent{
		{ResourceID: "resource_b", Status: "drifted", Message: "brew check", DesiredVersion: 2},
		{ResourceID: "resource_a", Status: "in_sync", Message: "brew check", DesiredVersion: 1, AppliedVersion: 1},
	}

	first := driftIdempotencyKey("machine_1", resources, events)
	second := driftIdempotencyKey(
		"machine_1",
		[]DesiredResource{
			{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"sourceKind": "brew", "name": "kubectl", "desiredVersion": "1.31.1"}},
			{ID: "resource_b", Kind: "package", Name: "ripgrep", DesiredVersion: 2, Spec: map[string]interface{}{"sourceKind": "brew", "name": "ripgrep", "desiredVersion": "14.1.0"}},
		},
		[]ResourceEvent{
			{ResourceID: "resource_a", Status: "in_sync", Message: "brew check", DesiredVersion: 1, AppliedVersion: 1},
			{ResourceID: "resource_b", Status: "drifted", Message: "brew check", DesiredVersion: 2},
		},
	)

	if first != second {
		t.Fatalf("driftIdempotencyKey() = %q then %q, want stable key for same desired state", first, second)
	}
}

func TestDriftIdempotencyKey_changesWhenDesiredStateFingerprintChanges(t *testing.T) {
	baseResources := []DesiredResource{
		{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "1.31.1", "name": "kubectl", "sourceKind": "brew"}},
	}
	baseEvents := []ResourceEvent{
		{ResourceID: "resource_a", Status: "in_sync", Message: "brew check", DesiredVersion: 1, AppliedVersion: 1},
	}
	base := driftIdempotencyKey("machine_1", baseResources, baseEvents)
	tests := []struct {
		name      string
		machineID string
		resources []DesiredResource
		events    []ResourceEvent
	}{
		{
			name:      "machine",
			machineID: "machine_2",
			resources: []DesiredResource{
				{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "1.31.1", "name": "kubectl", "sourceKind": "brew"}},
			},
			events: []ResourceEvent{
				{ResourceID: "resource_a", Status: "in_sync", Message: "brew check", DesiredVersion: 1, AppliedVersion: 1},
			},
		},
		{
			name:      "desired version",
			machineID: "machine_1",
			resources: []DesiredResource{
				{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 2, Spec: map[string]interface{}{"desiredVersion": "1.31.2", "name": "kubectl", "sourceKind": "brew"}},
			},
			events: []ResourceEvent{
				{ResourceID: "resource_a", Status: "drifted", Message: "brew check", DesiredVersion: 2},
			},
		},
		{
			name:      "spec",
			machineID: "machine_1",
			resources: []DesiredResource{
				{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "1.31.2", "name": "kubectl", "sourceKind": "brew"}},
			},
			events: []ResourceEvent{
				{ResourceID: "resource_a", Status: "in_sync", Message: "brew check", DesiredVersion: 1, AppliedVersion: 1},
			},
		},
		{
			name:      "resource membership",
			machineID: "machine_1",
			resources: []DesiredResource{
				{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "1.31.1", "name": "kubectl", "sourceKind": "brew"}},
				{ID: "resource_b", Kind: "package", Name: "ripgrep", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "14.1.0", "name": "ripgrep", "sourceKind": "brew"}},
			},
			events: []ResourceEvent{
				{ResourceID: "resource_a", Status: "in_sync", Message: "brew check", DesiredVersion: 1, AppliedVersion: 1},
				{ResourceID: "resource_b", Status: "drifted", Message: "brew check", DesiredVersion: 1},
			},
		},
		{
			name:      "observed event",
			machineID: "machine_1",
			resources: []DesiredResource{
				{ID: "resource_a", Kind: "package", Name: "kubectl", DesiredVersion: 1, Spec: map[string]interface{}{"desiredVersion": "1.31.1", "name": "kubectl", "sourceKind": "brew"}},
			},
			events: []ResourceEvent{
				{ResourceID: "resource_a", Status: "drifted", Message: "brew check", DesiredVersion: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := driftIdempotencyKey(tt.machineID, tt.resources, tt.events); got == base {
				t.Fatalf("driftIdempotencyKey() = %q, want different key from base %q", got, base)
			}
		})
	}
}
