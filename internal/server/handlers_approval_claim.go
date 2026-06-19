package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func handleApprovalClaim(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters) http.HandlerFunc {
	type requestBody struct {
		ApprovalID string `json:"approvalId"`
		Nonce      string `json:"nonce"`
		Verifier   string `json:"verifier"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		now := clock().UTC()
		ip := requestIP(r)
		if !limits.allowApprovalClaimIP(ip, now) {
			writeJSONError(w, http.StatusTooManyRequests, "approval_claim_rate_limited", "Approval claim rate limit exceeded.")
			return
		}
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if body.ApprovalID == "" || body.Nonce == "" || body.Verifier == "" {
			writeJSONError(w, http.StatusBadRequest, "approval_claim_invalid", "Approval id, nonce, and verifier are required.")
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
