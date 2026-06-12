package server

import (
	"fmt"

	"github.com/4gly/neul/internal/domain"
	"github.com/4gly/neul/internal/domain/dotfiles"
)

func validPackageSource(sourceKind string) bool {
	return sourceKind == "brew" || sourceKind == "apt" || sourceKind == "mise"
}

func mergeResourcePatch(homeDir string, resource resourceResponse, patch map[string]interface{}) (map[string]interface{}, string, error) {
	nextSpec := make(map[string]interface{}, len(resource.Spec)+len(patch))
	for key, value := range resource.Spec {
		nextSpec[key] = value
	}
	for key, value := range patch {
		if value == nil {
			return nil, "", fmt.Errorf("nil patch value")
		}
		nextSpec[key] = value
	}
	nextName := resource.Name
	if rawName, ok := nextSpec["name"]; ok {
		name, ok := rawName.(string)
		if !ok || name == "" {
			return nil, "", fmt.Errorf("invalid resource name")
		}
		nextName = name
	}
	if resource.Kind == string(domain.ResourceKindPackage) {
		sourceKind, ok := nextSpec["sourceKind"].(string)
		if !ok || !validPackageSource(sourceKind) {
			return nil, "", fmt.Errorf("invalid package source")
		}
		desiredVersion, ok := nextSpec["desiredVersion"].(string)
		if !ok || desiredVersion == "" {
			return nil, "", fmt.Errorf("invalid package version")
		}
		nextSpec["name"] = nextName
	}
	if resource.Kind == string(domain.ResourceKindDotfile) {
		path, ok := nextSpec["path"].(string)
		if !ok {
			return nil, "", fmt.Errorf("invalid dotfile path")
		}
		normalizedPath, err := dotfiles.NormalizeAllowedPath(homeDir, path)
		if err != nil {
			return nil, "", err
		}
		nextSpec["path"] = normalizedPath
		nextName = normalizedPath
	}
	return nextSpec, nextName, nil
}
