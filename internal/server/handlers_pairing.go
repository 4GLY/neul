package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
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

		approval, approvalLinked, err := approvalForPairingClaim(r, tx, pairing.id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_lookup_failed", "Could not look up approval metadata.")
			return
		}
		if approvalLinked && !sameMachinePreview(approval.Machine, body.Machine) {
			writeJSONError(w, http.StatusConflict, "approval_machine_metadata_mismatch", "Machine metadata does not match the approved request.")
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
		if err := markExistingResourcesPendingForMachine(r, tx, machineID, now); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "machine_pending_failed", "Could not mark machine pending.")
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
		if approvalLinked {
			err = markApprovalClaimed(r.Context(), tx, approvalClaimedUpdate{
				ApprovalID:  approval.ID,
				MachineID:   machineID,
				ClaimedAt:   now.Format(time.RFC3339Nano),
				RetainUntil: now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			})
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "approval_claim_failed", "Could not mark approval claimed.")
				return
			}
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

func handleApprovalStart(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters) http.HandlerFunc {
	type requestBody struct {
		Nonce             string           `json:"nonce"`
		VerifierChallenge string           `json:"verifierChallenge"`
		Machine           pairClaimMachine `json:"machine"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		now := clock().UTC()
		if !limits.allowApprovalStart(requestIP(r), now) {
			writeJSONError(w, http.StatusTooManyRequests, "approval_start_rate_limited", "Approval start rate limit exceeded.")
			return
		}
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if !validBase64URLBytes(body.Nonce, 32) ||
			!validBase64URLBytes(body.VerifierChallenge, sha256.Size) ||
			body.Machine.Name == "" ||
			body.Machine.OS == "" ||
			body.Machine.Arch == "" ||
			body.Machine.AgentVersion == "" {
			writeJSONError(w, http.StatusBadRequest, "approval_start_invalid", "Approval nonce, verifier challenge, and machine preview are required.")
			return
		}
		approvalID, err := randomToken("approval")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_start_failed", "Could not create approval.")
			return
		}
		csrf, err := randomToken("csrf")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_start_failed", "Could not create approval.")
			return
		}
		comparisonCode, err := randomComparisonCode()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_start_failed", "Could not create approval.")
			return
		}
		expiresAt := now.Add(pairingTTL).Format(time.RFC3339Nano)
		record := approvalRecord{
			ID:                approvalID,
			NonceHash:         hashSecret(body.Nonce),
			VerifierChallenge: body.VerifierChallenge,
			CSRFToken:         csrf,
			ComparisonCode:    comparisonCode,
			State:             "pending",
			Machine:           body.Machine,
			CreatedAt:         now.Format(time.RFC3339Nano),
			ExpiresAt:         expiresAt,
		}
		if err := insertApprovalRecord(r.Context(), db, record); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_start_failed", "Could not create approval.")
			return
		}
		approvalURL := requestOrigin(r) + "/enroll/approve?approval=" + url.QueryEscape(approvalID) + "&nonce=" + url.QueryEscape(body.Nonce)
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"approvalId":     approvalID,
			"approvalUrl":    approvalURL,
			"comparisonCode": comparisonCode,
			"expiresAt":      expiresAt,
			"pollAfterMs":    2000,
		})
	}
}

func handleApprovalApprove(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters) http.HandlerFunc {
	type requestBody struct {
		ApprovalID string `json:"approvalId"`
		Nonce      string `json:"nonce"`
		CSRFToken  string `json:"csrfToken"`
		Decision   string `json:"decision"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		now := clock().UTC()
		session, ok := approvalOwnerSessionKey(w, r, db)
		if !ok {
			return
		}
		if !limits.allowApprovalApprove(session, now) {
			writeJSONError(w, http.StatusTooManyRequests, "approval_approve_rate_limited", "Approval approve rate limit exceeded.")
			return
		}
		if !sameOriginRequest(r) {
			writeJSONError(w, http.StatusForbidden, "approval_origin_invalid", "Approval request origin is invalid.")
			return
		}
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if body.ApprovalID == "" || body.Nonce == "" || body.CSRFToken == "" || (body.Decision != "approve" && body.Decision != "cancel") {
			writeJSONError(w, http.StatusBadRequest, "approval_approve_invalid", "Approval id, nonce, CSRF token, and decision are required.")
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not approve request.")
			return
		}
		defer func() {
			_ = tx.Rollback()
		}()
		record, err := getApprovalRecordForUpdate(r.Context(), tx, body.ApprovalID)
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "approval_not_found", "Approval was not found.")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_lookup_failed", "Could not look up approval.")
			return
		}
		if record.NonceHash != hashSecret(body.Nonce) {
			writeJSONError(w, http.StatusBadRequest, "approval_approve_invalid", "Approval id, nonce, CSRF token, and decision are required.")
			return
		}
		if subtle.ConstantTimeCompare([]byte(record.CSRFToken), []byte(body.CSRFToken)) != 1 {
			writeJSONError(w, http.StatusForbidden, "approval_csrf_invalid", "Approval CSRF token is invalid.")
			return
		}
		if approvalRecordExpired(record, now) {
			writeJSONError(w, http.StatusGone, "approval_expired", "Approval expired.")
			return
		}
		if record.State != "pending" {
			writeJSONError(w, http.StatusConflict, "approval_not_pending", "Approval is not pending.")
			return
		}
		status := "approved"
		if body.Decision == "cancel" {
			status = "cancelled"
			err = markApprovalCancelled(r.Context(), tx, record.ID, now.Format(time.RFC3339Nano))
		} else {
			err = markApprovalApproved(r.Context(), tx, record.ID, now.Format(time.RFC3339Nano))
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_update_failed", "Could not update approval.")
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not approve request.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": status, "expiresAt": record.ExpiresAt})
	}
}

