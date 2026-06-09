package domain

import (
	"testing"
	"time"
)

func TestMachineStatus_whenHeartbeatFreshAndNoDrift_isHealthy(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add(-30 * time.Second),
		HasReport:       true,
		DriftCount:      0,
		PendingCount:    0,
		BlockedCount:    0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusHealthy {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusHealthy)
	}
}

func TestMachineStatus_whenHeartbeatMissing_isUnknown(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		DriftCount:   0,
		PendingCount: 0,
		BlockedCount: 0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusUnknown {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusUnknown)
	}
}

func TestMachineStatus_whenHeartbeatFreshButReportMissing_isUnknown(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add(-30 * time.Second),
		HasReport:       false,
		DriftCount:      0,
		PendingCount:    0,
		BlockedCount:    0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusUnknown {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusUnknown)
	}
}

func TestMachineStatus_whenHeartbeatIsOlderThanFiveMinutes_isOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add((-5 * time.Minute) - time.Second),
		HasReport:       true,
		DriftCount:      1,
		PendingCount:    1,
		BlockedCount:    1,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusOffline {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusOffline)
	}
}

func TestMachineStatus_whenHeartbeatIsStaleAndReportMissing_isOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add((-5 * time.Minute) - time.Second),
		HasReport:       false,
		DriftCount:      0,
		PendingCount:    0,
		BlockedCount:    0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusOffline {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusOffline)
	}
}

func TestMachineStatus_whenHeartbeatIsStaleAndReportHealthy_isOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add((-5 * time.Minute) - time.Second),
		HasReport:       true,
		DriftCount:      0,
		PendingCount:    0,
		BlockedCount:    0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusOffline {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusOffline)
	}
}

func TestMachineStatus_whenHeartbeatIsJustUnderFiveMinutes_isNotOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add((-5 * time.Minute) + time.Second),
		HasReport:       true,
		DriftCount:      1,
		PendingCount:    0,
		BlockedCount:    0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusDrifted {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusDrifted)
	}
}

func TestMachineStatus_whenBlockedExists_isBlocked(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add(-30 * time.Second),
		HasReport:       true,
		DriftCount:      0,
		PendingCount:    0,
		BlockedCount:    1,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusBlocked {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusBlocked)
	}
}

func TestMachineStatus_whenPendingExists_isPending(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := MachineSnapshot{
		LastHeartbeatAt: now.Add(-30 * time.Second),
		HasReport:       true,
		DriftCount:      0,
		PendingCount:    1,
		BlockedCount:    0,
	}

	status := ComputeMachineStatus(machine, now)

	if status != MachineStatusPending {
		t.Fatalf("ComputeMachineStatus() = %q, want %q", status, MachineStatusPending)
	}
}
