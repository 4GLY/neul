package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

func approvalForPairingClaim(r *http.Request, tx *sql.Tx, pairingID string) (approvalRecord, bool, error) {
	record, err := getApprovalRecordByPairingIDForUpdate(r.Context(), tx, pairingID)
	if err == sql.ErrNoRows {
		return approvalRecord{}, false, nil
	}
	if err != nil {
		return approvalRecord{}, false, err
	}
	return record, true, nil
}

func sameMachinePreview(expected pairClaimMachine, got pairClaimMachine) bool {
	return expected.Name == got.Name &&
		expected.OS == got.OS &&
		expected.Arch == got.Arch &&
		expected.AgentVersion == got.AgentVersion
}

func validBase64URLBytes(value string, size int) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == size
}

func randomComparisonCode() (string, error) {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("random comparison code: %w", err)
	}
	n := binary.BigEndian.Uint32(bytes[:]) % 1_000_000
	return fmt.Sprintf("%03d-%03d", n/1000, n%1000), nil
}

func requestOrigin(r *http.Request) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return scheme + "://" + host
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func approvalOwnerSessionKey(w http.ResponseWriter, r *http.Request, db *sql.DB) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSONError(w, http.StatusUnauthorized, "owner_session_required", "Owner session is required.")
		return "", false
	}
	sessionHash := hashSecret(cookie.Value)
	var count int
	err = db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sessions WHERE session_hash = ?`, sessionHash).Scan(&count)
	if err != nil || count != 1 {
		writeJSONError(w, http.StatusUnauthorized, "owner_session_required", "Owner session is required.")
		return "", false
	}
	return sessionHash, true
}

func sameOriginRequest(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == requestOrigin(r)
	}
	referer := r.Header.Get("Referer")
	if referer == "" {
		return false
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		return false
	}
	return parsed.Scheme+"://"+parsed.Host == requestOrigin(r)
}

func approvalRecordExpired(record approvalRecord, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	return err == nil && now.After(expiresAt)
}

func approvalClaimProofValid(record approvalRecord, nonce string, verifier string) bool {
	if record.NonceHash != hashSecret(nonce) {
		return false
	}
	decodedVerifier, err := base64.RawURLEncoding.DecodeString(verifier)
	if err != nil || len(decodedVerifier) < 32 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(challenge), []byte(record.VerifierChallenge)) == 1
}

func createApprovalPairingCode(r *http.Request, tx *sql.Tx, now time.Time) (string, string, string, error) {
	code, err := randomToken("pair")
	if err != nil {
		return "", "", "", fmt.Errorf("generate pair code: %w", err)
	}
	pairingID := "pairing_" + hashSecret(code)[:16]
	expiresAt := now.Add(pairingTTL).Format(time.RFC3339Nano)
	_, err = tx.ExecContext(
		r.Context(),
		`INSERT INTO pairing_codes (id, code_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		pairingID,
		hashSecret(code),
		expiresAt,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", "", "", fmt.Errorf("insert approval pairing code: %w", err)
	}
	return code, pairingID, expiresAt, nil
}
