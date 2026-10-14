// API contract shared by the hub (src/server) and the web UI (src/web). See docs/design.md §6.

export type Transport = "local" | "ssh";
export type Platform = "posix" | "windows";
export type RunStatus = "ok" | "unreachable" | "error";
export type MachineStatus = "online" | "unreachable" | "error" | "unknown";

export interface Facts {
	hostname: string | null;
	os: string | null; // uname -s: Darwin | Linux | Windows
	osName: string | null; // "macOS 26.1", "Arch Linux", "Windows 11 Pro"
	kernel: string | null;
	arch: string | null;
	model: string | null;
	user: string | null;
	cpuCount: number | null;
	uptimeS: number | null;
	load: [number, number, number] | null;
	memTotalB: number | null;
	memAvailableB: number | null;
	disk: {
		path: string;
		totalB: number;
		usedB: number;
		availableB: number;
	} | null;
	battery: { percent: number; state: string } | null;
	updates: {
		brew: string[] | null; // `brew outdated` names; null = brew not present
		pacman: string[] | null; // `checkupdates` names; null = unavailable or failed
	};
	repos: RepoStatus[] | null; // null = no repos configured or windows
}

export interface RepoStatus {
	path: string; // relative to $HOME
	state: "ok" | "missing";
	dirty: number | null; // `git status --porcelain` entries
	ahead: number | null; // commits ahead of upstream; null = no upstream
}

export interface BrewPackage {
	name: string;
	versions: string[];
	onRequest: boolean; // formula installed on request (brew leaves); always true for casks
}

export interface PacmanPackage {
	name: string;
	version: string;
}

export interface MiseTool {
	name: string;
	activeVersion: string | null; // version active in $HOME context (global config)
	requestedVersion: string | null; // e.g. "latest", "24", "1.14"
	installedVersions: string[];
	source: string | null; // config file that requests it
}

export type FileKind = "file" | "symlink" | "dir" | "broken" | "missing";

export interface TrackedFile {
	path: string; // relative to $HOME (or the unmatched pattern when kind = "missing")
	kind: FileKind;
	sha256: string | null;
	size: number | null;
	linkTarget: string | null;
}

export interface Inventory {
	brewFormulae: BrewPackage[] | null; // null = brew not present
	brewCasks: BrewPackage[] | null;
	pacmanExplicit: PacmanPackage[] | null; // null = pacman not present
	pacmanTotal: number | null;
	pacmanSudo?: boolean | null; // `sudo -n pacman` works; absent in inventories collected before P3
	mise: MiseTool[] | null; // null = mise not present
	files: TrackedFile[] | null; // null = not collected (e.g. windows)
}

export interface InventoryCounts {
	brewFormulae: number | null;
	brewLeaves: number | null;
	brewCasks: number | null;
	pacmanExplicit: number | null;
	pacmanTotal: number | null;
	miseTools: number | null;
	files: number | null;
}

export interface RunSummary {
	id: number;
	machineId: number;
	startedAt: string; // ISO 8601 UTC
	finishedAt: string;
	durationMs: number;
	status: RunStatus;
	error: string | null;
	inventoryChanged: boolean; // this run produced a new inventory version
}

export interface MachineSummary {
	id: number;
	name: string;
	alias: string;
	transport: Transport;
	platform: Platform;
	enabled: boolean;
	notifyUnreachable: boolean; // alert when the machine stays unreachable (off for laptops that sleep)
	status: MachineStatus;
	collecting: boolean; // a collection is in flight right now
	lastRun: RunSummary | null;
	lastSeenAt: string | null; // finishedAt of the last ok run
	facts: Facts | null; // from the last ok run
	counts: InventoryCounts | null; // from the latest inventory
}

export interface MachineDetail extends MachineSummary {
	inventory: Inventory | null;
	inventorySince: string | null; // first_seen_at of the current inventory version
	recentRuns: RunSummary[]; // newest first, at most 50
}

export interface CompareMachine {
	id: number;
	name: string;
	status: MachineStatus;
}

export interface CompareRow {
	key: string; // package / tool name, or file path
	cells: Record<number, string | null>; // machineId -> value (version, short hash, "missing", "→ target"); null = absent
	uniform: boolean; // every machine that has the category reports the same value
	dependencyOnly?: boolean; // brew formulae: never installed on request, so not adoptable
}

export interface CompareSection {
	id: "brewFormulae" | "brewCasks" | "mise" | "pacman" | "files";
	title: string;
	machineIds: number[]; // machines that report this category (columns that apply)
	rows: CompareRow[];
}

export interface CompareView {
	machines: CompareMachine[];
	sections: CompareSection[];
}

export interface Settings {
	trackedPaths: string[];
	collectIntervalMin: number;
	alertRules: AlertRules;
}

// P2 monitoring and alerts (design.md §5, §6, §8 P2).

/** Each threshold is null when its rule is off. */
export interface AlertRules {
	unreachableMin: number | null; // no successful collection for this many minutes
	diskPercent: number | null; // disk used ≥ this percent
	updatesCount: number | null; // brew + pacman pending updates ≥ this
	trackedFiles: boolean; // tracked file content changed
	repos: string[]; // git repos ($HOME-relative) to watch for uncommitted or unpushed work
}

