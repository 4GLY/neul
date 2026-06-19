package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPairInit_whenOwnerAuthenticated_createsOpaqueCodeWithTenMinuteExpiry(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	request := httptest.NewRequest(http.MethodPost, "/api/pair/init", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expiresAt"`
	}
	decodeJSONResponse(t, recorder, &body)
	if !strings.HasPrefix(body.Code, "pair_") {
		t.Fatalf("code = %q, want opaque pair_ token", body.Code)
	}
	if body.ExpiresAt != now.Add(10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expiresAt = %q, want exactly +10 minutes", body.ExpiresAt)
	}

	var storedHash string
	var plaintextCount int
	if err := db.QueryRowContext(context.Background(), `SELECT code_hash FROM pairing_codes`).Scan(&storedHash); err != nil {
		t.Fatalf("query code_hash error = %v", err)
	}
	if storedHash == "" || storedHash == body.Code {
		t.Fatalf("stored code hash = %q, want non-empty non-plaintext hash", storedHash)
	}
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pairing_codes WHERE code_hash = ?`, body.Code).Scan(&plaintextCount); err != nil {
		t.Fatalf("query plaintext count error = %v", err)
	}
	if plaintextCount != 0 {
		t.Fatalf("plaintext code was stored")
	}
}

func TestPairClaim_whenCodeIsValid_createsMachineAndReturnsTokenOnce(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	code := initPairCode(t, router, cookie)

	claimBody := `{"code":"` + code + `","machine":{"name":"nima-studio","os":"darwin","arch":"arm64","agentVersion":"0.1.0"}}`
	request := httptest.NewRequest(http.MethodPost, "/api/pair/claim", strings.NewReader(claimBody))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		MachineID    string `json:"machineId"`
		MachineToken string `json:"machineToken"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.MachineID == "" || body.MachineToken == "" {
		t.Fatalf("claim response = %+v, want machine id and token", body)
	}

	var tokenHash string
	if err := db.QueryRowContext(context.Background(), `SELECT token_hash FROM machine_tokens WHERE machine_id = ?`, body.MachineID).Scan(&tokenHash); err != nil {
		t.Fatalf("query token hash error = %v", err)
	}
	if tokenHash == "" || tokenHash == body.MachineToken {
		t.Fatalf("stored token hash = %q, want non-plaintext hash", tokenHash)
	}

	reuse := httptest.NewRequest(http.MethodPost, "/api/pair/claim", strings.NewReader(claimBody))
	reuseRecorder := httptest.NewRecorder()
	router.ServeHTTP(reuseRecorder, reuse)
	if reuseRecorder.Code != http.StatusConflict {
		t.Fatalf("reuse status = %d, want %d; body=%s", reuseRecorder.Code, http.StatusConflict, reuseRecorder.Body.String())
	}
	if !strings.Contains(reuseRecorder.Body.String(), "code_used") {
		t.Fatalf("reuse body = %s, want code_used", reuseRecorder.Body.String())
	}
}

func TestPairClaim_whenCodeExpired_returnsGoneWithErrorCode(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	currentTime := now
	router, cookie := authenticatedRouterWithClock(t, db, func() time.Time {
		return currentTime
	})
	code := initPairCode(t, router, cookie)
	currentTime = now.Add(10*time.Minute + time.Nanosecond)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/claim",
		strings.NewReader(`{"code":"`+code+`","machine":{"name":"late","os":"darwin","arch":"arm64"}}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusGone, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pairing_code_expired"`) {
		t.Fatalf("body = %s, want pairing_code_expired", recorder.Body.String())
	}
}

