package server

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestBootstrap_whenDatabaseIsEmpty_printsSetupTokenOnceAndStoresOnlyHash(t *testing.T) {
	db := openServerTestDB(t)
	var first bytes.Buffer

	result, err := BootstrapOwner(context.Background(), db, &first)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if result.SetupToken == "" {
		t.Fatal("SetupToken is empty")
	}
	if !strings.Contains(first.String(), result.SetupToken) {
		t.Fatalf("stdout does not contain setup token")
	}

	var storedHash string
	if err := db.QueryRowContext(context.Background(), `SELECT setup_token_hash FROM owners WHERE id = ?`, result.OwnerID).Scan(&storedHash); err != nil {
		t.Fatalf("query setup hash error = %v", err)
	}
	if storedHash == "" || storedHash == result.SetupToken {
		t.Fatalf("stored setup hash = %q, want non-empty non-plaintext hash", storedHash)
	}

	var second bytes.Buffer
	secondResult, err := BootstrapOwner(context.Background(), db, &second)
	if err != nil {
		t.Fatalf("BootstrapOwner() second error = %v", err)
	}
	if secondResult.SetupToken != "" {
		t.Fatalf("second SetupToken = %q, want empty", secondResult.SetupToken)
	}
	if second.Len() != 0 {
		t.Fatalf("second stdout = %q, want empty", second.String())
	}
}

func TestBootstrap_whenExistingSetupTokenIsExpired_rotatesAndPrintsReplacement(t *testing.T) {
	db := openServerTestDB(t)
	var first bytes.Buffer
	firstResult, err := BootstrapOwnerWithClock(context.Background(), db, &first, func() time.Time {
		return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("BootstrapOwner() first error = %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE owners SET created_at = ? WHERE id = ?`,
		"2000-01-01T00:00:00Z",
		firstResult.OwnerID,
	); err != nil {
		t.Fatalf("expire setup token error = %v", err)
	}

	var second bytes.Buffer
	secondResult, err := BootstrapOwnerWithClock(context.Background(), db, &second, func() time.Time {
		return time.Date(2026, 6, 9, 12, 11, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("BootstrapOwner() second error = %v", err)
	}
	if secondResult.SetupToken == "" {
		t.Fatal("replacement SetupToken is empty")
	}
	if secondResult.SetupToken == firstResult.SetupToken {
		t.Fatal("replacement SetupToken reused the expired token")
	}
	if !strings.Contains(second.String(), secondResult.SetupToken) {
		t.Fatalf("stdout does not contain replacement setup token")
	}
}

func TestBootstrapWithClock_whenConfiguredSetupTokenTTLExpires_rotatesAndPrintsReplacement(t *testing.T) {
	db := openServerTestDB(t)
	var first bytes.Buffer
	firstResult, err := BootstrapOwnerWithClock(context.Background(), db, &first, func() time.Time {
		return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	}, time.Millisecond)
	if err != nil {
		t.Fatalf("BootstrapOwnerWithClock() first error = %v", err)
	}

	var second bytes.Buffer
	secondResult, err := BootstrapOwnerWithClock(context.Background(), db, &second, func() time.Time {
		return time.Date(2026, 6, 9, 12, 0, 0, int(2*time.Millisecond), time.UTC)
	}, time.Millisecond)
	if err != nil {
		t.Fatalf("BootstrapOwnerWithClock() second error = %v", err)
	}
	if secondResult.SetupToken == "" {
		t.Fatal("replacement SetupToken is empty")
	}
	if secondResult.SetupToken == firstResult.SetupToken {
		t.Fatal("replacement SetupToken reused the expired token")
	}
	if !strings.Contains(second.String(), secondResult.SetupToken) {
		t.Fatalf("stdout does not contain replacement setup token")
	}
}
