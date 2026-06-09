package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"
)

const setupTokenTTL = 10 * time.Minute

var errSetupTokenAlreadyUsed = errors.New("setup token already used")
var errSetupTokenChanged = errors.New("setup token changed")

func effectiveSetupTokenTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return setupTokenTTL
	}
	return ttl
}

func rotateSetupToken(ctx context.Context, db *sql.DB, ownerID string, now time.Time, out io.Writer) (string, error) {
	var oldHash sql.NullString
	var oldCreatedAt string
	if err := db.QueryRowContext(ctx, `SELECT setup_token_hash, created_at FROM owners WHERE id = ?`, ownerID).Scan(&oldHash, &oldCreatedAt); err != nil {
		return "", fmt.Errorf("query setup token before rotation: %w", err)
	}
	if !oldHash.Valid || oldHash.String == "" {
		return "", errSetupTokenAlreadyUsed
	}
	token, err := randomToken("setup")
	if err != nil {
		return "", err
	}
	newHash := hashSecret(token)
	result, err := db.ExecContext(
		ctx,
		`UPDATE owners SET setup_token_hash = ?, created_at = ? WHERE id = ? AND setup_token_hash = ?`,
		newHash,
		now.UTC().Format(time.RFC3339Nano),
		ownerID,
		oldHash.String,
	)
	if err != nil {
		return "", fmt.Errorf("rotate expired setup token: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("check setup token rotation: %w", err)
	}
	if rowsAffected != 1 {
		var currentHash sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT setup_token_hash FROM owners WHERE id = ?`, ownerID).Scan(&currentHash); err != nil {
			return "", fmt.Errorf("query setup token after failed rotation: %w", err)
		}
		if currentHash.Valid && currentHash.String != "" {
			return "", errSetupTokenChanged
		}
		return "", errSetupTokenAlreadyUsed
	}
	if _, err := fmt.Fprintf(out, "neul setup token: %s\n", token); err != nil {
		if restoreErr := restoreSetupToken(ctx, db, ownerID, oldHash.String, oldCreatedAt, newHash); restoreErr != nil {
			return "", errors.Join(fmt.Errorf("print setup token: %w", err), restoreErr)
		}
		return "", fmt.Errorf("print setup token: %w", err)
	}
	return token, nil
}

func restoreSetupToken(ctx context.Context, db *sql.DB, ownerID string, oldHash string, oldCreatedAt string, newHash string) error {
	result, err := db.ExecContext(
		ctx,
		`UPDATE owners SET setup_token_hash = ?, created_at = ? WHERE id = ? AND setup_token_hash = ?`,
		oldHash,
		oldCreatedAt,
		ownerID,
		newHash,
	)
	if err != nil {
		return fmt.Errorf("restore setup token after print failure: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check setup token restore: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("restore setup token after print failure: current token changed")
	}
	return nil
}

func createSessionAndConsumeSetup(ctx context.Context, db *sql.DB, ownerID string, setupTokenHash string, now time.Time) (string, error) {
	sessionToken, err := randomToken("session")
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin session transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `UPDATE owners SET setup_token_hash = NULL WHERE id = ? AND setup_token_hash = ?`, ownerID, setupTokenHash)
	if err != nil {
		return "", fmt.Errorf("consume setup token: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("check setup token consumption: %w", err)
	}
	if rowsAffected != 1 {
		var currentHash sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT setup_token_hash FROM owners WHERE id = ?`, ownerID).Scan(&currentHash); err != nil {
			return "", fmt.Errorf("query setup token after failed consumption: %w", err)
		}
		if currentHash.Valid && currentHash.String != "" {
			return "", errSetupTokenChanged
		}
		return "", errSetupTokenAlreadyUsed
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO sessions (id, owner_id, session_hash, created_at) VALUES (?, ?, ?, ?)`,
		"session_"+hashSecret(sessionToken)[:16],
		ownerID,
		hashSecret(sessionToken),
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit session transaction: %w", err)
	}
	return sessionToken, nil
}

func setupTokenExpired(createdAt string, now time.Time, ttl time.Duration) bool {
	createdTime, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return true
	}
	return now.Sub(createdTime) > ttl
}
