import { Database } from "bun:sqlite";
import { mkdirSync } from "node:fs";
import { dirname } from "node:path";
import type {
	ExecChunk,
	ExecRunDetail,
	ExecRunSummary,
	ExecStatus,
	ExecTarget,
	ExecTargetDetail,
	Facts,
	Inventory,
	Platform,
	RunStatus,
	RunSummary,
	SavedCommand,
	Settings,
	Transport,
} from "../shared/types.ts";

const SCHEMA = `
CREATE TABLE IF NOT EXISTS machines (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  alias TEXT NOT NULL,
  transport TEXT NOT NULL CHECK (transport IN ('local','ssh')),
  platform TEXT NOT NULL CHECK (platform IN ('posix','windows')),
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS inventories (
  id INTEGER PRIMARY KEY,
  machine_id INTEGER NOT NULL REFERENCES machines ON DELETE CASCADE,
  hash TEXT NOT NULL,
  data_json TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  UNIQUE (machine_id, hash)
);
CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  machine_id INTEGER NOT NULL REFERENCES machines ON DELETE CASCADE,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  duration_ms INTEGER NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('ok','unreachable','error')),
  error TEXT,
  facts_json TEXT,
  inventory_id INTEGER REFERENCES inventories,
  inventory_changed INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS runs_machine_id ON runs (machine_id, id);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS exec_runs (
  id INTEGER PRIMARY KEY,
  command TEXT NOT NULL,
  timeout_s INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  created_by TEXT NOT NULL,
  finished_at TEXT
);
CREATE TABLE IF NOT EXISTS exec_targets (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES exec_runs ON DELETE CASCADE,
  machine_id INTEGER REFERENCES machines ON DELETE SET NULL,
  machine_name TEXT NOT NULL,
  platform TEXT NOT NULL CHECK (platform IN ('posix','windows')),
  status TEXT NOT NULL CHECK (status IN ('running','ok','failed','unreachable','timeout','canceled','interrupted')),
  exit_code INTEGER,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  output_json TEXT NOT NULL DEFAULT '[]',
  truncated INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS exec_targets_run_id ON exec_targets (run_id, id);
CREATE TABLE IF NOT EXISTS saved_commands (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  command TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`;

export const RUNS_KEPT_PER_MACHINE = 2000;
export const EXEC_RUNS_KEPT = 500;

export const DEFAULT_SETTINGS: Settings = {
	trackedPaths: [
		".zshrc",
		".zshenv",
		".zprofile",
		".bashrc",
		".gitconfig",
		".config/git/config",
		".ssh/config",
		".ssh/config.d/*.conf",
		".config/mise/config.toml",
		".config/starship.toml",
		".config/ghostty/config",
	],
	collectIntervalMin: 10,
};

export interface MachineRow {
	id: number;
	name: string;
	alias: string;
	transport: Transport;
	platform: Platform;
	enabled: number;
	created_at: string;
}

interface RunRow {
	id: number;
	machine_id: number;
	started_at: string;
	finished_at: string;
	duration_ms: number;
	status: RunStatus;
	error: string | null;
	facts_json: string | null;
	inventory_changed: number;
}

interface InventoryRow {
	id: number;
	data_json: string;
	first_seen_at: string;
}

export interface NewRun {
	machineId: number;
	startedAt: string;
	finishedAt: string;
	status: RunStatus;
	error: string | null;
	facts: Facts | null;
	inventory: Inventory | null;
}

function toRunSummary(row: RunRow): RunSummary {
	return {
		id: row.id,
		machineId: row.machine_id,
		startedAt: row.started_at,
		finishedAt: row.finished_at,
		durationMs: row.duration_ms,
		status: row.status,
		error: row.error,
		inventoryChanged: row.inventory_changed === 1,
	};
}

interface ExecRunRow {
	id: number;
	command: string;
	timeout_s: number;
	created_at: string;
	created_by: string;
	finished_at: string | null;
}

interface ExecTargetRow {
	run_id: number;
	machine_id: number | null;
	machine_name: string;
	platform: Platform;
	status: ExecStatus;
	exit_code: number | null;
	started_at: string;
	finished_at: string | null;
	truncated: number;
	output_json?: string;
}

interface SavedCommandRow {
	id: number;
	name: string;
	command: string;
	created_at: string;
	updated_at: string;
}

export interface ExecTargetResult {
	status: Exclude<ExecStatus, "running" | "interrupted">;
	exitCode: number | null;
	finishedAt: string;
	output: ExecChunk[];
	truncated: boolean;
}