func TestPairPoll_whenCodeClaimed_returnsClaimedMachine(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	code := initPairCode(t, router, cookie)
	claim := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/claim",
		strings.NewReader(`{"code":"`+code+`","machine":{"name":"nima-studio","os":"darwin","arch":"arm64"}}`),
	)
	claimRecorder := httptest.NewRecorder()
	router.ServeHTTP(claimRecorder, claim)
	if claimRecorder.Code != http.StatusCreated {
		t.Fatalf("claim status = %d, want %d; body=%s", claimRecorder.Code, http.StatusCreated, claimRecorder.Body.String())
	}

	poll := httptest.NewRequest(http.MethodGet, "/api/pair/poll?code="+code, http.NoBody)
	poll.AddCookie(cookie)
	pollRecorder := httptest.NewRecorder()
	router.ServeHTTP(pollRecorder, poll)

	if pollRecorder.Code != http.StatusOK {
		t.Fatalf("poll status = %d, want %d; body=%s", pollRecorder.Code, http.StatusOK, pollRecorder.Body.String())
	}
	var body struct {
		Status    string `json:"status"`
		MachineID string `json:"machineId"`
	}
	decodeJSONResponse(t, pollRecorder, &body)
	if body.Status != "claimed" || body.MachineID == "" {
		t.Fatalf("poll body = %+v, want claimed machine", body)
	}
}

func TestOnboardingPairPollUnauthenticated(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	code := initPairCode(t, router, cookie)

	poll := httptest.NewRequest(http.MethodGet, "/api/pair/poll?code="+code, http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, poll)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
}

func TestOnboardingPairPollExpiredUnusedCode(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	code := initPairCode(t, router, cookie)
	expiredAt := now.Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(context.Background(), `UPDATE pairing_codes SET expires_at = ? WHERE code_hash = ?`, expiredAt, hashSecret(code)); err != nil {
		t.Fatalf("expire pair code error = %v", err)
	}

	poll := httptest.NewRequest(http.MethodGet, "/api/pair/poll?code="+code, http.NoBody)
	poll.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, poll)

	var body struct {
		Status    string `json:"status"`
		ExpiresAt string `json:"expiresAt"`
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	decodeJSONResponse(t, recorder, &body)
	if body.Status != "expired" || body.ExpiresAt != expiredAt {
		t.Fatalf("body = %+v, want expired with expiresAt", body)
	}
}

func TestOnboardingPairPollClaimed(t *testing.T) {
	TestPairPoll_whenCodeClaimed_returnsClaimedMachine(t)
}

func TestOnboardingPairInitResponseShape(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	request := httptest.NewRequest(http.MethodPost, "/api/pair/init", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var body struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expiresAt"`
	}
	decodeJSONResponse(t, recorder, &body)
	if recorder.Code != http.StatusCreated || !strings.HasPrefix(body.Code, "pair_") || body.ExpiresAt != now.Add(pairingTTL).Format(time.RFC3339Nano) {
		t.Fatalf("init = status %d body %+v, want code and expiresAt", recorder.Code, body)
	}
}

