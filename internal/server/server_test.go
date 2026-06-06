package server

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4gly/neul/internal/store"
)

func TestBootstrap_whenDatabaseIsEmpty_printsSetupTokenOnceAndStoresOnlyHash(t *testing.T) {
	db := openServerTestDB(t)
	var first bytes.Buffer

	result, err := BootstrapOwner(context.Background(), db, &first)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if result.SetupToken == "" {
		t.Fatal("SetupToken is empty")
	}
	if !strings.Contains(first.String(), result.SetupToken) {
		t.Fatalf("stdout does not contain setup token")
	}

	var storedHash string
	if err := db.QueryRowContext(context.Background(), `SELECT setup_token_hash FROM owners WHERE id = ?`, result.OwnerID).Scan(&storedHash); err != nil {
		t.Fatalf("query setup hash error = %v", err)
	}
	if storedHash == "" || storedHash == result.SetupToken {
		t.Fatalf("stored setup hash = %q, want non-empty non-plaintext hash", storedHash)
	}

	var second bytes.Buffer
	secondResult, err := BootstrapOwner(context.Background(), db, &second)
	if err != nil {
		t.Fatalf("BootstrapOwner() second error = %v", err)
	}
	if secondResult.SetupToken != "" {
		t.Fatalf("second SetupToken = %q, want empty", secondResult.SetupToken)
	}
	if second.Len() != 0 {
		t.Fatalf("second stdout = %q, want empty", second.String())
	}
}

func TestSessionLocal_whenSetupTokenIsValid_setsCookieAndConsumesToken(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	router := NewRouter(Config{DB: db})

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

	reuse := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	reuseRecorder := httptest.NewRecorder()
	router.ServeHTTP(reuseRecorder, reuse)
	if reuseRecorder.Code != http.StatusConflict {
		t.Fatalf("reuse status = %d, want %d", reuseRecorder.Code, http.StatusConflict)
	}
}

func TestAuth_whenMutationHasNoCookie_returnsUnauthorized(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodPost, "/api/pair/init", http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestHealthz_returnsOk(t *testing.T) {
	db := openServerTestDB(t)
	router := NewRouter(Config{DB: db})

	request := httptest.NewRequest(http.MethodGet, "/api/healthz", http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if strings.TrimSpace(recorder.Body.String()) != `{"ok":true}` {
		t.Fatalf("body = %q, want health JSON", recorder.Body.String())
	}
}

func TestStaticSpa_whenNonApiPath_returnsIndexHtml(t *testing.T) {
	db := openServerTestDB(t)
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<main>neul</main>"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	router := NewRouter(Config{DB: db, StaticDir: staticDir})

	request := httptest.NewRequest(http.MethodGet, "/machines/work", http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "neul") {
		t.Fatalf("body = %q, want SPA index", recorder.Body.String())
	}
}

func TestStaticSpa_whenAssetPathExists_servesAssetFile(t *testing.T) {
	db := openServerTestDB(t)
	staticDir := t.TempDir()
	assetDir := filepath.Join(staticDir, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<main>neul</main>"), 0o644); err != nil {
		t.Fatalf("WriteFile(index) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "index.js"), []byte("console.log('neul')"), 0o644); err != nil {
		t.Fatalf("WriteFile(asset) error = %v", err)
	}
	router := NewRouter(Config{DB: db, StaticDir: staticDir})

	request := httptest.NewRequest(http.MethodGet, "/assets/index.js", http.NoBody)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if strings.Contains(recorder.Body.String(), "<main>neul</main>") {
		t.Fatalf("asset request returned SPA index: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "console.log") {
		t.Fatalf("body = %s, want asset content", recorder.Body.String())
	}
}

func openServerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "neul-test.sqlite")
	db, err := store.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	if err := store.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return db
}
