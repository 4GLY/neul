import { Database } from "bun:sqlite";
import { mkdirSync } from "node:fs";
import { dirname } from "node:path";
import type {
	Facts,
	Inventory,
	Platform,
	RunStatus,
	RunSummary,
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
`;

export const RUNS_KEPT_PER_MACHINE = 2000;

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
}
