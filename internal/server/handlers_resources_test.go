package server

import (
	"context"
	"encoding/json"
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

func TestResources_patchRejectsHostileDotfilePath(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	homeDir := t.TempDir()
	db := openServerTestDB(t)
	router, cookie := authenticatedRouterWithConfig(t, Config{DB: db, Clock: func() time.Time { return now }, HomeDir: homeDir})
	resourceID := createDotfileResource(t, router, cookie)

	patch := httptest.NewRequest(
		http.MethodPatch,
		"/api/resources/"+resourceID,
		strings.NewReader(`{"path":"/etc/passwd"}`),
	)
	patch.AddCookie(cookie)
	patchRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchRecorder, patch)
	if patchRecorder.Code != http.StatusBadRequest {
		t.Fatalf("patch status = %d, want %d; body=%s", patchRecorder.Code, http.StatusBadRequest, patchRecorder.Body.String())
	}
	resource, err := queryResourceByID(patch, db, resourceID)
	if err != nil {
		t.Fatalf("queryResourceByID() error = %v", err)
	}
	if resource.Spec["path"] != "~/.zshrc" || resource.DesiredVersion != 1 {
		t.Fatalf("resource after rejected patch = %+v, want original path/version", resource)
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

func TestResources_patchPackagePreservesBrewDesiredStateForAgent(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_desired", "mtn_desired")
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

	desired := httptest.NewRequest(http.MethodGet, "/api/agent/desired-state", http.NoBody)
	desired.Header.Set("Authorization", "Bearer mtn_desired")
	desiredRecorder := httptest.NewRecorder()
	router.ServeHTTP(desiredRecorder, desired)
	if desiredRecorder.Code != http.StatusOK {
		t.Fatalf("desired status = %d, want %d; body=%s", desiredRecorder.Code, http.StatusOK, desiredRecorder.Body.String())
	}
	var body struct {
		Resources []resourceResponse `json:"resources"`
	}
	if err := json.Unmarshal(desiredRecorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v; body=%s", err, desiredRecorder.Body.String())
	}
	if len(body.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1; body=%s", len(body.Resources), desiredRecorder.Body.String())
	}
	resource := body.Resources[0]
	if resource.ID != resourceID || resource.Name != "kubectl" || resource.DesiredVersion != 2 {
		t.Fatalf("resource = %+v, want patched kubectl desired version 2", resource)
	}
	if resource.Spec["sourceKind"] != "brew" || resource.Spec["desiredVersion"] != "1.2.3" || resource.Spec["name"] != "kubectl" {
		t.Fatalf("spec = %+v, want preserved brew desired state", resource.Spec)
	}
}

func TestResources_deletePackageRemovesFromAgentDesiredState(t *testing.T) {
	now := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	db := openServerTestDB(t)
	router, cookie := authenticatedRouter(t, db, now)
	seedMachineWithToken(t, db, "machine_delete_desired", "mtn_delete_desired")
	resourceID := createPackageResource(t, router, cookie)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/resources/"+resourceID, http.NoBody)
	deleteRequest.AddCookie(cookie)
	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body=%s", deleteRecorder.Code, http.StatusNoContent, deleteRecorder.Body.String())
	}

	desired := httptest.NewRequest(http.MethodGet, "/api/agent/desired-state", http.NoBody)
	desired.Header.Set("Authorization", "Bearer mtn_delete_desired")
	desiredRecorder := httptest.NewRecorder()
	router.ServeHTTP(desiredRecorder, desired)
	if desiredRecorder.Code != http.StatusOK {
		t.Fatalf("desired status = %d, want %d; body=%s", desiredRecorder.Code, http.StatusOK, desiredRecorder.Body.String())
	}
	var body struct {
		Resources []resourceResponse `json:"resources"`
	}
	if err := json.Unmarshal(desiredRecorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v; body=%s", err, desiredRecorder.Body.String())
	}
	if len(body.Resources) != 0 {
		t.Fatalf("resources = %+v, want deleted package omitted", body.Resources)
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

func createDotfileResource(t *testing.T, router http.Handler, cookie *http.Cookie) string {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/resources/dotfile",
		strings.NewReader(`{"path":"~/.zshrc","content":"export NEUL=1","mode":"0644","applyMode":"copy","targetSegment":"base"}`),
	)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create dotfile status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	decodeJSONResponse(t, recorder, &body)
	if body.ID == "" {
		t.Fatal("created dotfile resource id is empty")
	}
	return body.ID
}
