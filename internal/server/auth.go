package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const sessionCookieName = "neul_session"
const setupTokenOutputPrefix = "neul setup token: "

type BootstrapResult struct {
	OwnerID    string
	SetupToken string
}

func BootstrapOwner(ctx context.Context, db *sql.DB, out io.Writer) (BootstrapResult, error) {
	var ownerID string
	var setupTokenHash sql.NullString
	err := db.QueryRowContext(ctx, `SELECT id, setup_token_hash FROM owners ORDER BY created_at LIMIT 1`).Scan(&ownerID, &setupTokenHash)
	if err == nil {
		return BootstrapResult{OwnerID: ownerID}, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return BootstrapResult{}, fmt.Errorf("query owner: %w", err)
	}

	token, err := randomToken("setup")
	if err != nil {
		return BootstrapResult{}, err
	}
	ownerID = "owner_local"
	_, err = db.ExecContext(
		ctx,
		`INSERT INTO owners (id, setup_token_hash, created_at) VALUES (?, ?, ?)`,
		ownerID,
		hashSecret(token),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("insert owner: %w", err)
	}
	if err := createDefaultProfile(ctx, db, ownerID); err != nil {
		return BootstrapResult{}, err
	}
	if _, err := fmt.Fprint(out, formatSetupTokenOutput(token)); err != nil {
		return BootstrapResult{}, fmt.Errorf("print setup token: %w", err)
	}
	return BootstrapResult{OwnerID: ownerID, SetupToken: token}, nil
}

func formatSetupTokenOutput(token string) string {
	return setupTokenOutputPrefix + token + "\n"
}

func createDefaultProfile(ctx context.Context, db *sql.DB, ownerID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(
		ctx,
		`INSERT INTO profiles (id, owner_id, name, created_at) VALUES (?, ?, 'base', ?)`,
		"profile_base",
		ownerID,
		now,
	)
	if err != nil {
		return fmt.Errorf("insert default profile: %w", err)
	}
	_, err = db.ExecContext(
		ctx,
		`INSERT INTO segments (id, profile_id, name, priority, created_at) VALUES (?, 'profile_base', 'base', 0, ?)`,
		"segment_base",
		now,
	)
	if err != nil {
		return fmt.Errorf("insert default segment: %w", err)
	}
	return nil
}

func handleLocalSession(db *sql.DB) http.HandlerFunc {
	type requestBody struct {
		SetupToken string `json:"setupToken"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "Request body must be JSON.")
			return
		}
		if body.SetupToken == "" {
			writeJSONError(w, http.StatusBadRequest, "setup_token_required", "Setup token is required.")
			return
		}

		ctx := r.Context()
		var ownerID string
		var tokenHash sql.NullString
		err := db.QueryRowContext(ctx, `SELECT id, setup_token_hash FROM owners ORDER BY created_at LIMIT 1`).Scan(&ownerID, &tokenHash)
		if err != nil {
			writeJSONError(w, http.StatusConflict, "owner_not_bootstrapped", "Owner is not bootstrapped.")
			return
		}
		if !tokenHash.Valid || tokenHash.String == "" {
			writeJSONError(w, http.StatusConflict, "setup_token_used", "Setup token was already used.")
			return
		}
		if tokenHash.String != hashSecret(body.SetupToken) {
			writeJSONError(w, http.StatusUnauthorized, "setup_token_invalid", "Setup token is invalid.")
			return
		}

		sessionToken, err := randomToken("session")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "token_generation_failed", "Could not create session.")
			return
		}
		_, err = db.ExecContext(
			ctx,
			`INSERT INTO sessions (id, owner_id, session_hash, created_at) VALUES (?, ?, ?, ?)`,
			"session_"+hashSecret(sessionToken)[:16],
			ownerID,
			hashSecret(sessionToken),
			time.Now().UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "session_create_failed", "Could not create session.")
			return
		}
		if _, err := db.ExecContext(ctx, `UPDATE owners SET setup_token_hash = NULL WHERE id = ?`, ownerID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "setup_token_consume_failed", "Could not consume setup token.")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    sessionToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func requireOwnerSession(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Owner session is required.")
			return
		}
		var count int
		err = db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sessions WHERE session_hash = ?`, hashSecret(cookie.Value)).Scan(&count)
		if err != nil || count != 1 {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Owner session is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func randomToken(prefix string) (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
