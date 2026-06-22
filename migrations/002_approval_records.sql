CREATE TABLE IF NOT EXISTS approval_records (
	id TEXT PRIMARY KEY,
	nonce_hash TEXT NOT NULL,
	verifier_challenge TEXT NOT NULL,
	csrf_token TEXT NOT NULL,
	comparison_code TEXT NOT NULL,
	state TEXT NOT NULL,
	machine_name TEXT NOT NULL,
	machine_os TEXT NOT NULL,
	machine_arch TEXT NOT NULL,
	machine_agent_version TEXT NOT NULL,
	approval_pairing_id TEXT,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	approved_at TEXT,
	cancelled_at TEXT,
	pair_code_issued_at TEXT,
	claimed_at TEXT,
	claimed_machine_id TEXT,
	claimed_retain_until TEXT,
	claim_failure_count INTEGER NOT NULL DEFAULT 0,
	last_failure_at TEXT,
	last_failure_ip TEXT
);

CREATE INDEX IF NOT EXISTS idx_approval_records_state_expires
	ON approval_records (state, expires_at);

CREATE INDEX IF NOT EXISTS idx_approval_records_pairing
	ON approval_records (approval_pairing_id);
