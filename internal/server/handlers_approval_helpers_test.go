package server

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type approvalStartForTest struct {
	MachineName string
	OS          string
	Arch        string
	Version     string
}

type approvalForTest struct {
	ApprovalID string
	Nonce      string
	Verifier   string
}

func startApprovalForTest(t *testing.T, router http.Handler, options approvalStartForTest) approvalForTest {
	t.Helper()
	if options.MachineName == "" {
		options.MachineName = "joon-macbook"
	}
	if options.OS == "" {
		options.OS = "darwin"
	}
	if options.Arch == "" {
		options.Arch = "arm64"
	}
	if options.Version == "" {
		options.Version = "0.1.0"
	}
	nonce, verifier, challenge := approvalClientProofForTest()
	request := httptest.NewRequest(
		http.MethodPost,
		"https://neul.local/api/pair/approval/start",
		strings.NewReader(`{"nonce":"`+nonce+`","verifierChallenge":"`+challenge+`","machine":{"name":"`+options.MachineName+`","os":"`+options.OS+`","arch":"`+options.Arch+`","agentVersion":"`+options.Version+`"}}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("approval start status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var body struct {
		ApprovalID string `json:"approvalId"`
	}
	decodeJSONResponse(t, recorder, &body)
	return approvalForTest{ApprovalID: body.ApprovalID, Nonce: nonce, Verifier: verifier}
}

func approvalClientProofForTest() (string, string, string) {
	nonce := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	verifier := base64.RawURLEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789"))
	sum := sha256.Sum256([]byte(verifier))
	return nonce, verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func claimApprovalForTest(t *testing.T, router http.Handler, approval approvalForTest) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/pair/approval/claim",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","verifier":"`+approval.Verifier+`"}`),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type approvalStatusForTestResponse struct {
	CSRFToken string `json:"csrfToken"`
}

func approvalStatusForTest(t *testing.T, router http.Handler, cookie *http.Cookie, approvalID string) approvalStatusForTestResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/pair/approval/status?approvalId="+approvalID, http.NoBody)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("approval status status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body approvalStatusForTestResponse
	decodeJSONResponse(t, recorder, &body)
	return body
}

func approveApprovalForTest(t *testing.T, router http.Handler, cookie *http.Cookie, approval approvalForTest, csrf string) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"https://neul.local/api/pair/approval/approve",
		strings.NewReader(`{"approvalId":"`+approval.ApprovalID+`","nonce":"`+approval.Nonce+`","csrfToken":"`+csrf+`","decision":"approve"}`),
	)
	request.Header.Set("Origin", "https://neul.local")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("approval approve status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}
