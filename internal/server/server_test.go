package server

import (
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
