package server

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestRotateSetupToken_whenPrintFails_doesNotPersistReplacement(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	var oldHash string
	if err := db.QueryRowContext(context.Background(), `SELECT setup_token_hash FROM owners WHERE id = ?`, boot.OwnerID).Scan(&oldHash); err != nil {
		t.Fatalf("query setup hash error = %v", err)
	}

	_, err = rotateSetupToken(context.Background(), db, boot.OwnerID, time.Now().UTC(), failingWriter{})
	if err == nil {
		t.Fatal("rotateSetupToken() error = nil, want print error")
	}
	var currentHash string
	if err := db.QueryRowContext(context.Background(), `SELECT setup_token_hash FROM owners WHERE id = ?`, boot.OwnerID).Scan(&currentHash); err != nil {
		t.Fatalf("query current setup hash error = %v", err)
	}
	if currentHash != oldHash {
		t.Fatalf("setup hash changed after print failure")
	}
}

func TestRotateSetupToken_whenPersistFails_doesNotPrintReplacement(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	var rotated bytes.Buffer

	_, err = rotateSetupToken(context.Background(), db, boot.OwnerID, time.Now().UTC(), &rotated)
	if err == nil {
		t.Fatal("rotateSetupToken() error = nil, want persist error")
	}
	if rotated.Len() != 0 {
		t.Fatalf("printed replacement after persist failure: %q", rotated.String())
	}
}

func TestRotateSetupToken_whenSetupTokenIsAlreadyConsumed_doesNotPrintReplacement(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE owners SET setup_token_hash = NULL WHERE id = ?`,
		boot.OwnerID,
	); err != nil {
		t.Fatalf("consume setup token hash error = %v", err)
	}
	var rotated bytes.Buffer

	_, err = rotateSetupToken(context.Background(), db, boot.OwnerID, time.Now().UTC(), &rotated)
	if !errors.Is(err, errSetupTokenAlreadyUsed) {
		t.Fatalf("rotateSetupToken() error = %v, want errSetupTokenAlreadyUsed", err)
	}
	if rotated.Len() != 0 {
		t.Fatalf("printed replacement after consumed setup token: %q", rotated.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}