func TestApprovalStart_whenValid_returnsApprovalURLAndComparisonCode(t *testing.T) {
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db, Clock: func() time.Time { return now }})
	nonce, verifier, challenge := approvalClientProofForTest()

	request := httptest.NewRequest(
		http.MethodPost,
		"https://neul.local/api/pair/approval/start",
		strings.NewReader(`{"nonce":"`+nonce+`","verifierChallenge":"`+challenge+`","machine":{"name":"joon-macbook","os":"darwin","arch":"arm64","agentVersion":"0.1.0"}}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		ApprovalID     string `json:"approvalId"`
		ApprovalURL    string `json:"approvalUrl"`
		ComparisonCode string `json:"comparisonCode"`
		ExpiresAt      string `json:"expiresAt"`
		PollAfterMs    int    `json:"pollAfterMs"`
	}
	decodeJSONResponse(t, recorder, &body)
	if !strings.HasPrefix(body.ApprovalID, "approval_") {
		t.Fatalf("approvalId = %q, want approval_ prefix", body.ApprovalID)
	}
	if body.ExpiresAt != now.Add(10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expiresAt = %q, want approval TTL", body.ExpiresAt)
	}
	if body.PollAfterMs != 2000 {
		t.Fatalf("pollAfterMs = %d, want 2000", body.PollAfterMs)
	}
	if len(body.ComparisonCode) != len("123-456") || body.ComparisonCode[3] != '-' {
		t.Fatalf("comparisonCode = %q, want nnn-nnn", body.ComparisonCode)
	}
	approvalURL, err := url.Parse(body.ApprovalURL)
	if err != nil {
		t.Fatalf("Parse approvalUrl error = %v", err)
	}
	if approvalURL.Scheme != "https" || approvalURL.Host != "neul.local" || approvalURL.Path != "/enroll/approve" {
		t.Fatalf("approvalUrl = %q, want owner approval route on request origin", body.ApprovalURL)
	}
	query := approvalURL.Query()
	if query.Get("approval") != body.ApprovalID || query.Get("nonce") != nonce {
		t.Fatalf("approvalUrl query = %v, want approval id and nonce", query)
	}
	for _, forbidden := range []string{"pair_", "mtn_", "setup_", verifier, challenge} {
		if strings.Contains(body.ApprovalURL, forbidden) {
			t.Fatalf("approvalUrl leaked %q: %s", forbidden, body.ApprovalURL)
		}
	}
}

func TestApprovalApprove_whenMissingOwnerSession_returnsOwnerSessionRequired(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodPost, "/api/pair/approval/approve", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "owner_session_required") {
		t.Fatalf("body = %s, want owner_session_required", recorder.Body.String())
	}
}

func TestApprovalApprove_whenCSRFInvalid_returnsApprovalCSRFInvalid(t *testing.T) {
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	approval := startApprovalForTest(t, router, approvalStartForTest{})

	request := httptest.NewRequest(
		http.MethodPost,
		"https://neul.local/api/pair/approval/approve",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","csrfToken":"wrong","decision":"approve"}`),
	)
	request.Header.Set("Origin", "https://neul.local")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_csrf_invalid") {
		t.Fatalf("body = %s, want approval_csrf_invalid", recorder.Body.String())
	}
}

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

func TestApprovalStart_whenIPRateLimitExceeded_returnsApprovalStartRateLimited(t *testing.T) {
	db := openServerTestDB(t)
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	router := NewRouter(Config{DB: db, Clock: func() time.Time { return now }})
	var recorder *httptest.ResponseRecorder

	for range 11 {
		nonce, _, challenge := approvalClientProofForTest()
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/pair/approval/start",
			strings.NewReader(`{"nonce":"`+nonce+`","verifierChallenge":"`+challenge+`","machine":{"name":"rate","os":"darwin","arch":"arm64","agentVersion":"0.1.0"}}`),
		)
		request.RemoteAddr = "203.0.113.7:4321"
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
	}

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_start_rate_limited") {
		t.Fatalf("body = %s, want approval_start_rate_limited", recorder.Body.String())
	}
}

func TestApprovalApprove_whenOwnerSessionRateLimitExceeded_returnsApprovalApproveRateLimited(t *testing.T) {
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC))
	var recorder *httptest.ResponseRecorder

	for range 21 {
		request := httptest.NewRequest(http.MethodPost, "https://neul.local/api/pair/approval/approve", strings.NewReader(`{}`))
		request.Header.Set("Origin", "https://neul.local")
		request.AddCookie(cookie)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
	}

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_approve_rate_limited") {
		t.Fatalf("body = %s, want approval_approve_rate_limited", recorder.Body.String())
	}
}

func TestApprovalStatus_whenOwnerSessionRateLimitExceeded_returnsApprovalStatusRateLimited(t *testing.T) {
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC))
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	var recorder *httptest.ResponseRecorder

	for range 121 {
		request := httptest.NewRequest(http.MethodGet, "/api/pair/approval/status?approvalId="+approval.ApprovalID, http.NoBody)
		request.AddCookie(cookie)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
	}

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_status_rate_limited") {
		t.Fatalf("body = %s, want approval_status_rate_limited", recorder.Body.String())
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

