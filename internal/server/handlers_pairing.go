package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const pairingTTL = 10 * time.Minute

func handlePairInit(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := clock().UTC()
		code, err := randomToken("pair")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "token_generation_failed", "Could not create pairing code.")
			return
		}
		_, err = db.ExecContext(
			r.Context(),
			`INSERT INTO pairing_codes (id, code_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
			"pairing_"+hashSecret(code)[:16],
			hashSecret(code),
			now.Add(pairingTTL).Format(time.RFC3339Nano),
			now.Format(time.RFC3339Nano),
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "pairing_create_failed", "Could not create pairing code.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{
			"code":      code,
			"expiresAt": now.Add(pairingTTL).Format(time.RFC3339Nano),
		})
	})
}

func handlePairClaim(db *sql.DB, clock func() time.Time) http.HandlerFunc {
	type requestBody struct {
		Code    string           `json:"code"`
		Machine pairClaimMachine `json:"machine"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if body.Code == "" || body.Machine.Name == "" || body.Machine.OS == "" || body.Machine.Arch == "" {
			writeJSONError(w, http.StatusBadRequest, "pair_claim_invalid", "Pairing code and machine metadata are required.")
			return
		}

		now := clock().UTC()
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not claim pairing code.")
			return
		}
		defer func() {
			_ = tx.Rollback()
		}()

		pairing, err := queryPairingForClaim(r, tx, body.Code)
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "pairing_code_not_found", "Pairing code was not found.")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "pairing_lookup_failed", "Could not look up pairing code.")
			return
		}
		if pairing.usedAt.Valid {
			writeJSONError(w, http.StatusConflict, "code_used", "Pairing code was already used.")
			return
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, pairing.expiresAt)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "pairing_expiry_invalid", "Pairing code expiry is invalid.")
			return
		}
		if now.After(expiresAt) {
			writeJSONError(w, http.StatusGone, "pairing_code_expired", "Pairing code expired.")
			return
		}

		machineID := "machine_" + hashSecret(body.Code + body.Machine.Name)[:16]
		machineToken, err := randomToken("mtn")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "token_generation_failed", "Could not create machine token.")
			return
		}
		if err := insertClaimedMachine(r, tx, machineID, machineToken, body.Machine, now); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "machine_create_failed", "Could not create machine.")
			return
		}
		_, err = tx.ExecContext(
			r.Context(),
			`UPDATE pairing_codes SET used_at = ?, machine_id = ? WHERE id = ?`,
			now.Format(time.RFC3339Nano),
			machineID,
			pairing.id,
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "pairing_consume_failed", "Could not consume pairing code.")
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not claim pairing code.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{
			"machineId":    machineID,
			"machineToken": machineToken,
		})
	}
}

func handlePairPoll(db *sql.DB, clock func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			writeJSONError(w, http.StatusBadRequest, "pairing_code_required", "Pairing code is required.")
			return
		}
		var machineID sql.NullString
		var usedAt sql.NullString
		var expiresAt string
		err := db.QueryRowContext(
			r.Context(),
			`SELECT machine_id, used_at, expires_at FROM pairing_codes WHERE code_hash = ?`,
			hashSecret(code),
		).Scan(&machineID, &usedAt, &expiresAt)
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "pairing_code_not_found", "Pairing code was not found.")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "pairing_lookup_failed", "Could not look up pairing code.")
			return
		}
		if usedAt.Valid && machineID.Valid {
			writeJSON(w, http.StatusOK, map[string]string{
				"status":    "claimed",
				"machineId": machineID.String,
				"expiresAt": expiresAt,
			})
			return
		}
		expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "pairing_expiry_invalid", "Pairing code expiry is invalid.")
			return
		}
		if clock().UTC().After(expiry) {
			writeJSON(w, http.StatusOK, map[string]string{
				"status":    "expired",
				"expiresAt": expiresAt,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":    "pending",
			"expiresAt": expiresAt,
		})
	})
}

type pairingRow struct {
	id        string
	expiresAt string
	usedAt    sql.NullString
}

func queryPairingForClaim(r *http.Request, tx *sql.Tx, code string) (pairingRow, error) {
	var row pairingRow
	err := tx.QueryRowContext(
		r.Context(),
		`SELECT id, expires_at, used_at FROM pairing_codes WHERE code_hash = ?`,
		hashSecret(code),
	).Scan(&row.id, &row.expiresAt, &row.usedAt)
	return row, err
}

func insertClaimedMachine(r *http.Request, tx *sql.Tx, machineID string, machineToken string, machine pairClaimMachine, now time.Time) error {
	_, err := tx.ExecContext(
		r.Context(),
		`INSERT INTO machines (id, name, os, arch, agent_version, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		machineID,
		machine.Name,
		machine.OS,
		machine.Arch,
		machine.AgentVersion,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert machine: %w", err)
	}
	_, err = tx.ExecContext(
		r.Context(),
		`INSERT INTO machine_tokens (id, machine_id, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		"machine_token_"+hashSecret(machineToken)[:16],
		machineID,
		hashSecret(machineToken),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert machine token: %w", err)
	}
	return nil
}

type pairClaimMachine struct {
	Name         string `json:"name"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	AgentVersion string `json:"agentVersion"`
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