export type AlertRule = "unreachable" | "disk" | "updates" | "repo" | "files";

export const ALERT_RULE_LABELS: Record<AlertRule, string> = {
	unreachable: "연결 끊김",
	disk: "디스크",
	updates: "업데이트 대기",
	repo: "저장소 변경",
	files: "추적 파일 변경",
};

export interface Alert {
	id: number;
	machineId: number;
	machineName: string;
	rule: AlertRule;
	subject: string; // disk path, repo path, or "" when the rule has one alert per machine
	message: string;
	openedAt: string;
	resolvedAt: string | null; // null while open; events (files) are resolved when opened
}

export interface PushSubscriptionInput {
	endpoint: string;
	keys: { p256dh: string; auth: string };
}

export interface AddMachineRequest {
	alias: string;
	name?: string;
	transport?: Transport;
	platform?: Platform; // omitted = auto-detect
}

export interface UpdateMachineRequest {
	name?: string;
	enabled?: boolean;
	platform?: Platform;
	notifyUnreachable?: boolean;
}

// P1 remote execution (design.md §5, §6).

export type ExecStatus =
	| "running"
	| "ok" // exit 0
	| "failed" // non-zero exit
	| "unreachable" // ssh exit 255
	| "timeout"
	| "canceled"
	| "interrupted"; // the hub stopped while the target was running

export type ExecStream = "stdout" | "stderr";

export interface ExecChunk {
	stream: ExecStream;
	data: string;
}

export interface ExecTarget {
	machineId: number | null; // null once the machine is deleted
	machineName: string; // name at the time of the run
	platform: Platform;
	status: ExecStatus;
	exitCode: number | null;
	startedAt: string;
	finishedAt: string | null;
	truncated: boolean; // output exceeded the per-target limit and the rest was dropped
	command: string | null; // this target's own script (apply runs); null = the run's command
}

export interface ExecTargetDetail extends ExecTarget {
	output: ExecChunk[]; // in arrival order; consecutive chunks of one stream are merged
}

export interface ExecRunSummary {
	id: number;
	kind: "command" | "apply"; // apply = desired-state changes generated by the hub
	command: string; // apply runs: a one-line summary; each target has its own command
	timeoutS: number;
	createdAt: string;
	createdBy: string;
	finishedAt: string | null; // null while any target is running
	targets: ExecTarget[];
}

export interface ExecRunDetail extends Omit<ExecRunSummary, "targets"> {
	targets: ExecTargetDetail[];
}

export interface ExecRequest {
	command: string;
	machineIds: number[];
	timeoutS?: number; // per target, default 300
}

/** Server-sent events on GET /api/exec/:id/events; the SSE event name is `type`. */
export type ExecEvent =
	| { type: "snapshot"; run: ExecRunDetail } // first event of every connection
	| { type: "chunk"; machineId: number; stream: ExecStream; data: string }
	| { type: "exit"; machineId: number; target: ExecTarget }
	| { type: "done"; finishedAt: string };

export interface SavedCommand {
	id: number;
	name: string;
	command: string;
	createdAt: string;
	updatedAt: string;
}

export interface SavedCommandInput {
	name: string;
	command: string;
}

export interface ApiError {
	error: string;
}

// P3 desired state for packages and tools (design.md §5, §6, §8 P3).

export type DesiredManager = "brew_formula" | "brew_cask" | "pacman" | "mise";
export type DesiredEnsure = "present" | "absent";
export type DesiredStatus =
	| "in_sync"
	| "drifted"
	| "pending"
	| "blocked"
	| "unknown";

export interface DesiredItem {
	id: number;
	manager: DesiredManager;
	name: string;
	version: string | null; // mise only: the version requested in the global config
	ensure: DesiredEnsure;
	machineIds: number[]; // target machines
	createdAt: string;
}

export interface DesiredCell {
	status: DesiredStatus;
	detail: string | null; // what differs, or why the machine is blocked
}

export interface DesiredRow extends DesiredItem {
	cells: Record<number, DesiredCell>; // machineId -> state, target machines only
}

export interface DesiredView {
	machines: { id: number; name: string; status: MachineStatus }[];
	items: DesiredRow[];
}

export interface DesiredInput {
	manager: DesiredManager;
	name: string;
	version?: string | null;
	ensure: DesiredEnsure;
	machineIds: number[];
}

export interface DesiredUpdate {
	version?: string | null;
	ensure?: DesiredEnsure;
	machineIds?: number[];
}

/** One machine's view of an item before adopting it. */
export interface DesiredCandidate {
	machineId: number;
	machineName: string;
	blocked: string | null; // why this machine cannot be a target
	installed: boolean; // brew/pacman: installed; mise: requested in the global config
	version: string | null; // mise: the globally requested version
}

export interface PlanStep {
	itemId: number;
	manager: DesiredManager;
	name: string;
	version: string | null;
	action: "install" | "remove";
}

export interface MachinePlan {
	machineId: number;
	machineName: string;
	steps: PlanStep[];
	command: string; // exactly what the apply run executes
}

export interface ApplyRequest {
	plans: { machineId: number; command: string }[]; // as confirmed; must match the current plan
}