const EXEC_TARGET_COLUMNS =
	"run_id, machine_id, machine_name, platform, status, exit_code, started_at, finished_at, truncated";

function toExecTarget(row: ExecTargetRow): ExecTarget {
	return {
		machineId: row.machine_id,
		machineName: row.machine_name,
		platform: row.platform,
		status: row.status,
		exitCode: row.exit_code,
		startedAt: row.started_at,
		finishedAt: row.finished_at,
		truncated: row.truncated === 1,
	};
}

function toExecRun(row: ExecRunRow) {
	return {
		id: row.id,
		command: row.command,
		timeoutS: row.timeout_s,
		createdAt: row.created_at,
		createdBy: row.created_by,
		finishedAt: row.finished_at,
	};
}

function toSavedCommand(row: SavedCommandRow): SavedCommand {
	return {
		id: row.id,
		name: row.name,
		command: row.command,
		createdAt: row.created_at,
		updatedAt: row.updated_at,
	};
}

export class Store {
	readonly db: Database;

	constructor(path: string) {
		if (path !== ":memory:") mkdirSync(dirname(path), { recursive: true });
		this.db = new Database(path, { strict: true });
		this.db.run("PRAGMA journal_mode = WAL");
		this.db.run("PRAGMA foreign_keys = ON");
		this.db.run(SCHEMA);
	}

	listMachines(): MachineRow[] {
		return this.db
			.query<MachineRow, []>("SELECT * FROM machines ORDER BY name")
			.all();
	}

	getMachine(id: number): MachineRow | null {
		return this.db
			.query<MachineRow, [number]>("SELECT * FROM machines WHERE id = ?")
			.get(id);
	}

	addMachine(m: {
		name: string;
		alias: string;
		transport: Transport;
		platform: Platform;
	}): MachineRow {
		const row = this.db
			.query<MachineRow, [string, string, Transport, Platform, string]>(
				"INSERT INTO machines (name, alias, transport, platform, created_at) VALUES (?, ?, ?, ?, ?) RETURNING *",
			)
			.get(m.name, m.alias, m.transport, m.platform, new Date().toISOString());
		if (!row) throw new Error("machine insert returned no row");
		return row;
	}

	updateMachine(
		id: number,
		patch: { name?: string; enabled?: boolean; platform?: Platform },
	): void {
		const current = this.getMachine(id);
		if (!current) return;
		this.db
			.query(
				"UPDATE machines SET name = ?, enabled = ?, platform = ? WHERE id = ?",
			)
			.run(
				patch.name ?? current.name,
				patch.enabled === undefined ? current.enabled : patch.enabled ? 1 : 0,
				patch.platform ?? current.platform,
				id,
			);
	}

	deleteMachine(id: number): void {
		this.db.query("DELETE FROM machines WHERE id = ?").run(id);
	}

	/** Record a collection run; inventories are deduplicated by content hash (design.md §4.3). */
	recordRun(run: NewRun): RunSummary {
		return this.db.transaction(() => {
			let inventoryId: number | null = null;
			let changed = false;
			if (run.inventory) {
				const json = JSON.stringify(run.inventory);
				const hash = new Bun.CryptoHasher("sha256").update(json).digest("hex");
				const existing = this.db
					.query<{ id: number }, [number, string]>(
						"SELECT id FROM inventories WHERE machine_id = ? AND hash = ?",
					)
					.get(run.machineId, hash);
				const previous = this.latestInventoryId(run.machineId);
				if (existing) {
					inventoryId = existing.id;
					this.db
						.query("UPDATE inventories SET last_seen_at = ? WHERE id = ?")
						.run(run.finishedAt, existing.id);
				} else {
					inventoryId =
						this.db
							.query<{ id: number }, [number, string, string, string, string]>(
								"INSERT INTO inventories (machine_id, hash, data_json, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, ?) RETURNING id",
							)
							.get(run.machineId, hash, json, run.finishedAt, run.finishedAt)
							?.id ?? null;
				}
				changed = previous !== null && previous !== inventoryId;
			}
			const row = this.db
				.query<
					RunRow,
					[
						number,
						string,
						string,
						number,
						RunStatus,
						string | null,
						string | null,
						number | null,
						number,
					]
				>(
					`INSERT INTO runs (machine_id, started_at, finished_at, duration_ms, status, error, facts_json, inventory_id, inventory_changed)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *`,
				)
				.get(
					run.machineId,
					run.startedAt,
					run.finishedAt,
					Date.parse(run.finishedAt) - Date.parse(run.startedAt),
					run.status,
					run.error,
					run.facts ? JSON.stringify(run.facts) : null,
					inventoryId,
					changed ? 1 : 0,
				);
			if (!row) throw new Error("run insert returned no row");
			this.db
				.query(
					`DELETE FROM runs WHERE machine_id = ? AND id NOT IN
             (SELECT id FROM runs WHERE machine_id = ? ORDER BY id DESC LIMIT ?)`,
				)
				.run(run.machineId, run.machineId, RUNS_KEPT_PER_MACHINE);
			return toRunSummary(row);
		})();
	}

