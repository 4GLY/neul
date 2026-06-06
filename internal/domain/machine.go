package domain

import "time"

type MachineStatus string

const (
	MachineStatusHealthy MachineStatus = "healthy"
	MachineStatusDrifted MachineStatus = "drifted"
	MachineStatusPending MachineStatus = "pending"
	MachineStatusOffline MachineStatus = "offline"
	MachineStatusBlocked MachineStatus = "blocked"
)

const OfflineThreshold = 5 * time.Minute

type MachineSnapshot struct {
	LastHeartbeatAt time.Time
	DriftCount      int
	PendingCount    int
	BlockedCount    int
}

func ComputeMachineStatus(machine MachineSnapshot, now time.Time) MachineStatus {
	if now.Sub(machine.LastHeartbeatAt) > OfflineThreshold {
		return MachineStatusOffline
	}
	if machine.BlockedCount > 0 {
		return MachineStatusBlocked
	}
	if machine.DriftCount > 0 {
		return MachineStatusDrifted
	}
	if machine.PendingCount > 0 {
		return MachineStatusPending
	}
	return MachineStatusHealthy
}
