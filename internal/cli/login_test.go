package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLogin_whenConfigExists_failsBeforeApprovalStart(t *testing.T) {
	configDir := t.TempDir()
	if err := writeConfig(filepath.Join(configDir, configFileName), Config{
		ServerURL:    "https://neul.example",
		MachineID:    "machine_existing",
		MachineToken: "mtn_existing",
	}); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	var approvalStarts atomic.Int64
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
	for _, want := range []string{"이미 Neul config가 있습니다", "neul up"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestLogin_whenApprovalApproved_claimsPairCodeAndWritesConfig0600(t *testing.T) {
	configDir := t.TempDir()
	server := newLoginApprovalServer(t, loginApprovalServerOptions{
		claimStatus: "approved",
		pairCode:    "pair_from_approval",
	})
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", configDir}, &stdout, &stdout)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if server.approvalStarts.Load() != 1 {
		t.Fatalf("approval starts = %d, want 1", server.approvalStarts.Load())
	}
	if server.pairClaims.Load() != 1 {
		t.Fatalf("pair claims = %d, want 1", server.pairClaims.Load())
	}
	if strings.Contains(stdout.String(), "pair_from_approval") || strings.Contains(stdout.String(), "mtn_secret") {
		t.Fatalf("stdout leaked credential: %q", stdout.String())
	}
	for _, want := range []string{"로그인이 완료되었습니다", "neul up"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	configPath := filepath.Join(configDir, configFileName)
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config stat error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 0600", info.Mode().Perm())
	}
	var config Config
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if config.ServerURL != server.URL || config.MachineID != "machine_login" || config.MachineToken != "mtn_secret" {
		t.Fatalf("config = %+v, want login credentials", config)
	}
}

func TestLogin_whenApprovalExpired_printsRecoverableFailure(t *testing.T) {
	server := newLoginApprovalServer(t, loginApprovalServerOptions{errorCode: "approval_expired", errorStatus: http.StatusGone})
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", t.TempDir()}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want approval expired failure")
	}
	for _, want := range []string{"승인 시간이 만료되었습니다", "neul login --server"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestLogin_whenApprovalCancelled_printsRecoverableFailure(t *testing.T) {
	server := newLoginApprovalServer(t, loginApprovalServerOptions{errorCode: "approval_cancelled", errorStatus: http.StatusConflict})
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", t.TempDir()}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want approval cancelled failure")
	}
	for _, want := range []string{"승인이 취소되었습니다", "neul login --server"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestLogin_whenPairCodeAlreadyIssued_printsRestartGuidance(t *testing.T) {
	server := newLoginApprovalServer(t, loginApprovalServerOptions{errorCode: "approval_pair_code_issued", errorStatus: http.StatusConflict})
	defer server.Close()
	var stdout strings.Builder

	err := Run([]string{"login", "--server", server.URL, "--config-dir", t.TempDir()}, &stdout, &stdout)

	if err == nil {
		t.Fatal("Run() error = nil, want pair-code issued failure")
	}
	for _, want := range []string{"pair code가 이미 발급되었습니다", "neul login --server"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

type loginApprovalServerOptions struct {
	claimStatus        string
	pairCode           string
	errorCode          string
	errorStatus        int
	startErrorCode     string
	startErrorStatus   int
	pairClaimErrorCode string
	pairClaimStatus    int
}

type loginApprovalServer struct {
	*httptest.Server
	approvalStarts atomic.Int64
	pairClaims     atomic.Int64
}

func newLoginApprovalServer(t *testing.T, options loginApprovalServerOptions) *loginApprovalServer {
	t.Helper()
	restoreBrowser := overrideBrowserOpenForTest(t, func(string) error {
		return nil
	})
	t.Cleanup(restoreBrowser)
	loginServer := &loginApprovalServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/pair/approval/start":
			loginServer.approvalStarts.Add(1)
			if options.startErrorCode != "" {
				w.WriteHeader(options.startErrorStatus)
				_, _ = fmt.Fprintf(w, `{"error":{"code":"%s","message":"terminal"}}`, options.startErrorCode)
				return
			}
			var body struct {
				Nonce             string `json:"nonce"`
				VerifierChallenge string `json:"verifierChallenge"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("approval start decode error = %v", err)
			}
			if body.Nonce == "" || body.VerifierChallenge == "" {
				t.Fatalf("approval start body = %+v, want nonce and verifierChallenge", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"approvalId":"approval_1","approvalUrl":"%s/enroll/approve?approval=approval_1&nonce=nonce","comparisonCode":"742-918","expiresAt":"2026-06-19T08:10:00Z","pollAfterMs":1}`, loginServer.URL)
		case "/api/pair/approval/claim":
			writeApprovalClaimForTest(w, options)
		case "/api/pair/claim":
			loginServer.pairClaims.Add(1)
			if options.pairClaimErrorCode != "" {
				w.WriteHeader(options.pairClaimStatus)
				_, _ = fmt.Fprintf(w, `{"error":{"code":"%s","message":"terminal"}}`, options.pairClaimErrorCode)
				return
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("pair claim decode error = %v", err)
			}
			if body.Code != options.pairCode && options.pairCode != "" {
				t.Fatalf("pair claim code = %q, want %q", body.Code, options.pairCode)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"machineId":"machine_login","machineToken":"mtn_secret"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	loginServer.Server = server
	return loginServer
}

func writeApprovalClaimForTest(w http.ResponseWriter, options loginApprovalServerOptions) {
	if options.errorCode != "" {
		w.WriteHeader(options.errorStatus)
		_, _ = fmt.Fprintf(w, `{"error":{"code":"%s","message":"terminal"}}`, options.errorCode)
		return
	}
	if options.claimStatus == "pending" {
		_, _ = w.Write([]byte(`{"status":"pending","retryAfterMs":1}`))
		return
	}
	pairCode := options.pairCode
	if pairCode == "" {
		pairCode = "pair_from_approval"
	}
	_, _ = fmt.Fprintf(w, `{"status":"approved","pairCode":"%s","pairCodeExpiresAt":"2026-06-19T08:15:00Z"}`, pairCode)
}
