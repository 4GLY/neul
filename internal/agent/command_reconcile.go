package agent

import "context"

func (c *Client) commandReportFor(ctx context.Context, command agentCommand, resources []DesiredResource) commandReport {
	switch command.Type {
	case "reconcile_now":
		return commandReport{
			MachineID: c.config.MachineID,
			CommandID: command.ID,
			Status:    "finished",
			Events:    applyAllPackageResources(ctx, c.brew, resources),
		}
	case "repair_drift":
		return commandReport{
			MachineID: c.config.MachineID,
			CommandID: command.ID,
			Status:    "finished",
			Events:    applyRequestedPackageResources(ctx, c.brew, resources, commandResourceIDs(command.Payload)),
		}
	default:
		return commandReport{
			MachineID: c.config.MachineID,
			CommandID: command.ID,
			Status:    "unsupported_command",
		}
	}
}

func applyAllPackageResources(ctx context.Context, adapter PackageAdapter, resources []DesiredResource) []ResourceEvent {
	events := make([]ResourceEvent, 0, len(resources))
	for _, resource := range resources {
		if resource.Kind != "package" {
			continue
		}
		events = append(events, ApplyPackageResource(ctx, adapter, resource))
	}
	return events
}

func applyRequestedPackageResources(ctx context.Context, adapter PackageAdapter, resources []DesiredResource, ids []string) []ResourceEvent {
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
		if resource.Kind != "package" {
			continue
		}
		events = append(events, ApplyPackageResource(ctx, adapter, resource))
	}
	return events
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