	/** Inventory referenced by the most recent run that produced one. */
	private latestInventoryId(machineId: number): number | null {
		return (
			this.db
				.query<{ inventory_id: number }, [number]>(
					"SELECT inventory_id FROM runs WHERE machine_id = ? AND inventory_id IS NOT NULL ORDER BY id DESC LIMIT 1",
				)
				.get(machineId)?.inventory_id ?? null
		);
	}

	latestInventory(
		machineId: number,
	): { inventory: Inventory; since: string } | null {
		const id = this.latestInventoryId(machineId);
		if (id === null) return null;
		const row = this.db
			.query<InventoryRow, [number]>(
				"SELECT id, data_json, first_seen_at FROM inventories WHERE id = ?",
			)
			.get(id);
		return row
			? { inventory: JSON.parse(row.data_json), since: row.first_seen_at }
			: null;
	}

	recentRuns(machineId: number, limit: number): RunSummary[] {
		return this.db
			.query<RunRow, [number, number]>(
				"SELECT * FROM runs WHERE machine_id = ? ORDER BY id DESC LIMIT ?",
			)
			.all(machineId, limit)
			.map(toRunSummary);
	}

	lastRun(machineId: number): RunSummary | null {
		return this.recentRuns(machineId, 1)[0] ?? null;
	}

	lastOkRun(
		machineId: number,
	): { finishedAt: string; facts: Facts | null } | null {
		const row = this.db
			.query<RunRow, [number]>(
				"SELECT * FROM runs WHERE machine_id = ? AND status = 'ok' ORDER BY id DESC LIMIT 1",
			)
			.get(machineId);
		return row
			? {
					finishedAt: row.finished_at,
					facts: row.facts_json ? JSON.parse(row.facts_json) : null,
				}
			: null;
	}

	getSettings(): Settings {
		const rows = this.db
			.query<{ key: string; value_json: string }, []>("SELECT * FROM settings")
			.all();
		const stored = Object.fromEntries(
			rows.map((r) => [r.key, JSON.parse(r.value_json)]),
		);
		return { ...DEFAULT_SETTINGS, ...stored };
	}

	putSettings(settings: Settings): void {
		const upsert = this.db.query(
			"INSERT INTO settings (key, value_json) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value_json = excluded.value_json",
		);
		this.db.transaction(() => {
			upsert.run("trackedPaths", JSON.stringify(settings.trackedPaths));
			upsert.run(
				"collectIntervalMin",
				JSON.stringify(settings.collectIntervalMin),
			);
		})();
	}

	/**
	 * Start an exec run with every target `running`; keeps the newest EXEC_RUNS_KEPT runs.
	 * `targetIds[i]` is the exec_targets row of `machines[i]`.
	 */
	createExecRun(run: {
		command: string;
		timeoutS: number;
		createdBy: string;
		createdAt: string;
		machines: MachineRow[];
	}): { run: ExecRunDetail; targetIds: number[] } {
		return this.db.transaction(() => {
			const row = this.db
				.query<ExecRunRow, [string, number, string, string]>(
					"INSERT INTO exec_runs (command, timeout_s, created_at, created_by) VALUES (?, ?, ?, ?) RETURNING *",
				)
				.get(run.command, run.timeoutS, run.createdAt, run.createdBy);
			if (!row) throw new Error("exec run insert returned no row");
			const insertTarget = this.db.query<
				{ id: number },
				[number, number, string, Platform, string]
			>(
				"INSERT INTO exec_targets (run_id, machine_id, machine_name, platform, status, started_at) VALUES (?, ?, ?, ?, 'running', ?) RETURNING id",
			);
			const targetIds = run.machines.map((m) => {
				const target = insertTarget.get(
					row.id,
					m.id,
					m.name,
					m.platform,
					run.createdAt,
				);
				if (!target) throw new Error("exec target insert returned no row");
				return target.id;
			});
			this.db
				.query(
					"DELETE FROM exec_runs WHERE id NOT IN (SELECT id FROM exec_runs ORDER BY id DESC LIMIT ?)",
				)
				.run(EXEC_RUNS_KEPT);
			const detail = this.getExecRun(row.id);
			if (!detail) throw new Error("exec run vanished after insert");
			return { run: detail, targetIds };
		})();
	}