func TestApprovalStatus_whenLocked_returnsTerminalLockedState(t *testing.T) {
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC))
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	if _, err := db.ExecContext(context.Background(), `UPDATE approval_records SET state = 'locked' WHERE id = ?`, approval.ApprovalID); err != nil {
		t.Fatalf("lock approval error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/pair/approval/status?approvalId="+approval.ApprovalID, http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Status string `json:"status"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.Status != "locked" {
		t.Fatalf("status = %q, want locked", body.Status)
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

func authenticatedRouter(t *testing.T, db *sql.DB, now time.Time) (http.Handler, *http.Cookie) {
	t.Helper()
	return authenticatedRouterWithClock(t, db, func() time.Time { return now })
}

func authenticatedRouterWithClock(t *testing.T, db *sql.DB, clock func() time.Time) (http.Handler, *http.Cookie) {
	t.Helper()
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	router := NewRouter(Config{DB: db, Clock: clock})
	request := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("session status = %d, want %d; body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	return router, cookies[0]
}

func initPairCode(t *testing.T, router http.Handler, cookie *http.Cookie) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/pair/init", http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("init status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.Code == "" {
		t.Fatal("pair code is empty")
	}
	return body.Code
}

func decodeJSONResponse(t *testing.T, recorder *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	if err := json.NewDecoder(recorder.Result().Body).Decode(dst); err != nil {
		t.Fatalf("Decode() error = %v; body=%s", err, recorder.Body.String())
	}
}

type approvalStartForTest struct {
	MachineName string
	OS          string
	Arch        string
	Version     string
}

type approvalForTest struct {
	ApprovalID string
	Nonce      string
	Verifier   string
}

func startApprovalForTest(t *testing.T, router http.Handler, options approvalStartForTest) approvalForTest {
	t.Helper()
	if options.MachineName == "" {
		options.MachineName = "joon-macbook"
	}
	if options.OS == "" {
		options.OS = "darwin"
	}
	if options.Arch == "" {
		options.Arch = "arm64"
	}
	if options.Version == "" {
		options.Version = "0.1.0"
	}
	nonce, verifier, challenge := approvalClientProofForTest()
	request := httptest.NewRequest(
		http.MethodPost,
		"https://neul.local/api/pair/approval/start",
		strings.NewReader(`{"nonce":"`+nonce+`","verifierChallenge":"`+challenge+`","machine":{"name":"`+options.MachineName+`","os":"`+options.OS+`","arch":"`+options.Arch+`","agentVersion":"`+options.Version+`"}}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("approval start status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		ApprovalID string `json:"approvalId"`
	}
	decodeJSONResponse(t, recorder, &body)
	return approvalForTest{ApprovalID: body.ApprovalID, Nonce: nonce, Verifier: verifier}
}

func approvalClientProofForTest() (string, string, string) {
	nonce := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	verifier := base64.RawURLEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789"))
	sum := sha256.Sum256([]byte(verifier))
	return nonce, verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func claimApprovalForTest(t *testing.T, router http.Handler, approval approvalForTest) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/approval/claim",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","verifier":"`+approval.Verifier+`"}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type approvalStatusForTestResponse struct {
	CSRFToken string `json:"csrfToken"`
}

func approvalStatusForTest(t *testing.T, router http.Handler, cookie *http.Cookie, approvalID string) approvalStatusForTestResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/pair/approval/status?approvalId="+approvalID, http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("approval status status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body approvalStatusForTestResponse
	decodeJSONResponse(t, recorder, &body)
	return body
}

func approveApprovalForTest(t *testing.T, router http.Handler, cookie *http.Cookie, approval approvalForTest, csrf string) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"https://neul.local/api/pair/approval/approve",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","csrfToken":"`+csrf+`","decision":"approve"}`),
	)
	request.Header.Set("Origin", "https://neul.local")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("approval approve status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}
