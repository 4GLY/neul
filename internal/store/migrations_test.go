package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrationsCreateMvpTables(t *testing.T) {
	db := openTestDB(t)

	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	expectedTables := []string{
		"owners",
		"sessions",
		"pairing_codes",
		"machine_tokens",
		"machines",
		"profiles",
		"segments",
		"resources",
		"file_versions",
		"reconcile_runs",
		"reconcile_events",
		"agent_reports",
		"agent_commands",
	}
	for _, table := range expectedTables {
		if !tableExists(t, db, table) {
			t.Fatalf("table %q does not exist", table)
		}
	}
}

func TestMigrationsDoNotCreateSecretTables(t *testing.T) {
	db := openTestDB(t)

	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	if tableExists(t, db, "secrets") {
		t.Fatal("table secrets exists, want absent")
	}
}

func TestForeignKeysRejectOrphanMachineToken(t *testing.T) {
	db := openTestDB(t)

	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO machine_tokens (id, machine_id, token_hash, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		"token_1",
		"missing_machine",
		"hash",
	)
	if err == nil {
		t.Fatal("insert orphan machine token succeeded, want foreign key error")
	}
}

func TestIdempotencyPreventsDuplicateReconcileRuns(t *testing.T) {
	db := openTestDB(t)

	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	seedMachine(t, db)
	for i := 0; i < 2; i++ {
		_, err := db.ExecContext(
			context.Background(),
			`INSERT OR IGNORE INTO reconcile_runs (id, machine_id, reason, idempotency_key, status, created_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			"run_1",
			"machine_1",
			"repair_drift",
			"repair-key",
			"queued",
		)
		if err != nil {
			t.Fatalf("insert reconcile run %d error = %v", i, err)
		}
	}

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM reconcile_runs`).Scan(&count); err != nil {
		t.Fatalf("count reconcile_runs error = %v", err)
	}
	if count != 1 {
		t.Fatalf("reconcile run count = %d, want 1", count)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return db
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		table,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master error = %v", err)
	}
	return count == 1
}

func seedMachine(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO machines (id, name, os, arch, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"machine_1",
		"work-macbook",
		"macOS",
		"arm64",
	)
	if err != nil {
		t.Fatalf("seed machine error = %v", err)
	}
}
