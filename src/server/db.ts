import { Database } from "bun:sqlite";
import { mkdirSync } from "node:fs";
import { dirname } from "node:path";
import type {
	Alert,
	AlertRule,
	DesiredEnsure,
	DesiredItem,
	DesiredManager,
	ExecChunk,
	ExecRunDetail,
	ExecRunSummary,
	ExecStatus,
	ExecTarget,
	ExecTargetDetail,
	Facts,
	Inventory,
	Platform,
	PushSubscriptionInput,
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
CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY,
  machine_id INTEGER NOT NULL REFERENCES machines ON DELETE CASCADE,
  rule TEXT NOT NULL CHECK (rule IN ('unreachable','disk','updates','repo','files')),
  subject TEXT NOT NULL,
  message TEXT NOT NULL,
  opened_at TEXT NOT NULL,
  resolved_at TEXT,
  notified_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS alerts_open ON alerts (machine_id, rule, subject) WHERE resolved_at IS NULL;
CREATE TABLE IF NOT EXISTS desired_items (
  id INTEGER PRIMARY KEY,
  manager TEXT NOT NULL CHECK (manager IN ('brew_formula','brew_cask','pacman','mise')),
  name TEXT NOT NULL,
  version TEXT,
  ensure TEXT NOT NULL CHECK (ensure IN ('present','absent')),
  created_at TEXT NOT NULL,
  UNIQUE (manager, name)
);
CREATE TABLE IF NOT EXISTS desired_targets (
  item_id INTEGER NOT NULL REFERENCES desired_items ON DELETE CASCADE,
  machine_id INTEGER NOT NULL REFERENCES machines ON DELETE CASCADE,
  PRIMARY KEY (item_id, machine_id)
);
CREATE TABLE IF NOT EXISTS push_subscriptions (
  id INTEGER PRIMARY KEY,
  endpoint TEXT UNIQUE NOT NULL,
  p256dh TEXT NOT NULL,
  auth TEXT NOT NULL,
  user_agent TEXT,
  created_at TEXT NOT NULL
);
`;

/** Columns added after a table first shipped: [table, column, definition]. */
const ADDED_COLUMNS: [string, string, string][] = [
	["machines", "notify_unreachable", "INTEGER NOT NULL DEFAULT 1"],
	["exec_runs", "kind", "TEXT NOT NULL DEFAULT 'command'"],
	["exec_targets", "command", "TEXT"],
];

export const RUNS_KEPT_PER_MACHINE = 2000;
export const EXEC_RUNS_KEPT = 500;
export const RESOLVED_ALERTS_KEPT = 1000;

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
	alertRules: {
		unreachableMin: 30,
		diskPercent: 90,
		updatesCount: 20,
		trackedFiles: true,
		repos: [],
	},
};

export interface MachineRow {
	id: number;
	name: string;
	alias: string;
	transport: Transport;
	platform: Platform;
	enabled: number;
	notify_unreachable: number;
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
	kind: "command" | "apply";
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
	command: string | null;
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
	"run_id, machine_id, machine_name, platform, status, exit_code, started_at, finished_at, truncated, command";

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
		command: row.command,
	};
}

function toExecRun(row: ExecRunRow) {
	return {
		id: row.id,
		kind: row.kind,
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

export interface AlertCondition {
	rule: AlertRule;
	subject: string;
	message: string;
}

export interface VapidKeys {
	publicKey: string;
	privateKey: string;
}

export interface PushSubscriptionRow {
	id: number;
	endpoint: string;
	p256dh: string;
	auth: string;
	user_agent: string | null;
	created_at: string;
}

interface AlertRow {
	id: number;
	machine_id: number;
	machine_name: string;
	rule: AlertRule;
	subject: string;
	message: string;
	opened_at: string;
	resolved_at: string | null;
}

const ALERT_SELECT =
	"SELECT a.id, a.machine_id, m.name AS machine_name, a.rule, a.subject, a.message, a.opened_at, a.resolved_at FROM alerts a JOIN machines m ON m.id = a.machine_id";

function toAlert(row: AlertRow): Alert {
	return {
		id: row.id,
		machineId: row.machine_id,
		machineName: row.machine_name,
		rule: row.rule,
		subject: row.subject,
		message: row.message,
		openedAt: row.opened_at,
		resolvedAt: row.resolved_at,
	};
}

interface DesiredItemRow {
	id: number;
	manager: DesiredManager;
	name: string;
	version: string | null;
	ensure: DesiredEnsure;
	created_at: string;
}

function toDesiredItem(row: DesiredItemRow, machineIds: number[]): DesiredItem {
	return {
		id: row.id,
		manager: row.manager,
		name: row.name,
		version: row.version,
		ensure: row.ensure,
		machineIds,
		createdAt: row.created_at,
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
		for (const [table, column, definition] of ADDED_COLUMNS) {
			const columns = this.db
				.query<{ name: string }, []>(`PRAGMA table_info(${table})`)
				.all();
			if (!columns.some((c) => c.name === column))
				this.db.run(`ALTER TABLE ${table} ADD COLUMN ${column} ${definition}`);
		}
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
		patch: {
			name?: string;
			enabled?: boolean;
			platform?: Platform;
			notifyUnreachable?: boolean;
		},
	): void {
		const current = this.getMachine(id);
		if (!current) return;
		const flag = (value: boolean | undefined, fallback: number) =>
			value === undefined ? fallback : value ? 1 : 0;
		this.db
			.query(
				"UPDATE machines SET name = ?, enabled = ?, platform = ?, notify_unreachable = ? WHERE id = ?",
			)
			.run(
				patch.name ?? current.name,
				flag(patch.enabled, current.enabled),
				patch.platform ?? current.platform,
				flag(patch.notifyUnreachable, current.notify_unreachable),
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
		if (!row) return null;
		// Runs recorded before P2 have no updates/repos in their facts.
		const facts: Facts | null = row.facts_json
			? {
					updates: { brew: null, pacman: null },
					repos: null,
					...JSON.parse(row.facts_json),
				}
			: null;
		return { finishedAt: row.finished_at, facts };
	}

	private readValue<T>(key: string): T | undefined {
		const row = this.db
			.query<{ value_json: string }, [string]>(
				"SELECT value_json FROM settings WHERE key = ?",
			)
			.get(key);
		return row ? (JSON.parse(row.value_json) as T) : undefined;
	}

	private writeValue(key: string, value: unknown): void {
		this.db
			.query(
				"INSERT INTO settings (key, value_json) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value_json = excluded.value_json",
			)
			.run(key, JSON.stringify(value));
	}

	/** Only the Settings keys: the table also holds hub secrets that the API must not return. */
	getSettings(): Settings {
		return {
			trackedPaths:
				this.readValue<string[]>("trackedPaths") ??
				DEFAULT_SETTINGS.trackedPaths,
			collectIntervalMin:
				this.readValue<number>("collectIntervalMin") ??
				DEFAULT_SETTINGS.collectIntervalMin,
			alertRules: {
				...DEFAULT_SETTINGS.alertRules,
				...this.readValue<Partial<Settings["alertRules"]>>("alertRules"),
			},
		};
	}

	putSettings(settings: Settings): void {
		this.db.transaction(() => {
			this.writeValue("trackedPaths", settings.trackedPaths);
			this.writeValue("collectIntervalMin", settings.collectIntervalMin);
			this.writeValue("alertRules", settings.alertRules);
		})();
	}

	getVapidKeys(): VapidKeys | null {
		return this.readValue<VapidKeys>("vapidKeys") ?? null;
	}

	putVapidKeys(keys: VapidKeys): void {
		this.writeValue("vapidKeys", keys);
	}

	/**
	 * Reconcile a machine's open alerts with the conditions that hold now. Only rules in
	 * `evaluated` are touched, so a rule that could not be checked (no facts) stays as it was.
	 * Returns the alerts opened by this call.
	 */
	syncAlerts(
		machineId: number,
		evaluated: AlertRule[],
		active: AlertCondition[],
		at: string,
	): Alert[] {
		return this.db.transaction(() => {
			const key = (rule: string, subject: string) => `${rule}\u0000${subject}`;
			const open = new Map(
				this.db
					.query<AlertRow, [number]>(
						`${ALERT_SELECT} WHERE a.machine_id = ? AND a.resolved_at IS NULL`,
					)
					.all(machineId)
					.map((row) => [key(row.rule, row.subject), row]),
			);
			const opened: Alert[] = [];
			for (const c of active) {
				const existing = open.get(key(c.rule, c.subject));
				if (existing) {
					open.delete(key(c.rule, c.subject));
					if (existing.message !== c.message)
						this.db
							.query("UPDATE alerts SET message = ? WHERE id = ?")
							.run(c.message, existing.id);
					continue;
				}
				opened.push(this.insertAlert(machineId, c, at, null));
			}
			const resolve = this.db.query(
				"UPDATE alerts SET resolved_at = ? WHERE id = ?",
			);
			for (const row of open.values()) {
				if (evaluated.includes(row.rule)) resolve.run(at, row.id);
			}
			return opened;
		})();
	}

	/** A one-off event alert (tracked file change): opened and resolved at once. */
	recordAlertEvent(
		machineId: number,
		condition: AlertCondition,
		at: string,
	): Alert {
		return this.insertAlert(machineId, condition, at, at);
	}

	private insertAlert(
		machineId: number,
		c: AlertCondition,
		at: string,
		resolvedAt: string | null,
	): Alert {
		const row = this.db
			.query<
				{ id: number },
				[number, AlertRule, string, string, string, string | null]
			>(
				"INSERT INTO alerts (machine_id, rule, subject, message, opened_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING id",
			)
			.get(machineId, c.rule, c.subject, c.message, at, resolvedAt);
		if (!row) throw new Error("alert insert returned no row");
		this.db
			.query(
				`DELETE FROM alerts WHERE resolved_at IS NOT NULL AND id NOT IN
           (SELECT id FROM alerts WHERE resolved_at IS NOT NULL ORDER BY id DESC LIMIT ?)`,
			)
			.run(RESOLVED_ALERTS_KEPT);
		const alert = this.getAlert(row.id);
		if (!alert) throw new Error("alert vanished after insert");
		return alert;
	}

	getAlert(id: number): Alert | null {
		const row = this.db
			.query<AlertRow, [number]>(`${ALERT_SELECT} WHERE a.id = ?`)
			.get(id);
		return row ? toAlert(row) : null;
	}

	/** Open alerts first (newest first), then resolved ones when `includeResolved`. */
	listAlerts(includeResolved: boolean, limit = 200): Alert[] {
		return this.db
			.query<AlertRow, [number]>(
				`${ALERT_SELECT} ${includeResolved ? "" : "WHERE a.resolved_at IS NULL"}
         ORDER BY a.resolved_at IS NOT NULL, a.id DESC LIMIT ?`,
			)
			.all(limit)
			.map(toAlert);
	}

	markAlertNotified(id: number, at: string): void {
		this.db.query("UPDATE alerts SET notified_at = ? WHERE id = ?").run(at, id);
	}

	listPushSubscriptions(): PushSubscriptionRow[] {
		return this.db
			.query<PushSubscriptionRow, []>("SELECT * FROM push_subscriptions")
			.all();
	}

	putPushSubscription(
		sub: PushSubscriptionInput,
		userAgent: string | null,
	): void {
		this.db
			.query(
				`INSERT INTO push_subscriptions (endpoint, p256dh, auth, user_agent, created_at) VALUES (?, ?, ?, ?, ?)
         ON CONFLICT (endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent`,
			)
			.run(
				sub.endpoint,
				sub.keys.p256dh,
				sub.keys.auth,
				userAgent,
				new Date().toISOString(),
			);
	}

	deletePushSubscription(endpoint: string): void {
		this.db
			.query("DELETE FROM push_subscriptions WHERE endpoint = ?")
			.run(endpoint);
	}

	/**
	 * Start an exec run with every target `running`; keeps the newest EXEC_RUNS_KEPT runs.
	 * `targetIds[i]` is the exec_targets row of `targets[i]`; a target command of null runs
	 * the run's command.
	 */
	createExecRun(run: {
		kind: "command" | "apply";
		command: string;
		timeoutS: number;
		createdBy: string;
		createdAt: string;
		targets: { machine: MachineRow; command: string | null }[];
	}): { run: ExecRunDetail; targetIds: number[] } {
		return this.db.transaction(() => {
			const row = this.db
				.query<ExecRunRow, [string, string, number, string, string]>(
					"INSERT INTO exec_runs (kind, command, timeout_s, created_at, created_by) VALUES (?, ?, ?, ?, ?) RETURNING *",
				)
				.get(run.kind, run.command, run.timeoutS, run.createdAt, run.createdBy);
			if (!row) throw new Error("exec run insert returned no row");
			const insertTarget = this.db.query<
				{ id: number },
				[number, number, string, Platform, string, string | null]
			>(
				"INSERT INTO exec_targets (run_id, machine_id, machine_name, platform, status, started_at, command) VALUES (?, ?, ?, ?, 'running', ?, ?) RETURNING id",
			);
			const targetIds = run.targets.map(({ machine: m, command }) => {
				const target = insertTarget.get(
					row.id,
					m.id,
					m.name,
					m.platform,
					run.createdAt,
					command,
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

	listDesired(): DesiredItem[] {
		const targets = new Map<number, number[]>();
		for (const t of this.db
			.query<{ item_id: number; machine_id: number }, []>(
				"SELECT item_id, machine_id FROM desired_targets ORDER BY machine_id",
			)
			.all()) {
			const list = targets.get(t.item_id) ?? [];
			list.push(t.machine_id);
			targets.set(t.item_id, list);
		}
		return this.db
			.query<DesiredItemRow, []>(
				"SELECT * FROM desired_items ORDER BY manager, name",
			)
			.all()
			.map((row) => toDesiredItem(row, targets.get(row.id) ?? []));
	}

	getDesired(id: number): DesiredItem | null {
		return this.listDesired().find((item) => item.id === id) ?? null;
	}

	addDesired(item: {
		manager: DesiredManager;
		name: string;
		version: string | null;
		ensure: DesiredEnsure;
		machineIds: number[];
	}): DesiredItem {
		const id = this.db.transaction(() => {
			const row = this.db
				.query<
					{ id: number },
					[DesiredManager, string, string | null, DesiredEnsure, string]
				>(
					"INSERT INTO desired_items (manager, name, version, ensure, created_at) VALUES (?, ?, ?, ?, ?) RETURNING id",
				)
				.get(
					item.manager,
					item.name,
					item.version,
					item.ensure,
					new Date().toISOString(),
				);
			if (!row) throw new Error("desired item insert returned no row");
			this.setDesiredTargets(row.id, item.machineIds);
			return row.id;
		})();
		const created = this.getDesired(id);
		if (!created) throw new Error("desired item vanished after insert");
		return created;
	}

	updateDesired(
		id: number,
		patch: {
			version: string | null;
			ensure: DesiredEnsure;
			machineIds: number[];
		},
	): void {
		this.db.transaction(() => {
			this.db
				.query("UPDATE desired_items SET version = ?, ensure = ? WHERE id = ?")
				.run(patch.version, patch.ensure, id);
			this.setDesiredTargets(id, patch.machineIds);
		})();
	}

	private setDesiredTargets(itemId: number, machineIds: number[]): void {
		this.db.query("DELETE FROM desired_targets WHERE item_id = ?").run(itemId);
		const insert = this.db.query(
			"INSERT INTO desired_targets (item_id, machine_id) VALUES (?, ?)",
		);
		for (const machineId of machineIds) insert.run(itemId, machineId);
	}

	deleteDesired(id: number): void {
		this.db.query("DELETE FROM desired_items WHERE id = ?").run(id);
	}
}
