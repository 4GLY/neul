package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApprovalClaim_whenPending_returnsRetryAfter(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})
	approval := startApprovalForTest(t, router, approvalStartForTest{})

	recorder := claimApprovalForTest(t, router, approval)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Status            string `json:"status"`
		ApprovalExpiresAt string `json:"approvalExpiresAt"`
		RetryAfterMs      int    `json:"retryAfterMs"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.Status != "pending" || body.RetryAfterMs != 2000 || body.ApprovalExpiresAt == "" {
		t.Fatalf("body = %+v, want pending retry response", body)
	}
}

func TestApprovalClaim_whenVerifierMatches_returnsPairCodeWithFreshTTL(t *testing.T) {
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	currentTime := now
	router, cookie := authenticatedRouterWithClock(t, db, func() time.Time { return currentTime })
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	csrf := approvalStatusForTest(t, router, cookie, approval.ApprovalID).CSRFToken
	approveApprovalForTest(t, router, cookie, approval, csrf)
	currentTime = now.Add(5 * time.Minute)

	recorder := claimApprovalForTest(t, router, approval)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Status            string `json:"status"`
		PairCode          string `json:"pairCode"`
		PairCodeExpiresAt string `json:"pairCodeExpiresAt"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.Status != "approved" || !strings.HasPrefix(body.PairCode, "pair_") {
		t.Fatalf("body = %+v, want approved pair code", body)
	}
	if body.PairCodeExpiresAt != currentTime.Add(10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("pairCodeExpiresAt = %q, want fresh pair-code TTL", body.PairCodeExpiresAt)
	}
}

func TestApprovalClaim_whenNonceMismatch_incrementsClaimFailureCount(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	approval.Nonce = approval.Nonce[:len(approval.Nonce)-1] + "A"

	recorder := claimApprovalForTest(t, router, approval)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT claim_failure_count FROM approval_records WHERE id = ?`, approval.ApprovalID).Scan(&count); err != nil {
		t.Fatalf("query claim failure count error = %v", err)
	}
	if count != 1 {
		t.Fatalf("claim_failure_count = %d, want 1", count)
	}
}

func TestApprovalClaim_whenSixthClaimAuthFailure_locksApproval(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	approval.Verifier = "malformed"

	var recorder *httptest.ResponseRecorder
	for range 6 {
		recorder = claimApprovalForTest(t, router, approval)
	}

	if recorder.Code != http.StatusLocked {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusLocked, recorder.Body.String())
	}
	var state string
	if err := db.QueryRowContext(context.Background(), `SELECT state FROM approval_records WHERE id = ?`, approval.ApprovalID).Scan(&state); err != nil {
		t.Fatalf("query approval state error = %v", err)
	}
	if state != "locked" {
		t.Fatalf("state = %q, want locked", state)
	}
}

func TestApprovalClaim_whenPendingPollRateLimitExceeded_returnsApprovalClaimRateLimited(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db, Clock: func() time.Time {
		return time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	}})
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	var recorder *httptest.ResponseRecorder

	for range 91 {
		recorder = claimApprovalForTest(t, router, approval)
	}

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_claim_rate_limited") {
		t.Fatalf("body = %s, want approval_claim_rate_limited", recorder.Body.String())
	}
}

func TestApprovalClaim_whenUnknownApprovalIDRateLimitExceeded_returnsApprovalClaimRateLimited(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db, Clock: func() time.Time {
		return time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	}})
	_, verifier, _ := approvalClientProofForTest()
	var recorder *httptest.ResponseRecorder

	for range 121 {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/pair/approval/claim",
			strings.NewReader(`{"approvalId":"approval_unknown","nonce":"nonce_unknown","verifier":"`+verifier+`"}`),
		)
		request.RemoteAddr = "203.0.113.77:4321"
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
	}

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_claim_rate_limited") {
		t.Fatalf("body = %s, want approval_claim_rate_limited", recorder.Body.String())
	}
	var approvalCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM approval_records`).Scan(&approvalCount); err != nil {
		t.Fatalf("query approval records error = %v", err)
	}
	if approvalCount != 0 {
		t.Fatalf("approval record count = %d, want 0", approvalCount)
	}
}

func TestApprovalClaim_whenMalformedRequestRateLimitExceeded_returnsApprovalClaimRateLimited(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db, Clock: func() time.Time {
		return time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	}})
	var recorder *httptest.ResponseRecorder

	for range 121 {
		request := httptest.NewRequest(http.MethodPost, "/api/pair/approval/claim", strings.NewReader(`{`))
		request.RemoteAddr = "203.0.113.88:4321"
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
	}

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_claim_rate_limited") {
		t.Fatalf("body = %s, want approval_claim_rate_limited", recorder.Body.String())
	}
}

func TestPairClaim_whenApprovalMetadataMismatch_rejectsBeforeMachineCredential(t *testing.T) {
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	approval := startApprovalForTest(t, router, approvalStartForTest{
		MachineName: "approved-macbook",
		OS:          "darwin",
		Arch:        "arm64",
		Version:     "0.1.0",
	})
	csrf := approvalStatusForTest(t, router, cookie, approval.ApprovalID).CSRFToken
	approveApprovalForTest(t, router, cookie, approval, csrf)
	claimRecorder := claimApprovalForTest(t, router, approval)
	var claimBody struct {
		PairCode string `json:"pairCode"`
	}
	decodeJSONResponse(t, claimRecorder, &claimBody)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/claim",
		strings.NewReader(`{"code":"`+claimBody.PairCode+`","machine":{"name":"different-macbook","os":"darwin","arch":"arm64","agentVersion":"0.1.0"}}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_machine_metadata_mismatch") {
		t.Fatalf("body = %s, want approval_machine_metadata_mismatch", recorder.Body.String())
	}
	var tokenCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM machine_tokens`).Scan(&tokenCount); err != nil {
		t.Fatalf("query machine tokens error = %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("machine token count = %d, want 0", tokenCount)
	}
}
