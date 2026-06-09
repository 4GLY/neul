package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionLocal_whenSetupTimestampIsMalformed_returnsExpired(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE owners SET created_at = ? WHERE id = ?`, "not-a-timestamp", boot.OwnerID); err != nil {
		t.Fatalf("corrupt setup timestamp error = %v", err)
	}
	var rotated bytes.Buffer
	router := NewRouter(Config{DB: db, SetupTokenWriter: &rotated})

	request := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusGone, recorder.Body.String())
	}
	if code := jsonErrorCode(t, recorder); code != "setup_token_expired" {
		t.Fatalf("error code = %q, want setup_token_expired", code)
	}
}

func TestSessionLocal_whenSetupTokenIsValid_setsCookieAndConsumesToken(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	var rotated bytes.Buffer
	router := NewRouter(Config{DB: db, SetupTokenWriter: &rotated})

	request := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	cookie := recorder.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Fatal("cookie HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Secure {
		t.Fatal("cookie Secure = true for HTTP request, want false")
	}

	reuse := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	reuseRecorder := httptest.NewRecorder()
	router.ServeHTTP(reuseRecorder, reuse)
	if reuseRecorder.Code != http.StatusConflict {
		t.Fatalf("reuse status = %d, want %d", reuseRecorder.Code, http.StatusConflict)
	}
	if code := jsonErrorCode(t, reuseRecorder); code != "setup_token_used" {
		t.Fatalf("reuse error code = %q, want setup_token_used", code)
	}
}

func TestSessionLocal_whenRequestIsTLS_setsSecureCookie(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodPost, "https://neul.local/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	cookie := recorder.Result().Cookies()[0]
	if !cookie.Secure {
		t.Fatal("cookie Secure = false for HTTPS request, want true")
	}
}

func TestSessionLocal_whenSetupTokenIsInvalid_returnsUnauthorizedCode(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	if _, err := BootstrapOwner(context.Background(), db, &out); err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"setup_wrong"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if code := jsonErrorCode(t, recorder); code != "setup_token_invalid" {
		t.Fatalf("error code = %q, want setup_token_invalid", code)
	}
}

func TestSessionLocal_whenSetupTokenIsExpired_returnsGoneCode(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE owners SET created_at = ? WHERE id = ?`, "2000-01-01T00:00:00Z", boot.OwnerID); err != nil {
		t.Fatalf("expire setup token error = %v", err)
	}
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusGone, recorder.Body.String())
	}
	if code := jsonErrorCode(t, recorder); code != "setup_token_expired" {
		t.Fatalf("error code = %q, want setup_token_expired", code)
	}
}

func TestSessionLocal_whenSetupTokenIsAlreadyConsumedInTransaction_returnsUsed(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	setupHash := hashSecret(boot.SetupToken)
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	if _, err := createSessionAndConsumeSetup(context.Background(), db, boot.OwnerID, setupHash, now); err != nil {
		t.Fatalf("first createSessionAndConsumeSetup() error = %v", err)
	}
	if _, err := createSessionAndConsumeSetup(context.Background(), db, boot.OwnerID, setupHash, now); !errors.Is(err, errSetupTokenAlreadyUsed) {
		t.Fatalf("second createSessionAndConsumeSetup() error = %v, want errSetupTokenAlreadyUsed", err)
	}

	var sessionCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sessions WHERE owner_id = ?`, boot.OwnerID).Scan(&sessionCount); err != nil {
		t.Fatalf("query sessions error = %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("session count = %d, want 1", sessionCount)
	}
}

func TestSessionLocal_whenSetupTokenIsExpired_rotatesAndPrintsReplacement(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE owners SET created_at = ? WHERE id = ?`, "2000-01-01T00:00:00Z", boot.OwnerID); err != nil {
		t.Fatalf("expire setup token error = %v", err)
	}
	var rotated bytes.Buffer
	router := NewRouter(Config{DB: db, SetupTokenWriter: &rotated})

	expiredRequest := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	expiredRecorder := httptest.NewRecorder()
	router.ServeHTTP(expiredRecorder, expiredRequest)

	if expiredRecorder.Code != http.StatusGone {
		t.Fatalf("expired status = %d, want %d", expiredRecorder.Code, http.StatusGone)
	}
	replacement := setupTokenFromOutput(t, rotated.String())
	reusedExpired := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	reusedExpiredRecorder := httptest.NewRecorder()
	router.ServeHTTP(reusedExpiredRecorder, reusedExpired)
	if reusedExpiredRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want %d", reusedExpiredRecorder.Code, http.StatusUnauthorized)
	}

	replacementRequest := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+replacement+`"}`))
	replacementRecorder := httptest.NewRecorder()
	router.ServeHTTP(replacementRecorder, replacementRequest)
	if replacementRecorder.Code != http.StatusNoContent {
		t.Fatalf("replacement status = %d, want %d; body=%s", replacementRecorder.Code, http.StatusNoContent, replacementRecorder.Body.String())
	}
}

func jsonErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(recorder.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON error body: %v", err)
	}
	return body.Error.Code
}

func setupTokenFromOutput(t *testing.T, output string) string {
	t.Helper()
	const prefix = "neul setup token: "
	start := strings.Index(output, prefix)
	if start < 0 {
		t.Fatalf("setup token output missing: %q", output)
	}
	token := strings.TrimSpace(output[start+len(prefix):])
	if !strings.HasPrefix(token, "setup_") {
		t.Fatalf("setup token output = %q, want setup_ prefix", token)
	}
	return token
}
