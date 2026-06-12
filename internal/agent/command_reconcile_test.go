package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRepairDrift_appliesOnlyRequestedResource_whenPayloadListsResourceIDs(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	reports, _ := runCommandReconcileTick(t, adapter, commandReconcileServerConfig{
		resources: []DesiredResource{brewCommandResource("resource_a"), brewCommandResource("resource_b")},
		commands:  []agentCommand{repairDriftCommand("command_repair_one", []string{"resource_a", "resource_a"})},
	})

	// Then
	requireAppliedNames(t, adapter, []string{"resource_a"})
	report := requireCommandReport(t, reports, "command_repair_one", "finished", 1)
	if report.Events[0].ResourceID != "resource_a" || report.Events[0].Status != "in_sync" {
		t.Fatalf("event = %+v, want resource_a in_sync", report.Events[0])
	}
}

func TestRepairDrift_postsFinishedNoop_whenPayloadResourceIDsEmpty(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	reports, _ := runCommandReconcileTick(t, adapter, commandReconcileServerConfig{
		commands: []agentCommand{repairDriftCommand("command_repair_empty", []string{})},
	})

	// Then
	if len(adapter.appliedNames) != 0 {
		t.Fatalf("appliedNames = %v, want none", adapter.appliedNames)
	}
	requireCommandReport(t, reports, "command_repair_empty", "finished", 0)
}

func TestRepairDrift_reportsMissingResource_whenPayloadContainsUnknownID(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	reports, _ := runCommandReconcileTick(t, adapter, commandReconcileServerConfig{
		resources: []DesiredResource{brewCommandResource("resource_a")},
		commands:  []agentCommand{repairDriftCommand("command_repair_missing", []string{"resource_a", "resource_missing"})},
	})

	// Then
	requireAppliedNames(t, adapter, []string{"resource_a"})
	report := requireCommandReport(t, reports, "command_repair_missing", "finished", 2)
	missing := report.Events[1]
	if missing.ResourceID != "" || missing.Status != "blocked" || !strings.HasPrefix(missing.Message, "resource_not_found:resource_missing") {
		t.Fatalf("missing event = %+v, want command-scoped blocked resource_not_found", missing)
	}
}

func TestAgentTick_ReconcileNow_appliesAllBrewResourcesAndIgnoresPayloadResourceIDs(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	reports, _ := runCommandReconcileTick(t, adapter, commandReconcileServerConfig{
		resources: []DesiredResource{brewCommandResource("resource_a"), brewCommandResource("resource_b")},
		commands:  []agentCommand{reconcileNowCommand("command_reconcile_all", []string{"resource_a"})},
	})

	// Then
	requireAppliedNames(t, adapter, []string{"resource_a", "resource_b"})
	requireCommandReport(t, reports, "command_reconcile_all", "finished", 2)
}

func TestAgentTick_ReconcileNow_emitsPackageEventOnly_whenDesiredStateIncludesDotfile(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	reports, _ := runCommandReconcileTick(t, adapter, commandReconcileServerConfig{
		resources: []DesiredResource{brewCommandResource("resource_brew"), blockedDotfileCommandResource("resource_dotfile")},
		commands:  []agentCommand{reconcileNowCommand("command_reconcile_package_only", nil)},
	})

	// Then
	requireAppliedNames(t, adapter, []string{"resource_brew"})
	report := requireCommandReport(t, reports, "command_reconcile_package_only", "finished", 1)
	if report.Events[0].ResourceID != "resource_brew" {
		t.Fatalf("events = %+v, want only brew package event", report.Events)
	}
	for _, event := range report.Events {
		if event.ResourceID == "resource_dotfile" || strings.Contains(event.Message, "path_not_allowed") {
			t.Fatalf("events = %+v, want no dotfile blocked command event", report.Events)
		}
	}
}

func TestAgentTick_ReconcileCommandAfterAck_doesNotRerunOnSecondTick(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	server, reports, keys := newCommandReconcileTestServer(t, commandReconcileServerConfig{
		resources:         []DesiredResource{brewCommandResource("resource_a")},
		commands:          []agentCommand{{ID: "command_once", Type: "reconcile_now", Payload: map[string]interface{}{}}},
		removeAfterReport: true,
	})
	defer server.Close()
	client := NewWithAdapters(commandReconcileConfig(server.URL), adapter)

	// When
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}

	// Then
	requireAppliedNames(t, adapter, []string{"resource_a"})
	if len(*reports) != 1 {
		t.Fatalf("reports = %+v, want one command report after ack", *reports)
	}
	if got := (*keys)[0]; got != "command-command_once" {
		t.Fatalf("idempotency key = %q, want command-command_once", got)
	}
}

func TestAgentTick_ReconcileReportFailure_doesNotMemoizeCommandAndRetriesWithSameIdempotencyKey(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	server, reports, keys := newCommandReconcileTestServer(t, commandReconcileServerConfig{
		resources:              []DesiredResource{brewCommandResource("resource_a")},
		commands:               []agentCommand{{ID: "command_retry", Type: "reconcile_now", Payload: map[string]interface{}{}}},
		failFirstCommandReport: true,
		removeAfterReport:      true,
	})
	defer server.Close()
	client := NewWithAdapters(commandReconcileConfig(server.URL), adapter)

	// When
	if err := client.Tick(context.Background()); err == nil {
		t.Fatalf("first Tick() error = nil, want reconcile-report failure")
	}
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}

	// Then
	requireAppliedNames(t, adapter, []string{"resource_a", "resource_a"})
	requireCommandReport(t, *reports, "command_retry", "finished", 1)
	if !slices.Equal(*keys, []string{"command-command_retry", "command-command_retry"}) {
		t.Fatalf("idempotency keys = %v, want stable command-command_retry retry key", *keys)
	}
}

