package agent

import (
	"context"
	"strings"
	"testing"
)

func TestHomebrew_fakeAdapterReportsInSyncDriftedAndApplySuccess(t *testing.T) {
	fake := &fakePackageAdapter{installed: map[string]string{"kubectl": "1.31.0"}}

	inSync := CheckPackage(context.Background(), fake, DesiredResource{
		ID: "resource_in_sync", Kind: "package", Spec: map[string]interface{}{"sourceKind": "brew", "name": "kubectl", "desiredVersion": "1.31.0"},
	})
	if inSync.Status != "in_sync" {
		t.Fatalf("inSync status = %s, want in_sync", inSync.Status)
	}

	drifted := CheckPackage(context.Background(), fake, DesiredResource{
		ID: "resource_drifted", Kind: "package", Spec: map[string]interface{}{"sourceKind": "brew", "name": "kubectl", "desiredVersion": "1.32.0"},
	})
	if drifted.Status != "drifted" {
		t.Fatalf("drifted status = %s, want drifted", drifted.Status)
	}

	applied := ApplyPackage(context.Background(), fake, DesiredResource{
		ID: "resource_apply", Kind: "package", Spec: map[string]interface{}{"sourceKind": "brew", "name": "helm", "desiredVersion": "latest"},
	})
	if applied.Status != "apply_success" {
		t.Fatalf("applied status = %s, want apply_success", applied.Status)
	}
}

func TestAgentReport_unsupportedMiseProducesUnsupportedAdapter(t *testing.T) {
	event := EvaluateResource(context.Background(), nil, DesiredResource{
		ID: "resource_mise", Kind: "package", Spec: map[string]interface{}{"sourceKind": "mise", "name": "node", "desiredVersion": "22"},
	})
	if event.Status != "unsupported_adapter" {
		t.Fatalf("status = %s, want unsupported_adapter", event.Status)
	}
}

func TestDotfile_rejectsPathOutsideAllowlist(t *testing.T) {
	event := EvaluateResource(context.Background(), nil, DesiredResource{
		ID: "resource_dot", Kind: "dotfile", Spec: map[string]interface{}{"path": "/etc/hosts"},
	})
	if event.Status != "blocked" || !strings.Contains(event.Message, "path_not_allowed") {
		t.Fatalf("event = %+v, want blocked path_not_allowed", event)
	}
}

type fakePackageAdapter struct {
	installed map[string]string
}

func (f *fakePackageAdapter) Check(_ context.Context, name string, desiredVersion string) (string, error) {
	if f.installed[name] == desiredVersion || desiredVersion == "latest" && f.installed[name] != "" {
		return "in_sync", nil
	}
	return "drifted", nil
}

func (f *fakePackageAdapter) Apply(_ context.Context, name string, desiredVersion string) (string, error) {
	f.installed[name] = desiredVersion
	return "apply_success", nil
}
