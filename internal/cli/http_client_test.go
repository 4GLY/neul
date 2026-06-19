package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoginApprovalHTTP_whenClientOverridden_usesPackageClient(t *testing.T) {
	var requestedPath string
	restoreClient := overrideHTTPClientForTest(t, &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestedPath = request.URL.Path
			return jsonHTTPResponse(http.StatusCreated, `{"approvalId":"approval_1","approvalUrl":"https://neul.example/approve","comparisonCode":"123456"}`), nil
		}),
	})
	defer restoreClient()

	_, err := startApproval("https://neul.example", approvalProof{
		Nonce:             "nonce",
		VerifierChallenge: "challenge",
	}, machineMetadata{Name: "test", OS: "darwin", Arch: "arm64"})

	if err != nil {
		t.Fatalf("startApproval() error = %v", err)
	}
	if requestedPath != "/api/pair/approval/start" {
		t.Fatalf("request path = %q, want approval start path", requestedPath)
	}
}

func TestLoginPairClaimHTTP_whenClientOverridden_usesPackageClient(t *testing.T) {
	var requestedPath string
	restoreClient := overrideHTTPClientForTest(t, &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestedPath = request.URL.Path
			return jsonHTTPResponse(http.StatusCreated, `{"machineId":"machine_1","machineToken":"mtn_secret"}`), nil
		}),
	})
	defer restoreClient()

	_, err := claimPairingCode("https://neul.example", "pair_abc")

	if err != nil {
		t.Fatalf("claimPairingCode() error = %v", err)
	}
	if requestedPath != "/api/pair/claim" {
		t.Fatalf("request path = %q, want pair claim path", requestedPath)
	}
}

func TestLoginCLIHTTPClient_hasFiniteTimeout(t *testing.T) {
	if cliHTTPClient.Timeout <= 0 {
		t.Fatalf("cliHTTPClient.Timeout = %v, want finite timeout", cliHTTPClient.Timeout)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func overrideHTTPClientForTest(t *testing.T, client *http.Client) func() {
	t.Helper()
	previous := cliHTTPClient
	cliHTTPClient = client
	return func() {
		cliHTTPClient = previous
	}
}

func jsonHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
