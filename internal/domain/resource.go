package domain

type ResourceKind string

const (
	ResourceKindPackage ResourceKind = "package"
	ResourceKindDotfile ResourceKind = "dotfile"
)

type ResourceState string

const (
	ResourceStateInSync  ResourceState = "in_sync"
	ResourceStatePending ResourceState = "pending"
	ResourceStateDrifted ResourceState = "drifted"
	ResourceStateBlocked ResourceState = "blocked"
	ResourceStateUnknown ResourceState = "unknown"
)
