package server

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateSessionAndConsumeSetup_whenSetupHashChanges_returnsChanged(t *testing.T) {
	db := openServerTestDB(t)
	var out bytes.Buffer
	boot, err := BootstrapOwner(context.Background(), db, &out)
	if err != nil {
		t.Fatalf("BootstrapOwner() error = %v", err)
	}
	oldHash := hashSecret(boot.SetupToken)
	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE owners SET setup_token_hash = ? WHERE id = ?`,
		hashSecret("setup_replacement"),
		boot.OwnerID,
	); err != nil {
		t.Fatalf("replace setup token hash error = %v", err)
	}

	_, err = createSessionAndConsumeSetup(
		context.Background(),
		db,
		boot.OwnerID,
		oldHash,
		time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	)
	if !errors.Is(err, errSetupTokenChanged) {
		t.Fatalf("createSessionAndConsumeSetup() error = %v, want errSetupTokenChanged", err)
	}
}
