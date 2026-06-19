package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLogin_whenConfigExists_failsBeforeApprovalStartAndBrowserOpen(t *testing.T) {
	configDir := t.TempDir()
	if err := writeConfig(filepath.Join(configDir, configFileName), Config{
		ServerURL:    "https://neul.example",
		MachineID:    "machine_existing",
		MachineToken: "mtn_existing",
	}); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	var approvalStarts atomic.Int64
	var browserOpens atomic.Int64
	restoreBrowser := overrideBrowserOpenForTest(t, func(string) error {
		browserOpens.Add(1)
		return nil
	})
	defer restoreBrowser()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pair/approval/start" {
			approvalStarts.Add(1)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", configDir}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want existing config failure")
	}
	if approvalStarts.Load() != 0 {
		t.Fatalf("approval starts = %d, want 0", approvalStarts.Load())
	}
	if browserOpens.Load() != 0 {
		t.Fatalf("browser opens = %d, want 0", browserOpens.Load())
	}
}

func TestLogin_whenBrowserOpenFails_continuesEnrollment(t *testing.T) {
	server := newLoginApprovalServer(t, loginApprovalServerOptions{
		claimStatus: "approved",
		pairCode:    "pair_from_approval",
	})
	defer server.Close()
	var browserOpens atomic.Int64
	restoreBrowser := overrideBrowserOpenForTest(t, func(string) error {
		browserOpens.Add(1)
		return errors.New("browser unavailable")
	})
	defer restoreBrowser()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", t.TempDir()}, &stdout, &stdout)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if browserOpens.Load() != 1 {
		t.Fatalf("browser opens = %d, want 1", browserOpens.Load())
	}
	if !strings.Contains(stdout.String(), "로그인이 완료되었습니다") {
		t.Fatalf("stdout = %q, want login success", stdout.String())
	}
}

func TestLogin_whenApprovalLocked_printsRecoverableFailure(t *testing.T) {
	server := newLoginApprovalServer(t, loginApprovalServerOptions{errorCode: "approval_locked", errorStatus: http.StatusLocked})
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", t.TempDir()}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want approval locked failure")
	}
	for _, want := range []string{"승인이 잠겼습니다", "neul login --server"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestLogin_whenServerPollingFails_printsRecoverableFailure(t *testing.T) {
	server := newLoginApprovalServer(t, loginApprovalServerOptions{errorCode: "approval_claim_failed", errorStatus: http.StatusInternalServerError})
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", t.TempDir()}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want server polling failure")
	}
	for _, want := range []string{"서버 승인 확인에 실패했습니다", "neul login --server"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func overrideBrowserOpenForTest(t *testing.T, open func(string) error) func() {
	t.Helper()
	previous := openApprovalURL
	openApprovalURL = open
	return func() {
		openApprovalURL = previous
	}
}