func handleApprovalClaim(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters) http.HandlerFunc {
	type requestBody struct {
		ApprovalID string `json:"approvalId"`
		Nonce      string `json:"nonce"`
		Verifier   string `json:"verifier"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		now := clock().UTC()
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if body.ApprovalID == "" || body.Nonce == "" || body.Verifier == "" {
			writeJSONError(w, http.StatusBadRequest, "approval_claim_invalid", "Approval id, nonce, and verifier are required.")
			return
		}
		ip := requestIP(r)
		if !limits.allowApprovalClaimIP(ip, now) {
			writeJSONError(w, http.StatusTooManyRequests, "approval_claim_rate_limited", "Approval claim rate limit exceeded.")
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not claim approval.")
			return
		}
		defer func() {
			_ = tx.Rollback()
		}()
		record, err := getApprovalRecordForUpdate(r.Context(), tx, body.ApprovalID)
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "approval_not_found", "Approval was not found.")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_lookup_failed", "Could not look up approval.")
			return
		}
		if !limits.allowApprovalClaimID(record.ID, now) {
			writeJSONError(w, http.StatusTooManyRequests, "approval_claim_rate_limited", "Approval claim rate limit exceeded.")
			return
		}
		if record.State == "locked" {
			writeJSONError(w, http.StatusLocked, "approval_locked", "Approval is locked.")
			return
		}
		if !approvalClaimProofValid(record, body.Nonce, body.Verifier) {
			status := http.StatusForbidden
			code := "approval_claim_denied"
			if record.ClaimFailureCount+1 >= 6 {
				if err := lockApprovalRecord(r.Context(), tx, record.ID); err != nil {
					writeJSONError(w, http.StatusInternalServerError, "approval_lock_failed", "Could not lock approval.")
					return
				}
				status = http.StatusLocked
				code = "approval_locked"
			}
			if err := incrementApprovalClaimFailure(r.Context(), tx, approvalClaimFailureUpdate{
				ApprovalID: record.ID,
				FailedAt:   now.Format(time.RFC3339Nano),
				FailureIP:  requestIP(r),
			}); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "approval_failure_update_failed", "Could not update approval failure count.")
				return
			}
			if err := tx.Commit(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not claim approval.")
				return
			}
			if status == http.StatusLocked {
				writeJSONError(w, status, code, "Approval is locked.")
				return
			}
			writeJSONError(w, status, code, "Approval claim was denied.")
			return
		}
		if approvalRecordExpired(record, now) && (record.State == "pending" || record.State == "approved") {
			writeJSONError(w, http.StatusGone, "approval_expired", "Approval expired.")
			return
		}
		switch record.State {
		case "pending":
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":            "pending",
				"approvalExpiresAt": record.ExpiresAt,
				"retryAfterMs":      2000,
			})
		case "approved":
			pairCode, pairingID, pairExpiresAt, err := createApprovalPairingCode(r, tx, now)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "approval_pair_code_failed", "Could not create pair code.")
				return
			}
			err = markApprovalPairCodeIssued(r.Context(), tx, approvalPairCodeIssuedUpdate{
				ApprovalID: record.ID,
				PairingID:  pairingID,
				IssuedAt:   now.Format(time.RFC3339Nano),
			})
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "approval_update_failed", "Could not update approval.")
				return
			}
			if err := tx.Commit(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "transaction_failed", "Could not claim approval.")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{
				"status":            "approved",
				"pairCode":          pairCode,
				"pairCodeExpiresAt": pairExpiresAt,
			})
		case "pair_code_issued":
			writeJSONError(w, http.StatusConflict, "approval_pair_code_issued", "Approval pair code was already issued.")
		case "claimed":
			writeJSON(w, http.StatusOK, map[string]string{
				"status":    "claimed",
				"machineId": record.ClaimedMachineID.String,
				"claimedAt": record.ClaimedAt.String,
			})
		case "cancelled":
			writeJSONError(w, http.StatusConflict, "approval_cancelled", "Approval was cancelled.")
		default:
			writeJSONError(w, http.StatusConflict, "approval_cancelled", "Approval was cancelled.")
		}
	}
}

func handleApprovalStatus(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := clock().UTC()
		session, ok := approvalOwnerSessionKey(w, r, db)
		if !ok {
			return
		}
		if !limits.allowApprovalStatus(session, requestIP(r), now) {
			writeJSONError(w, http.StatusTooManyRequests, "approval_status_rate_limited", "Approval status rate limit exceeded.")
			return
		}
		approvalID := r.URL.Query().Get("approvalId")
		if approvalID == "" {
			approvalID = r.URL.Query().Get("approval")
		}
		if approvalID == "" {
			writeJSONError(w, http.StatusNotFound, "approval_not_found", "Approval was not found.")
			return
		}
		record, err := loadApprovalRecord(r.Context(), db, approvalID)
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "approval_not_found", "Approval was not found.")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval_lookup_failed", "Could not look up approval.")
			return
		}
		status := record.State
		if approvalRecordExpired(record, now) && (record.State == "pending" || record.State == "approved") {
			status = "expired"
		}
		if status == "pair_code_issued" {
			status = "approved"
		}
		if status == "claimed" {
			writeJSON(w, http.StatusOK, map[string]string{
				"status":    "claimed",
				"machineId": record.ClaimedMachineID.String,
				"claimedAt": record.ClaimedAt.String,
				"expiresAt": record.ExpiresAt,
			})
			return
		}
		if status == "pending" || status == "approved" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":         status,
				"approvalId":     record.ID,
				"expiresAt":      record.ExpiresAt,
				"csrfToken":      record.CSRFToken,
				"comparisonCode": record.ComparisonCode,
				"machine":        record.Machine,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":    status,
			"expiresAt": record.ExpiresAt,
		})
	}
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

func approvalForPairingClaim(r *http.Request, tx *sql.Tx, pairingID string) (approvalRecord, bool, error) {
	record, err := getApprovalRecordByPairingIDForUpdate(r.Context(), tx, pairingID)
	if err == sql.ErrNoRows {
		return approvalRecord{}, false, nil
	}
	if err != nil {
		return approvalRecord{}, false, err
	}
	return record, true, nil
}

func sameMachinePreview(expected pairClaimMachine, got pairClaimMachine) bool {
	return expected.Name == got.Name &&
		expected.OS == got.OS &&
		expected.Arch == got.Arch &&
		expected.AgentVersion == got.AgentVersion
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

func validBase64URLBytes(value string, size int) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == size
}

func randomComparisonCode() (string, error) {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("random comparison code: %w", err)
	}
	n := binary.BigEndian.Uint32(bytes[:]) % 1_000_000
	return fmt.Sprintf("%03d-%03d", n/1000, n%1000), nil
}

func requestOrigin(r *http.Request) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return scheme + "://" + host
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func approvalOwnerSessionKey(w http.ResponseWriter, r *http.Request, db *sql.DB) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSONError(w, http.StatusUnauthorized, "owner_session_required", "Owner session is required.")
		return "", false
	}
	sessionHash := hashSecret(cookie.Value)
	var count int
	err = db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sessions WHERE session_hash = ?`, sessionHash).Scan(&count)
	if err != nil || count != 1 {
		writeJSONError(w, http.StatusUnauthorized, "owner_session_required", "Owner session is required.")
		return "", false
	}
	return sessionHash, true
}

