package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPackageAdapterDisabled_agentNewDefaultBlocksBrewWithoutExecutingPathBrew(t *testing.T) {
	// Given
	tempRoot := t.TempDir()
	fakeBin := filepath.Join(tempRoot, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	brewLog := filepath.Join(tempRoot, "brew.log")
	fakeBrew := filepath.Join(fakeBin, "brew")
	fakeBrewBody := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$BREW_LOG\"\nexit 42\n"
	if err := os.WriteFile(fakeBrew, []byte(fakeBrewBody), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", fakeBin)
	t.Setenv("BREW_LOG", brewLog)

	var report driftReport
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			_, _ = w.Write([]byte(`{"resources":[{"id":"resource_brew","kind":"package","name":"kubectl","desiredVersion":1,"spec":{"sourceKind":"brew","name":"kubectl","desiredVersion":"latest"}}]}`))
		case "/api/agent/drift-report":
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
		case "/api/agent/commands":
			_, _ = w.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	// When
	client := New(Config{ServerURL: server.URL, MachineID: "machine_safe", MachineToken: "mtn_safe"})
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	// Then
	if _, err := os.Stat(brewLog); !os.IsNotExist(err) {
		t.Fatalf("brew log stat err = %v, want fake brew not executed", err)
	}
	if len(report.Events) != 1 {
		t.Fatalf("events = %+v, want one brew event", report.Events)
	}
	event := report.Events[0]
	if event.Status != "blocked" {
		t.Fatalf("status = %s, want blocked", event.Status)
	}
	if !strings.HasPrefix(event.Message, "brew_disabled") {
		t.Fatalf("message = %q, want brew_disabled prefix", event.Message)
	}
}

func TestAgentNew_requiresEnablePackageAdaptersForProductionDiscovery(t *testing.T) {
	// Given
	configType := reflect.TypeOf(Config{})

	// When
	field, ok := configType.FieldByName("EnablePackageAdapters")

	// Then
	if !ok {
		t.Fatalf("Config missing EnablePackageAdapters bool; production package adapter discovery must be explicit opt-in")
	}
	if field.Type.Kind() != reflect.Bool {
		t.Fatalf("EnablePackageAdapters type = %s, want bool", field.Type)
	}
	zeroConfig := reflect.Zero(configType)
	if zeroConfig.FieldByIndex(field.Index).Bool() {
		t.Fatalf("EnablePackageAdapters zero value = true, want default false")
	}
}

func TestAgentReport_unsupportedPackageSourceKindsReturnBlockedMessages(t *testing.T) {
	tests := []struct {
		name       string
		sourceKind string
	}{
		{name: "mise package", sourceKind: "mise"},
		{name: "apt package", sourceKind: "apt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			resource := DesiredResource{
				ID:             "resource_" + tt.sourceKind,
				Kind:           "package",
				DesiredVersion: 1,
				Spec: map[string]interface{}{
					"sourceKind":     tt.sourceKind,
					"name":           "node",
					"desiredVersion": "latest",
				},
			}

			// When
			event := EvaluateResource(context.Background(), nil, resource)

			// Then
			if event.Status != "blocked" {
				t.Fatalf("status = %s, want blocked", event.Status)
			}
			message := strings.ToLower(event.Message)
			if !strings.Contains(message, tt.sourceKind) || !strings.Contains(message, "unsupported") {
				t.Fatalf("message = %q, want readable unsupported %s source message", event.Message, tt.sourceKind)
			}
			if strings.Contains(event.Status, "unsupported_adapter") || strings.Contains(event.Message, "unsupported_adapter") {
				t.Fatalf("event = %+v, want no unsupported_adapter status or message", event)
			}
		})
	}
}

func TestAgentReport_unsupportedMissingBrewAdapterReportsBrewUnavailable(t *testing.T) {
	resource := DesiredResource{
		ID:             "resource_brew",
		Kind:           "package",
		DesiredVersion: 1,
		Spec: map[string]interface{}{
			"sourceKind":     "brew",
			"name":           "kubectl",
			"desiredVersion": "latest",
		},
	}
	tests := []struct {
		name string
		run  func(context.Context, DesiredResource) ResourceEvent
	}{
		{name: "check", run: func(ctx context.Context, resource DesiredResource) ResourceEvent {
			return CheckPackage(ctx, nil, resource)
		}},
		{name: "apply", run: func(ctx context.Context, resource DesiredResource) ResourceEvent {
			return ApplyPackage(ctx, nil, resource)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			event := tt.run(context.Background(), resource)

			// Then
			if event.Status != "blocked" {
				t.Fatalf("status = %s, want blocked", event.Status)
			}
			if !strings.HasPrefix(event.Message, "brew_unavailable") {
				t.Fatalf("message = %q, want brew_unavailable prefix", event.Message)
			}
		})
	}
}

func TestDotfile_pathNotAllowedRemainsBlockedDuringPackageSafetyChanges(t *testing.T) {
	// Given
	resource := DesiredResource{
		ID:             "resource_dotfile",
		Kind:           "dotfile",
		DesiredVersion: 1,
		Spec:           map[string]interface{}{"path": "/etc/hosts"},
	}

	// When
	event := EvaluateResource(context.Background(), nil, resource)

	// Then
	if event.Status != "blocked" {
		t.Fatalf("status = %s, want blocked", event.Status)
	}
	if event.Message != "path_not_allowed" {
		t.Fatalf("message = %q, want path_not_allowed", event.Message)
	}
}
