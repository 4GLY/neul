package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