func TestCommandPolling_unknownCommandReportsUnsupportedAndDoesNotApplyPayloadResources(t *testing.T) {
	// Given
	adapter := &recordingCommandAdapter{}
	markerPath := filepath.Join(t.TempDir(), "payload_executed")
	reports, _ := runCommandReconcileTick(t, adapter, commandReconcileServerConfig{
		resources: []DesiredResource{brewCommandResource("resource_a")},
		commands: []agentCommand{{
			ID:      "command_unknown",
			Type:    "shell",
			Payload: map[string]interface{}{"resourceIds": []string{"resource_a"}, "touch": markerPath},
		}},
	})

	// Then
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("marker stat err = %v, want payload not executed", err)
	}
	if len(adapter.appliedNames) != 0 {
		t.Fatalf("appliedNames = %v, want no adapter apply for unknown command", adapter.appliedNames)
	}
	requireCommandReport(t, reports, "command_unknown", "unsupported_command", 0)
}

type commandReconcileServerConfig struct {
	resources              []DesiredResource
	commands               []agentCommand
	removeAfterReport      bool
	failFirstCommandReport bool
}

type observedCommandReport struct {
	CommandID, Status string
	Events            []ResourceEvent
}

func runCommandReconcileTick(t *testing.T, adapter *recordingCommandAdapter, config commandReconcileServerConfig) ([]observedCommandReport, []string) {
	t.Helper()
	server, reports, keys := newCommandReconcileTestServer(t, config)
	defer server.Close()
	if err := NewWithAdapters(commandReconcileConfig(server.URL), adapter).Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	return *reports, *keys
}

func newCommandReconcileTestServer(t *testing.T, config commandReconcileServerConfig) (*httptest.Server, *[]observedCommandReport, *[]string) {
	var reports []observedCommandReport
	var commandKeys []string
	commandReportAttempts := 0
	acked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case "/api/agent/desired-state":
			if err := json.NewEncoder(w).Encode(desiredStateResponse{Resources: config.resources}); err != nil {
				t.Fatalf("encode desired state error = %v", err)
			}
		case "/api/agent/drift-report":
			w.WriteHeader(http.StatusAccepted)
		case "/api/agent/commands":
			commands := config.commands
			if config.removeAfterReport && acked {
				commands = nil
			}
			if err := json.NewEncoder(w).Encode(commandPollResponse{Commands: commands}); err != nil {
				t.Fatalf("encode commands error = %v", err)
			}
		case "/api/agent/reconcile-report":
			commandReportAttempts++
			commandKeys = append(commandKeys, r.Header.Get("Idempotency-Key"))
			if config.failFirstCommandReport && commandReportAttempts == 1 {
				http.Error(w, "temporary report failure", http.StatusInternalServerError)
				return
			}
			var report observedCommandReport
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatalf("decode command report error = %v", err)
			}
			reports = append(reports, report)
			if report.Status == "finished" {
				acked = true
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	return server, &reports, &commandKeys
}

type recordingCommandAdapter struct {
	appliedNames []string
}

func (a *recordingCommandAdapter) Check(_ context.Context, name string, _ string) (string, error) {
	return "drifted", nil
}

func (a *recordingCommandAdapter) Apply(_ context.Context, name string, _ string) (string, error) {
	a.appliedNames = append(a.appliedNames, name)
	return "in_sync", nil
}

func commandReconcileConfig(serverURL string) Config {
	return Config{ServerURL: serverURL, MachineID: "machine_command", MachineToken: "mtn_command"}
}

func brewCommandResource(id string) DesiredResource {
	return DesiredResource{ID: id, Kind: "package", Name: id, DesiredVersion: 1, Spec: map[string]interface{}{"sourceKind": "brew", "name": id, "desiredVersion": "latest"}}
}

func blockedDotfileCommandResource(id string) DesiredResource {
	return DesiredResource{ID: id, Kind: "dotfile", Name: "/etc/hosts", DesiredVersion: 1, Spec: map[string]interface{}{"path": "/etc/hosts", "content": "127.0.0.1 localhost\n"}}
}

func repairDriftCommand(id string, resourceIDs []string) agentCommand {
	return agentCommand{ID: id, Type: "repair_drift", Payload: map[string]interface{}{"resourceIds": resourceIDs}}
}

func reconcileNowCommand(id string, resourceIDs []string) agentCommand {
	payload := map[string]interface{}{}
	if resourceIDs != nil {
		payload["resourceIds"] = resourceIDs
	}
	return agentCommand{ID: id, Type: "reconcile_now", Payload: payload}
}

func requireCommandReport(t *testing.T, reports []observedCommandReport, commandID string, status string, eventCount int) observedCommandReport {
	t.Helper()
	if len(reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one command report", reports)
	}
	report := reports[0]
	if report.CommandID != commandID || report.Status != status || len(report.Events) != eventCount {
		t.Fatalf("report = %+v, want commandId=%s status=%s events=%d", report, commandID, status, eventCount)
	}
	return report
}

func requireAppliedNames(t *testing.T, adapter *recordingCommandAdapter, want []string) {
	t.Helper()
	if !slices.Equal(adapter.appliedNames, want) {
		t.Fatalf("appliedNames = %v, want %v", adapter.appliedNames, want)
	}
}