func sameOriginRequest(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == requestOrigin(r)
	}
	referer := r.Header.Get("Referer")
	if referer == "" {
		return false
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		return false
	}
	return parsed.Scheme+"://"+parsed.Host == requestOrigin(r)
}

func approvalRecordExpired(record approvalRecord, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	return err == nil && now.After(expiresAt)
}

func approvalClaimProofValid(record approvalRecord, nonce string, verifier string) bool {
	if record.NonceHash != hashSecret(nonce) {
		return false
	}
	decodedVerifier, err := base64.RawURLEncoding.DecodeString(verifier)
	if err != nil || len(decodedVerifier) < 32 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(challenge), []byte(record.VerifierChallenge)) == 1
}

func createApprovalPairingCode(r *http.Request, tx *sql.Tx, now time.Time) (string, string, string, error) {
	code, err := randomToken("pair")
	if err != nil {
		return "", "", "", fmt.Errorf("generate pair code: %w", err)
	}
	pairingID := "pairing_" + hashSecret(code)[:16]
	expiresAt := now.Add(pairingTTL).Format(time.RFC3339Nano)
	_, err = tx.ExecContext(
		r.Context(),
		`INSERT INTO pairing_codes (id, code_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		pairingID,
		hashSecret(code),
		expiresAt,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", "", "", fmt.Errorf("insert approval pairing code: %w", err)
	}
	return code, pairingID, expiresAt, nil
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
