CREATE TABLE IF NOT EXISTS owners (
	id TEXT PRIMARY KEY,
	setup_token_hash TEXT,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	owner_id TEXT NOT NULL,
	session_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	expires_at TEXT,
	FOREIGN KEY (owner_id) REFERENCES owners(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS pairing_codes (
	id TEXT PRIMARY KEY,
	code_hash TEXT NOT NULL UNIQUE,
	machine_id TEXT,
	expires_at TEXT NOT NULL,
	used_at TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS machines (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	os TEXT NOT NULL,
	arch TEXT NOT NULL,
	last_heartbeat_at TEXT,
	agent_version TEXT,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS machine_tokens (
	id TEXT PRIMARY KEY,
	machine_id TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	revoked_at TEXT,
	FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS profiles (
	id TEXT PRIMARY KEY,
	owner_id TEXT,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (owner_id) REFERENCES owners(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS segments (
	id TEXT PRIMARY KEY,
	profile_id TEXT,
	name TEXT NOT NULL,
	priority INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS resources (
	id TEXT PRIMARY KEY,
	segment_id TEXT,
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	spec_json TEXT NOT NULL,
	desired_version INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY (segment_id) REFERENCES segments(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS file_versions (
	id TEXT PRIMARY KEY,
	resource_id TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS reconcile_runs (
	id TEXT PRIMARY KEY,
	machine_id TEXT NOT NULL,
	reason TEXT NOT NULL,
	idempotency_key TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	finished_at TEXT,
	FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS reconcile_events (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	resource_id TEXT,
	status TEXT NOT NULL,
	message TEXT,
	desired_version INTEGER,
	applied_version INTEGER,
	created_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES reconcile_runs(id) ON DELETE CASCADE,
	FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS agent_reports (
	id TEXT PRIMARY KEY,
	machine_id TEXT NOT NULL,
	idempotency_key TEXT NOT NULL UNIQUE,
	report_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_commands (
	id TEXT PRIMARY KEY,
	machine_id TEXT NOT NULL,
	command_type TEXT NOT NULL,
	payload_json TEXT NOT NULL,
	idempotency_key TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	acked_at TEXT,
	FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
);
