package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/4gly/neul/internal/domain"
)

type machineSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	AgentVersion    string `json:"agentVersion"`
	Status          string `json:"status"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	LastReconcileAt string `json:"lastReconcileAt,omitempty"`
	DriftCount      int    `json:"driftCount"`
	PendingCount    int    `json:"pendingCount"`
	BlockedCount    int    `json:"blockedCount"`
	ResourceCount   int    `json:"resourceCount"`
	AppliedCount    int    `json:"appliedCount"`
}

type machineCounts struct {
	Drifted         int
	Pending         int
	Blocked         int
	ResourceCount   int
	AppliedCount    int
	HasReport       bool
	LastReconcileAt string
}

func queryMachineSummaries(r *http.Request, db *sql.DB, now time.Time) ([]machineSummary, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT id, name, os, arch, COALESCE(last_heartbeat_at, ''), COALESCE(agent_version, '') FROM machines ORDER BY created_at DESC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query machines: %w", err)
	}
	defer rows.Close()

	machines := make([]machineSummary, 0)
	for rows.Next() {
		var machine machineSummary
		var lastHeartbeat string
		if err := rows.Scan(&machine.ID, &machine.Name, &machine.OS, &machine.Arch, &lastHeartbeat, &machine.AgentVersion); err != nil {
			return nil, fmt.Errorf("scan machine: %w", err)
		}
		if lastHeartbeat != "" {
			machine.LastHeartbeatAt = lastHeartbeat
		}
		machines = append(machines, machine)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machines: %w", err)
	}

	countsByMachine, err := queryMachineCountsByMachine(r, db)
	if err != nil {
		return nil, err
	}
	latestReports, err := queryLatestReports(r, db)
	if err != nil {
		return nil, err
	}
	for index := range machines {
		counts := countsByMachine[machines[index].ID]
		if latestReport, ok := latestReports[machines[index].ID]; ok {
			counts.HasReport = true
			counts.LastReconcileAt = latestReport
		}
		machines[index].DriftCount = counts.Drifted
		machines[index].PendingCount = counts.Pending
		machines[index].BlockedCount = counts.Blocked
		machines[index].ResourceCount = counts.ResourceCount
		machines[index].AppliedCount = counts.AppliedCount
		machines[index].LastReconcileAt = counts.LastReconcileAt
		snapshot := domain.MachineSnapshot{
			HasReport:    counts.HasReport,
			DriftCount:   counts.Drifted,
			PendingCount: counts.Pending,
			BlockedCount: counts.Blocked,
		}
		if machines[index].LastHeartbeatAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, machines[index].LastHeartbeatAt)
			if err != nil {
				return nil, fmt.Errorf("parse heartbeat: %w", err)
			}
			snapshot.LastHeartbeatAt = parsed
		}
		machines[index].Status = string(domain.ComputeMachineStatus(snapshot, now))
	}
	return machines, nil
}

func queryMachineCountsByMachine(r *http.Request, db *sql.DB) (map[string]machineCounts, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT machine_id, resource_id, status
		 FROM (
			 SELECT
				 rr.machine_id,
				 e.resource_id,
				 e.status,
				 ROW_NUMBER() OVER (
					 PARTITION BY rr.machine_id, e.resource_id
					 ORDER BY unixepoch(e.created_at) DESC, e.created_at DESC, e.rowid DESC
				 ) AS row_rank
			 FROM reconcile_events e
			 JOIN reconcile_runs rr ON rr.id = e.run_id
			 JOIN resources r ON r.id = e.resource_id
			 WHERE rr.status IN ('reported', 'finished')
		 )
		 WHERE row_rank = 1
		 ORDER BY machine_id ASC, resource_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query counts: %w", err)
	}
	defer rows.Close()

	countsByMachine := make(map[string]machineCounts)
	for rows.Next() {
		var machineID string
		var resourceID string
		var status string
		if err := rows.Scan(&machineID, &resourceID, &status); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		counts := countsByMachine[machineID]
		counts.ResourceCount++
		switch domain.ResourceState(status) {
		case domain.ResourceStateDrifted:
			counts.Drifted++
		case domain.ResourceStateBlocked:
			counts.Blocked++
		case domain.ResourceStatePending:
			counts.Pending++
		case domain.ResourceStateInSync:
			counts.AppliedCount++
		}
		countsByMachine[machineID] = counts
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate counts: %w", err)
	}
	return countsByMachine, nil
}

func queryLatestReports(r *http.Request, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT machine_id, created_at
		 FROM (
			 SELECT
				 machine_id,
				 created_at,
				 ROW_NUMBER() OVER (
					 PARTITION BY machine_id
					 ORDER BY unixepoch(created_at) DESC, created_at DESC, rowid DESC
				 ) AS row_rank
			 FROM reconcile_runs
			 WHERE status IN ('reported', 'finished')
		 )
		 WHERE row_rank = 1`,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest reports: %w", err)
	}
	defer rows.Close()

	latestReports := make(map[string]string)
	for rows.Next() {
		var machineID string
		var createdAt string
		if err := rows.Scan(&machineID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan latest report: %w", err)
		}
		latestReports[machineID] = createdAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest reports: %w", err)
	}
	return latestReports, nil
}

func queryMachineEvents(r *http.Request, db *sql.DB, machineID string) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(
		r.Context(),
		`SELECT e.id, COALESCE(e.resource_id, ''), e.status, COALESCE(e.message, ''), e.created_at
		 FROM reconcile_events e
		 JOIN reconcile_runs rr ON rr.id = e.run_id
		 WHERE rr.machine_id = ?
		 ORDER BY unixepoch(e.created_at) DESC, e.created_at DESC, e.rowid DESC
		 LIMIT 25`,
		machineID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id string
		var resourceID string
		var status string
		var message string
		var createdAt string
		if err := rows.Scan(&id, &resourceID, &status, &message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, map[string]interface{}{
			"id":         id,
			"resourceId": resourceID,
			"status":     status,
			"message":    message,
			"createdAt":  createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func latestEvent(events []map[string]interface{}) interface{} {
	if len(events) == 0 {
		return nil
	}
	return events[0]
}
