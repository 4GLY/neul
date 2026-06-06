package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecretsDisabled_returnsNotFound(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodGet, "/api/secrets", http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "secret schema") {
		t.Fatalf("body leaked secret schema: %s", recorder.Body.String())
	}
}

func TestSecurity_authRequiredForOwnerMutations(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})

	for _, path := range []string{
		"/api/pair/init",
		"/api/resources/package",
		"/api/resources/dotfile",
		"/api/machines/machine_1/repair-drift",
	} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestSecurity_machineTokenAndPairingCodeAreHashOnly(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	code := initPairCode(t, router, cookie)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/claim",
		strings.NewReader(`{"code":"`+code+`","machine":{"name":"secure","os":"darwin","arch":"arm64"}}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("claim status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	var plaintextCodeCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pairing_codes WHERE code_hash = ?`, code).Scan(&plaintextCodeCount); err != nil {
		t.Fatalf("query code count error = %v", err)
	}
	if plaintextCodeCount != 0 {
		t.Fatal("plaintext pairing code was stored")
	}
	var plaintextTokenCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM machine_tokens WHERE token_hash LIKE 'mtn_%'`).Scan(&plaintextTokenCount); err != nil {
		t.Fatalf("query token count error = %v", err)
	}
	if plaintextTokenCount != 0 {
		t.Fatal("plaintext machine token was stored")
	}
}

func TestScopeNoWebSocketRoute(t *testing.T) {
	db := openServerTestDB(t)
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<main>neul</main>"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	router := NewRouter(Config{DB: db, StaticDir: staticDir})

	request := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}
