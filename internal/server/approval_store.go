package server

import (
	"context"
	"database/sql"
	"fmt"
)

const approvalRecordInsertSQL = `INSERT INTO approval_records (id, nonce_hash, verifier_challenge, csrf_token, comparison_code, state, machine_name, machine_os, machine_arch, machine_agent_version, approval_pairing_id, created_at, expires_at, approved_at, cancelled_at, pair_code_issued_at, claimed_at, claimed_machine_id, claimed_retain_until, claim_failure_count, last_failure_at, last_failure_ip) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const approvalRecordSelectSQL = `SELECT id, nonce_hash, verifier_challenge, csrf_token, comparison_code, state, machine_name, machine_os, machine_arch, machine_agent_version, approval_pairing_id, created_at, expires_at, approved_at, cancelled_at, pair_code_issued_at, claimed_at, claimed_machine_id, claimed_retain_until, claim_failure_count, last_failure_at, last_failure_ip FROM approval_records`

type approvalRecord struct {
	ID, NonceHash, VerifierChallenge, CSRFToken string
	ComparisonCode, State                       string
	Machine                                     pairClaimMachine
	CreatedAt, ExpiresAt                        string
	ApprovedAt, CancelledAt                     sql.NullString
	PairCodeIssuedAt, ApprovalPairingID         sql.NullString
	ClaimedAt, ClaimedMachineID                 sql.NullString
	ClaimedRetainUntil                          sql.NullString
	ClaimFailureCount                           int
	LastFailureAt, LastFailureIP                sql.NullString
}

type approvalPairCodeIssuedUpdate struct {
	ApprovalID string
	PairingID  string
	IssuedAt   string
}

type approvalClaimedUpdate struct {
	ApprovalID  string
	MachineID   string
	RetainUntil string
	ClaimedAt   string
}

type approvalClaimFailureUpdate struct {
	ApprovalID string
	FailedAt   string
	FailureIP  string
}

func insertApprovalRecord(ctx context.Context, db *sql.DB, record approvalRecord) error {
	_, err := db.ExecContext(
		ctx,
		approvalRecordInsertSQL,
		record.ID,
		record.NonceHash,
		record.VerifierChallenge,
		record.CSRFToken,
		record.ComparisonCode,
		record.State,
		record.Machine.Name,
		record.Machine.OS,
		record.Machine.Arch,
		record.Machine.AgentVersion,
		record.ApprovalPairingID,
		record.CreatedAt,
		record.ExpiresAt,
		record.ApprovedAt,
		record.CancelledAt,
		record.PairCodeIssuedAt,
		record.ClaimedAt,
		record.ClaimedMachineID,
		record.ClaimedRetainUntil,
		record.ClaimFailureCount,
		record.LastFailureAt,
		record.LastFailureIP,
	)
	if err != nil {
		return fmt.Errorf("insert approval record: %w", err)
	}
	return nil
}

func loadApprovalRecord(ctx context.Context, db *sql.DB, approvalID string) (approvalRecord, error) {
	return scanApprovalRecord(db.QueryRowContext(ctx, approvalRecordSelectSQL+` WHERE id = ?`, approvalID))
}

func getApprovalRecordForUpdate(ctx context.Context, tx *sql.Tx, approvalID string) (approvalRecord, error) {
	if err := lockApprovalRecord(ctx, tx, approvalID); err != nil {
		return approvalRecord{}, err
	}
	return scanApprovalRecord(tx.QueryRowContext(ctx, approvalRecordSelectSQL+` WHERE id = ?`, approvalID))
}

func markApprovalApproved(ctx context.Context, tx *sql.Tx, approvalID string, approvedAt string) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE approval_records SET state = ?, approved_at = ? WHERE id = ?`,
		"approved",
		approvedAt,
		approvalID,
	)
	if err != nil {
		return fmt.Errorf("mark approval approved: %w", err)
	}
	return nil
}

func markApprovalCancelled(ctx context.Context, tx *sql.Tx, approvalID string, cancelledAt string) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE approval_records SET state = ?, cancelled_at = ? WHERE id = ?`,
		"cancelled",
		cancelledAt,
		approvalID,
	)
	if err != nil {
		return fmt.Errorf("mark approval cancelled: %w", err)
	}
	return nil
}

func markApprovalPairCodeIssued(ctx context.Context, tx *sql.Tx, update approvalPairCodeIssuedUpdate) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE approval_records SET state = ?, approval_pairing_id = ?, pair_code_issued_at = ? WHERE id = ?`,
		"pair_code_issued",
		update.PairingID,
		update.IssuedAt,
		update.ApprovalID,
	)
	if err != nil {
		return fmt.Errorf("mark approval pair code issued: %w", err)
	}
	return nil
}

func markApprovalClaimed(ctx context.Context, tx *sql.Tx, update approvalClaimedUpdate) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE approval_records SET state = ?, claimed_at = ?, claimed_machine_id = ?, claimed_retain_until = ? WHERE id = ?`,
		"claimed",
		update.ClaimedAt,
		update.MachineID,
		update.RetainUntil,
		update.ApprovalID,
	)
	if err != nil {
		return fmt.Errorf("mark approval claimed: %w", err)
	}
	return nil
}

func incrementApprovalClaimFailure(ctx context.Context, tx *sql.Tx, update approvalClaimFailureUpdate) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE approval_records
		 SET claim_failure_count = claim_failure_count + 1, last_failure_at = ?, last_failure_ip = ?
		 WHERE id = ?`,
		update.FailedAt,
		update.FailureIP,
		update.ApprovalID,
	)
	if err != nil {
		return fmt.Errorf("increment approval claim failure: %w", err)
	}
	return nil
}

func lockApprovalRecord(ctx context.Context, tx *sql.Tx, approvalID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE approval_records SET id = id WHERE id = ?`, approvalID)
	if err != nil {
		return fmt.Errorf("lock approval record: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read locked approval rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanApprovalRecord(row *sql.Row) (approvalRecord, error) {
	var record approvalRecord
	err := row.Scan(
		&record.ID,
		&record.NonceHash,
		&record.VerifierChallenge,
		&record.CSRFToken,
		&record.ComparisonCode,
		&record.State,
		&record.Machine.Name,
		&record.Machine.OS,
		&record.Machine.Arch,
		&record.Machine.AgentVersion,
		&record.ApprovalPairingID,
		&record.CreatedAt,
		&record.ExpiresAt,
		&record.ApprovedAt,
		&record.CancelledAt,
		&record.PairCodeIssuedAt,
		&record.ClaimedAt,
		&record.ClaimedMachineID,
		&record.ClaimedRetainUntil,
		&record.ClaimFailureCount,
		&record.LastFailureAt,
		&record.LastFailureIP,
	)
	if err != nil {
		return approvalRecord{}, err
	}
	return record, nil
}
