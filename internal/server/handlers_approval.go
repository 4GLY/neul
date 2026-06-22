package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

func handleApprovalStart(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters, publicOrigin string) http.HandlerFunc {
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
		approvalURL := requestOrigin(r, publicOrigin) + "/enroll/approve?approval=" + url.QueryEscape(approvalID) + "&nonce=" + url.QueryEscape(body.Nonce)
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"approvalId":     approvalID,
			"approvalUrl":    approvalURL,
			"comparisonCode": comparisonCode,
			"expiresAt":      expiresAt,
			"pollAfterMs":    2000,
		})
	}
}

func handleApprovalApprove(db *sql.DB, clock func() time.Time, limits *approvalRateLimiters, publicOrigin string) http.HandlerFunc {
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
		if !sameOriginRequest(r, publicOrigin) {
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
