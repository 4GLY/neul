package agent

import "context"

func (c *Client) commandReportFor(ctx context.Context, command agentCommand, resources []DesiredResource) commandReport {
	switch command.Type {
	case "reconcile_now":
		return commandReport{
			MachineID: c.config.MachineID,
			CommandID: command.ID,
			Status:    "finished",
			Events:    applyAllResources(ctx, c.brew, c.config.HomeDir, resources),
		}
	case "repair_drift":
		return commandReport{
			MachineID: c.config.MachineID,
			CommandID: command.ID,
			Status:    "finished",
			Events:    applyRequestedResources(ctx, c.brew, c.config.HomeDir, resources, commandResourceIDs(command.Payload)),
		}
	default:
		return commandReport{
			MachineID: c.config.MachineID,
			CommandID: command.ID,
			Status:    "unsupported_command",
		}
	}
}

func applyAllResources(ctx context.Context, adapter PackageAdapter, homeDir string, resources []DesiredResource) []ResourceEvent {
	events := make([]ResourceEvent, 0, len(resources))
	for _, resource := range resources {
		events = append(events, applyResource(ctx, adapter, homeDir, resource))
	}
	return events
}

func applyRequestedResources(ctx context.Context, adapter PackageAdapter, homeDir string, resources []DesiredResource, ids []string) []ResourceEvent {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[string]DesiredResource, len(resources))
	for _, resource := range resources {
		byID[resource.ID] = resource
	}
	events := make([]ResourceEvent, 0, len(ids))
	for _, id := range ids {
		resource, ok := byID[id]
		if !ok {
			events = append(events, ResourceEvent{Status: "blocked", Message: "resource_not_found:" + id})
			continue
		}
		events = append(events, applyResource(ctx, adapter, homeDir, resource))
	}
	return events
}

func applyResource(ctx context.Context, adapter PackageAdapter, homeDir string, resource DesiredResource) ResourceEvent {
	switch resource.Kind {
	case "package":
		return ApplyPackageResource(ctx, adapter, resource)
	case "dotfile":
		return ApplyDotfile(ctx, homeDir, resource)
	default:
		return ResourceEvent{ResourceID: resource.ID, Status: "blocked", Message: resource.Kind + " resource kind is unsupported", DesiredVersion: resource.DesiredVersion}
	}
}

func commandResourceIDs(payload map[string]interface{}) []string {
	if payload == nil {
		return nil
	}
	raw, ok := payload["resourceIds"]
	if !ok {
		return nil
	}
	switch values := raw.(type) {
	case []string:
		ids := make([]string, 0, len(values))
		seen := make(map[string]struct{}, len(values))
		for _, id := range values {
			ids = appendResourceIDOnce(ids, seen, id)
		}
		return ids
	case []interface{}:
		ids := make([]string, 0, len(values))
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			id, ok := value.(string)
			if ok {
				ids = appendResourceIDOnce(ids, seen, id)
			}
		}
		return ids
	default:
		return nil
	}
}

func appendResourceIDOnce(ids []string, seen map[string]struct{}, id string) []string {
	if _, ok := seen[id]; ok {
		return ids
	}
	seen[id] = struct{}{}
	return append(ids, id)
}
