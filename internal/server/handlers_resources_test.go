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

func TestResources_createPackageStoresBrewAsSupported(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/resources/package",
		strings.NewReader(`{"name":"kubectl","sourceKind":"brew","desiredVersion":"latest","targetSegment":"base"}`),
	)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"sourceKind":"brew"`) || !strings.Contains(body, `"agentSupport":"supported"`) {
		t.Fatalf("body = %s, want brew supported resource", body)
	}
}

func TestResources_storeAptAndMiseAsUnsupported(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	for _, sourceKind := range []string{"apt", "mise"} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/resources/package",
			strings.NewReader(`{"name":"tool-`+sourceKind+`","sourceKind":"`+sourceKind+`","desiredVersion":"latest","targetSegment":"base"}`),
		)
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("%s status = %d, want %d; body=%s", sourceKind, recorder.Code, http.StatusCreated, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"agentSupport":"unsupported"`) {
			t.Fatalf("%s body = %s, want unsupported", sourceKind, recorder.Body.String())
		}
	}
}

func TestResources_rejectHostileDotfilePaths(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)

	for _, path := range []string{"/etc/hosts", "~/.config/../.ssh/id_rsa"} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/resources/dotfile",
			strings.NewReader(`{"path":"`+path+`","content":"x","mode":"0644","applyMode":"copy","targetSegment":"base"}`),
		)
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d; body=%s", path, recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "path_not_allowed") {
			t.Fatalf("%s body = %s, want path_not_allowed", path, recorder.Body.String())
		}
	}
}

func TestResources_rejectDotfileSymlinkEscape(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(configDir, "escape")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	db := openServerTestDB(t)
	router, cookie := authenticatedRouterWithConfig(t, Config{DB: db, Clock: func() time.Time { return now }, HomeDir: homeDir})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/resources/dotfile",
		strings.NewReader(`{"path":"~/.config/escape/file","content":"x","mode":"0644","applyMode":"copy","targetSegment":"base"}`),
	)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "path_not_allowed") {
		t.Fatalf("body = %s, want path_not_allowed", recorder.Body.String())
	}
}

func TestResources_patchAndDeleteIncrementDesiredVersion(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	resourceID := createPackageResource(t, router, cookie)

	patch := httptest.NewRequest(
		http.MethodPatch,
		"/api/resources/"+resourceID,
		strings.NewReader(`{"desiredVersion":"1.2.3"}`),
	)
	patch.AddCookie(cookie)
	patchRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchRecorder, patch)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want %d; body=%s", patchRecorder.Code, http.StatusOK, patchRecorder.Body.String())
	}
	if !strings.Contains(patchRecorder.Body.String(), `"desiredVersion":2`) {
		t.Fatalf("patch body = %s, want desiredVersion 2", patchRecorder.Body.String())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/resources/"+resourceID, http.NoBody)
	deleteRequest.AddCookie(cookie)
	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body=%s", deleteRecorder.Code, http.StatusNoContent, deleteRecorder.Body.String())
	}
	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM resources WHERE id = ?`, resourceID).Scan(&count); err != nil {
		t.Fatalf("query count error = %v", err)
	}
	if count != 0 {
		t.Fatalf("resource count = %d, want deleted", count)
	}
}

func authenticatedRouterWithConfig(t *testing.T, config Config) (http.Handler, *http.Cookie) {
	t.Helper()
	var out strings.Builder
	boot, err := BootstrapOwner(context.Background(), config.DB, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	router := NewRouter(config)
	request := httptest.NewRequest(http.MethodPost, "/api/session/local", strings.NewReader(`{"setupToken":"`+boot.SetupToken+`"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("session status = %d, want %d; body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	return router, recorder.Result().Cookies()[0]
}

func createPackageResource(t *testing.T, router http.Handler, cookie *http.Cookie) string {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/resources/package",
		strings.NewReader(`{"name":"kubectl","sourceKind":"brew","desiredVersion":"latest","targetSegment":"base"}`),
	)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.ID == "" {
		t.Fatal("created resource id is empty")
	}
	return body.ID
}