	/**
	 * Record a finished target. Returns the run's finishedAt once no target is running,
	 * otherwise null.
	 */
	finishExecTarget(
		runId: number,
		targetId: number,
		result: ExecTargetResult,
	): string | null {
		return this.db.transaction(() => {
			this.db
				.query(
					"UPDATE exec_targets SET status = ?, exit_code = ?, finished_at = ?, output_json = ?, truncated = ? WHERE id = ?",
				)
				.run(
					result.status,
					result.exitCode,
					result.finishedAt,
					JSON.stringify(result.output),
					result.truncated ? 1 : 0,
					targetId,
				);
			const running = this.db
				.query<{ n: number }, [number]>(
					"SELECT count(*) AS n FROM exec_targets WHERE run_id = ? AND status = 'running'",
				)
				.get(runId);
			if (running?.n) return null;
			this.db
				.query("UPDATE exec_runs SET finished_at = ? WHERE id = ?")
				.run(result.finishedAt, runId);
			return result.finishedAt;
		})();
	}

	/** Close runs left `running` by a previous hub process (design.md §8 P1). */
	interruptExecRuns(at: string): number {
		return this.db.transaction(() => {
			const changes = this.db
				.query(
					"UPDATE exec_targets SET status = 'interrupted', finished_at = ? WHERE status = 'running'",
				)
				.run(at).changes;
			this.db
				.query("UPDATE exec_runs SET finished_at = ? WHERE finished_at IS NULL")
				.run(at);
			return changes;
		})();
	}

	getExecRun(id: number): ExecRunDetail | null {
		const row = this.db
			.query<ExecRunRow, [number]>("SELECT * FROM exec_runs WHERE id = ?")
			.get(id);
		if (!row) return null;
		const targets = this.db
			.query<Required<ExecTargetRow>, [number]>(
				`SELECT ${EXEC_TARGET_COLUMNS}, output_json FROM exec_targets WHERE run_id = ? ORDER BY id`,
			)
			.all(id)
			.map(
				(t): ExecTargetDetail => ({
					...toExecTarget(t),
					output: JSON.parse(t.output_json),
				}),
			);
		return { ...toExecRun(row), targets };
	}

	listExecRuns(limit: number): ExecRunSummary[] {
		const rows = this.db
			.query<ExecRunRow, [number]>(
				"SELECT * FROM exec_runs ORDER BY id DESC LIMIT ?",
			)
			.all(limit);
		if (rows.length === 0) return [];
		const targets = new Map<number, ExecTarget[]>();
		for (const t of this.db
			.query<ExecTargetRow, [number]>(
				`SELECT ${EXEC_TARGET_COLUMNS} FROM exec_targets WHERE run_id >= ? ORDER BY id`,
			)
			.all(rows[rows.length - 1]?.id ?? 0)) {
			const list = targets.get(t.run_id) ?? [];
			list.push(toExecTarget(t));
			targets.set(t.run_id, list);
		}
		return rows.map((r) => ({
			...toExecRun(r),
			targets: targets.get(r.id) ?? [],
		}));
	}

	listSavedCommands(): SavedCommand[] {
		return this.db
			.query<SavedCommandRow, []>("SELECT * FROM saved_commands ORDER BY name")
			.all()
			.map(toSavedCommand);
	}

	getSavedCommand(id: number): SavedCommand | null {
		const row = this.db
			.query<SavedCommandRow, [number]>(
				"SELECT * FROM saved_commands WHERE id = ?",
			)
			.get(id);
		return row ? toSavedCommand(row) : null;
	}

	addSavedCommand(name: string, command: string): SavedCommand {
		const now = new Date().toISOString();
		const row = this.db
			.query<SavedCommandRow, [string, string, string, string]>(
				"INSERT INTO saved_commands (name, command, created_at, updated_at) VALUES (?, ?, ?, ?) RETURNING *",
			)
			.get(name, command, now, now);
		if (!row) throw new Error("saved command insert returned no row");
		return toSavedCommand(row);
	}

	updateSavedCommand(
		id: number,
		name: string,
		command: string,
	): SavedCommand | null {
		const row = this.db
			.query<SavedCommandRow, [string, string, string, number]>(
				"UPDATE saved_commands SET name = ?, command = ?, updated_at = ? WHERE id = ? RETURNING *",
			)
			.get(name, command, new Date().toISOString(), id);
		return row ? toSavedCommand(row) : null;
	}

	deleteSavedCommand(id: number): void {
		this.db.query("DELETE FROM saved_commands WHERE id = ?").run(id);
	}
}
