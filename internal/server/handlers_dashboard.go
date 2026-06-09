package server

import (
	"database/sql"
	"net/http"
	"time"
)

func handleDashboard(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machines, err := queryMachineSummaries(r, db, clock().UTC())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "dashboard_query_failed", "Could not read dashboard.")
			return
		}
		metrics := map[string]int{
			"total":   len(machines),
			"healthy": 0,
			"drifted": 0,
			"pending": 0,
			"offline": 0,
			"blocked": 0,
			"unknown": 0,
		}
		for _, machine := range machines {
			metrics[machine.Status]++
		}
		body := map[string]interface{}{
			"metrics":  metrics,
			"machines": machines,
			"activity": []interface{}{},
			"ledger":   []interface{}{},
		}
		if len(machines) == 0 {
			body["emptyState"] = map[string]string{
				"action": "create_pairing_code",
				"title":  "첫 머신을 등록하세요",
			}
		}
		writeJSON(w, http.StatusOK, body)
	})
}

func handleListMachines(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machines, err := queryMachineSummaries(r, db, clock().UTC())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "machines_query_failed", "Could not read machines.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"machines": machines})
	})
}

func handleGetMachine(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		machineID := r.PathValue("machineId")
		machines, err := queryMachineSummaries(r, db, clock().UTC())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "machine_query_failed", "Could not read machine.")
			return
		}
		var selected machineSummary
		found := false
		for _, machine := range machines {
			if machine.ID == machineID {
				selected = machine
				found = true
				break
			}
		}
		if !found {
			writeJSONError(w, http.StatusNotFound, "machine_not_found", "Machine was not found.")
			return
		}
		events, err := queryMachineEvents(r, db, machineID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "events_query_failed", "Could not read machine events.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"machine":         selected,
			"events":          events,
			"latestReconcile": latestEvent(events),
			"driftSummary": map[string]int{
				"drifted": selected.DriftCount,
				"pending": selected.PendingCount,
				"blocked": selected.BlockedCount,
			},
		})
	})
}
