package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

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

func TestApprovalStart_whenPublicOriginConfigured_returnsPublicApprovalURL(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db, PublicOrigin: "https://neul.4gly.dev"})
	nonce, _, challenge := approvalClientProofForTest()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://neul-server.neul.svc.cluster.local/api/pair/approval/start",
		strings.NewReader(`{"nonce":"`+nonce+`","verifierChallenge":"`+challenge+`","machine":{"name":"joon-macbook","os":"darwin","arch":"arm64","agentVersion":"0.1.0"}}`),
	)
	request.Host = "neul-server.neul.svc.cluster.local"
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		ApprovalURL string `json:"approvalUrl"`
	}
	decodeJSONResponse(t, recorder, &body)
	approvalURL, err := url.Parse(body.ApprovalURL)
	if err != nil {
		t.Fatalf("Parse approvalUrl error = %v", err)
	}
	if approvalURL.Scheme != "https" || approvalURL.Host != "neul.4gly.dev" {
		t.Fatalf("approvalUrl = %q, want configured public origin", body.ApprovalURL)
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

func TestApprovalApprove_whenPublicOriginConfigured_approvesRequest(t *testing.T) {
	db := openServerTestDB(t)
	router, cookie := authenticatedRouterWithConfig(t, Config{
		DB:           db,
		Clock:        func() time.Time { return time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC) },
		PublicOrigin: "https://neul.4gly.dev",
	})
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	csrf := approvalStatusForTest(t, router, cookie, approval.ApprovalID).CSRFToken
	request := httptest.NewRequest(
		http.MethodPost,
		"http://neul-server.neul.svc.cluster.local/api/pair/approval/approve",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","csrfToken":"`+csrf+`","decision":"approve"}`),
	)
	request.Host = "neul-server.neul.svc.cluster.local"
	request.Header.Set("Origin", "https://neul.4gly.dev")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestApprovalApprove_whenOriginDoesNotMatchPublicOrigin_returnsApprovalOriginInvalid(t *testing.T) {
	db := openServerTestDB(t)
	router, cookie := authenticatedRouterWithConfig(t, Config{
		DB:           db,
		Clock:        func() time.Time { return time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC) },
		PublicOrigin: "https://neul.4gly.dev",
	})
	approval := startApprovalForTest(t, router, approvalStartForTest{})
	csrf := approvalStatusForTest(t, router, cookie, approval.ApprovalID).CSRFToken
	request := httptest.NewRequest(
		http.MethodPost,
		"http://neul-server.neul.svc.cluster.local/api/pair/approval/approve",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","csrfToken":"`+csrf+`","decision":"approve"}`),
	)
	request.Host = "neul-server.neul.svc.cluster.local"
	request.Header.Set("Origin", "https://evil.example")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "approval_origin_invalid") {
		t.Fatalf("body = %s, want approval_origin_invalid", recorder.Body.String())
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
