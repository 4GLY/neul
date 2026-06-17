package agent

import (
	"context"
	"fmt"
	"os"
)

type DesiredResource struct {
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	Name           string                 `json:"name"`
	DesiredVersion int                    `json:"desiredVersion"`
	Spec           map[string]interface{} `json:"spec"`
}

type ResourceEvent struct {
	ResourceID     string `json:"resourceId"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	DesiredVersion int    `json:"desiredVersion"`
	AppliedVersion int    `json:"appliedVersion"`
}

type PackageAdapter interface {
	Check(ctx context.Context, name string, desiredVersion string) (string, error)
	Apply(ctx context.Context, name string, desiredVersion string) (string, error)
}

func EvaluateResource(ctx context.Context, brew PackageAdapter, resource DesiredResource) ResourceEvent {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}
	return EvaluateResourceWithHome(ctx, brew, homeDir, resource)
}

func EvaluateResourceWithHome(ctx context.Context, brew PackageAdapter, homeDir string, resource DesiredResource) ResourceEvent {
	switch resource.Kind {
	case "package":
		sourceKind := stringSpec(resource, "sourceKind")
		if sourceKind != "brew" {
			return unsupportedPackageSourceEvent(resource, sourceKind)
		}
		return CheckPackage(ctx, brew, resource)
	case "dotfile":
		return CheckDotfile(ctx, homeDir, resource)
	default:
		return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: resource.Kind + " resource kind is unsupported", DesiredVersion: resource.DesiredVersion}
	}
}

func CheckPackage(ctx context.Context, adapter PackageAdapter, resource DesiredResource) ResourceEvent {
	if adapter == nil {
		return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: "brew_unavailable: brew adapter unavailable", DesiredVersion: resource.DesiredVersion}
	}
	status, err := adapter.Check(ctx, stringSpec(resource, "name"), stringSpec(resource, "desiredVersion"))
	if err != nil {
		return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: err.Error(), DesiredVersion: resource.DesiredVersion}
	}
	appliedVersion := 0
	if status == "in_sync" {
		appliedVersion = resource.DesiredVersion
	}
	return ResourceEvent{ResourceID: resource.ID, Status: status, Message: "brew check", DesiredVersion: resource.DesiredVersion, AppliedVersion: appliedVersion}
}

func ApplyPackage(ctx context.Context, adapter PackageAdapter, resource DesiredResource) ResourceEvent {
	if adapter == nil {
		return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: "brew_unavailable: brew adapter unavailable", DesiredVersion: resource.DesiredVersion}
	}
	status, err := adapter.Apply(ctx, stringSpec(resource, "name"), stringSpec(resource, "desiredVersion"))
	if err != nil {
		return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: err.Error(), DesiredVersion: resource.DesiredVersion}
	}
	appliedVersion := 0
	if status == "in_sync" {
		appliedVersion = resource.DesiredVersion
	}
	return ResourceEvent{ResourceID: resource.ID, Status: status, Message: "brew apply", DesiredVersion: resource.DesiredVersion, AppliedVersion: appliedVersion}
}

func ApplyPackageResource(ctx context.Context, adapter PackageAdapter, resource DesiredResource) ResourceEvent {
	sourceKind := stringSpec(resource, "sourceKind")
	if sourceKind != "brew" {
		return unsupportedPackageSourceEvent(resource, sourceKind)
	}
	return ApplyPackage(ctx, adapter, resource)
}

func unsupportedPackageSourceEvent(resource DesiredResource, sourceKind string) ResourceEvent {
	if sourceKind == "" {
		sourceKind = "unknown"
	}
	return ResourceEvent{
		ResourceID:     resource.ID,
		Status:         "blocked",
		Message:        fmt.Sprintf("%s package source is unsupported", sourceKind),
		DesiredVersion: resource.DesiredVersion,
	}
}

type disabledBrewAdapter struct{}

func (disabledBrewAdapter) Check(_ context.Context, name string, _ string) (string, error) {
	return "", fmt.Errorf("brew_disabled: package adapters disabled for %s", name)
}

func (disabledBrewAdapter) Apply(_ context.Context, name string, _ string) (string, error) {
	return "", fmt.Errorf("brew_disabled: package adapters disabled for %s", name)
}

func stringSpec(resource DesiredResource, key string) string {
	value, _ := resource.Spec[key].(string)
	return value
}
