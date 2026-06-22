package server

import (
	"context"
	"testing"
)

func TestLockApprovalRecord_whenCalled_persistsLockedState(t *testing.T) {
	db := openServerTestDB(t)
	record := approvalRecord{
		ID:                "approval_lock_test",
		NonceHash:         "nonce_hash",
		VerifierChallenge: "verifier_challenge",
		CSRFToken:         "csrf_token",
		ComparisonCode:    "123-456",
		State:             "pending",
		Machine: pairClaimMachine{
			Name:         "work-macbook",
			OS:           "darwin",
			Arch:         "arm64",
			AgentVersion: "0.1.0",
		},
		CreatedAt: "2026-06-19T08:00:00Z",
		ExpiresAt: "2026-06-19T08:10:00Z",
	}
	if err := insertApprovalRecord(context.Background(), db, record); err != nil {
		t.Fatalf("insertApprovalRecord() error = %v", err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := lockApprovalRecord(context.Background(), tx, record.ID); err != nil {
		t.Fatalf("lockApprovalRecord() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	got, err := loadApprovalRecord(context.Background(), db, record.ID)
	if err != nil {
		t.Fatalf("loadApprovalRecord() error = %v", err)
	}
	if got.State != "locked" {
		t.Fatalf("state = %q, want locked", got.State)
	}
}
