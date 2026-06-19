package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var loginSleep = time.Sleep

type approvalStartResponse struct {
	ApprovalID     string `json:"approvalId"`
	ApprovalURL    string `json:"approvalUrl"`
	ComparisonCode string `json:"comparisonCode"`
	PollAfterMs    int    `json:"pollAfterMs"`
}

type approvalClaimResponse struct {
	Status       string `json:"status"`
	RetryAfterMs int    `json:"retryAfterMs"`
	PairCode     string `json:"pairCode"`
}

func runLogin(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	serverURL := flags.String("server", "", "server URL")
	configDir := flags.String("config-dir", defaultConfigDir(), "config directory")
	configPath := flags.String("config", "", "config path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	origin, err := normalizeServerOrigin(*serverURL)
	if err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		path = filepath.Join(*configDir, configFileName)
	}
	exists, err := configExists(path)
	if err != nil {
		return err
	}
	if exists {
		_, _ = fmt.Fprintf(stdout, "이미 Neul config가 있습니다: %s\n이 machine을 계속 연결하려면 실행: neul up\n", path)
		return errors.New("machine already configured")
	}

	proof, err := newApprovalProof()
	if err != nil {
		return err
	}
	machine := currentMachineMetadata()
	start, err := startApproval(origin, proof, machine)
	if err != nil {
		printRecoverableLoginFailure(stdout, origin)
		return err
	}
	if _, err := fmt.Fprintf(stdout, "브라우저에서 이 요청을 승인하세요: %s\n비교 코드: %s\n", start.ApprovalURL, start.ComparisonCode); err != nil {
		return err
	}
	_ = openApprovalURL(start.ApprovalURL)

	pairCode, err := pollApprovalClaim(origin, proof, start)
	if err != nil {
		printLoginFailure(stdout, origin, err)
		return err
	}
	claim, err := claimPairingCode(origin, pairCode)
	if err != nil {
		printRecoverableLoginFailure(stdout, origin)
		return err
	}
	config := Config{
		ServerURL:    origin,
		MachineID:    claim.MachineID,
		MachineToken: claim.MachineToken,
	}
	if err := writeConfig(path, config); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "로그인이 완료되었습니다.\n이 machine을 계속 연결하려면 실행: neul up")
	return err
}

type approvalProof struct {
	Nonce             string
	Verifier          string
	VerifierChallenge string
}

func newApprovalProof() (approvalProof, error) {
	nonce, err := randomBase64URL32()
	if err != nil {
		return approvalProof{}, err
	}
	verifier, err := randomBase64URL32()
	if err != nil {
		return approvalProof{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	return approvalProof{
		Nonce:             nonce,
		Verifier:          verifier,
		VerifierChallenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func randomBase64URL32() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate random proof: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func startApproval(serverURL string, proof approvalProof, machine machineMetadata) (approvalStartResponse, error) {
	requestBody := struct {
		Nonce             string          `json:"nonce"`
		VerifierChallenge string          `json:"verifierChallenge"`
		Machine           machineMetadata `json:"machine"`
	}{
		Nonce:             proof.Nonce,
		VerifierChallenge: proof.VerifierChallenge,
		Machine:           machine,
	}
	var response approvalStartResponse
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return approvalStartResponse{}, fmt.Errorf("encode approval start: %w", err)
	}
	err = decodeApprovalJSON(approvalJSONRequest{
		Endpoint:   serverURL + "/api/pair/approval/start",
		Encoded:    encoded,
		WantStatus: http.StatusCreated,
		Decode: func(body io.Reader) error {
			return json.NewDecoder(body).Decode(&response)
		},
	})
	if err != nil {
		return approvalStartResponse{}, err
	}
	if response.ApprovalID == "" || response.ApprovalURL == "" || response.ComparisonCode == "" {
		return approvalStartResponse{}, errors.New("approval start response was incomplete")
	}
	return response, nil
}

func pollApprovalClaim(serverURL string, proof approvalProof, start approvalStartResponse) (string, error) {
	retryAfter := 2 * time.Second
	if start.PollAfterMs > 0 {
		retryAfter = time.Duration(start.PollAfterMs) * time.Millisecond
	}
	for {
		requestBody := struct {
			ApprovalID string `json:"approvalId"`
			Nonce      string `json:"nonce"`
			Verifier   string `json:"verifier"`
		}{
			ApprovalID: start.ApprovalID,
			Nonce:      proof.Nonce,
			Verifier:   proof.Verifier,
		}
		var response approvalClaimResponse
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return "", fmt.Errorf("encode approval claim: %w", err)
		}
		err = decodeApprovalJSON(approvalJSONRequest{
			Endpoint:   serverURL + "/api/pair/approval/claim",
			Encoded:    encoded,
			WantStatus: http.StatusOK,
			Decode: func(body io.Reader) error {
				return json.NewDecoder(body).Decode(&response)
			},
		})
		if err != nil {
			return "", err
		}
		switch response.Status {
		case "pending":
			if response.RetryAfterMs > 0 {
				retryAfter = time.Duration(response.RetryAfterMs) * time.Millisecond
			}
			loginSleep(retryAfter)
		case "approved":
			if response.PairCode == "" {
				return "", errors.New("approval response did not include pair code")
			}
			return response.PairCode, nil
		case "claimed":
			return "", errors.New("approval already claimed")
		default:
			return "", fmt.Errorf("approval claim returned %q", response.Status)
		}
	}
}

type approvalJSONRequest struct {
	Endpoint   string
	Encoded    []byte
	WantStatus int
	Decode     func(io.Reader) error
}

func decodeApprovalJSON(request approvalJSONRequest) error {
	response, err := http.Post(request.Endpoint, "application/json", bytes.NewReader(request.Encoded))
	if err != nil {
		return fmt.Errorf("post %s: %w", request.Endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != request.WantStatus {
		return decodeAPIError(response.Body, response.StatusCode)
	}
	if err := request.Decode(response.Body); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func printLoginFailure(stdout io.Writer, serverURL string, err error) {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "approval_expired":
			_, _ = fmt.Fprintf(stdout, "승인 시간이 만료되었습니다. 다시 실행: neul login --server %s\n", serverURL)
		case "approval_cancelled":
			_, _ = fmt.Fprintf(stdout, "승인이 취소되었습니다. 다시 실행: neul login --server %s\n", serverURL)
		case "approval_locked":
			_, _ = fmt.Fprintf(stdout, "승인이 잠겼습니다. 다시 실행: neul login --server %s\n", serverURL)
		case "approval_pair_code_issued":
			_, _ = fmt.Fprintf(stdout, "pair code가 이미 발급되었습니다. 다시 실행: neul login --server %s\n", serverURL)
		default:
			_, _ = fmt.Fprintf(stdout, "서버 승인 확인에 실패했습니다. 다시 실행: neul login --server %s\n", serverURL)
		}
		return
	}
	_, _ = fmt.Fprintf(stdout, "서버 승인 확인에 실패했습니다. 다시 실행: neul login --server %s\n", serverURL)
}

func printRecoverableLoginFailure(stdout io.Writer, serverURL string) {
	_, _ = fmt.Fprintf(stdout, "로그인을 완료하지 못했습니다. 다시 실행: neul login --server %s\n", serverURL)
}

func normalizeServerOrigin(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("login requires --server")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("login requires --server <origin>")
	}
	return strings.TrimRight(raw, "/"), nil
}
