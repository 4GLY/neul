import type {
	DesiredCandidate,
	DesiredCell,
	DesiredFile,
	DesiredFileRow,
	DesiredItem,
	DesiredManager,
	DesiredView,
	FileCandidate,
	Inventory,
	MachinePlan,
	MiseTool,
	PlanStep,
	TrackedFile,
} from "../shared/types.ts";
import type { MachineRow, Store } from "./db.ts";
import type { Executor } from "./exec.ts";
import type { Hub } from "./hub.ts";
import type { Secrets } from "./secrets.ts";

type PackageStep = Extract<PlanStep, { kind: "package" }>;
type FileStep = Extract<PlanStep, { kind: "file" }>;

export const sha256Hex = (text: string): string =>
	new Bun.CryptoHasher("sha256").update(text).digest("hex");

/** mise's default global config; tools requested anywhere else are not "global". */
const GLOBAL_MISE_CONFIG = "/.config/mise/config.toml";

/** Package and tool names that are safe to pass to brew/pacman/mise (taps and backends included). */
export const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9@._+:/-]{0,199}$/;
export const VERSION_PATTERN = /^[A-Za-z0-9][A-Za-z0-9@._+:-]{0,99}$/;

/** Why a machine can never apply this manager's changes, or null when it can. */
export function blockedReason(
	manager: DesiredManager,
	machine: MachineRow,
	inventory: Inventory | null,
): string | null {
	if (machine.platform === "windows")
		return "Windows는 설정 배포 대상이 아닙니다";
	if (!inventory) return null;
	switch (manager) {
		case "brew_formula":
		case "brew_cask":
			return inventory.brewFormulae === null ? "brew가 없습니다" : null;
		case "pacman":
			if (inventory.pacmanExplicit === null) return "pacman이 없습니다";
			return inventory.pacmanSudo === true
				? null
				: "sudo -n pacman을 쓸 수 없습니다 (NOPASSWD 규칙 필요)";
		case "mise":
			return inventory.mise === null ? "mise가 없습니다" : null;
	}
}

function globalMiseTool(inventory: Inventory, name: string): MiseTool | null {
	const tool = inventory.mise?.find((t) => t.name === name);
	return tool?.requestedVersion && tool.source?.endsWith(GLOBAL_MISE_CONFIG)
		? tool
		: null;
}

/**
 * Current state of one item on a machine. brew counts any installed formula or cask, pacman
 * only explicitly installed packages (`pacman -Qe`), mise only tools requested in the global
 * config and actually installed.
 */
export function installedState(
	manager: DesiredManager,
	name: string,
	inventory: Inventory,
): { installed: boolean; version: string | null } {
	switch (manager) {
		case "brew_formula":
			return {
				installed: !!inventory.brewFormulae?.some((p) => p.name === name),
				version: null,
			};
		case "brew_cask":
			return {
				installed: !!inventory.brewCasks?.some((p) => p.name === name),
				version: null,
			};
		case "pacman":
			return {
				installed: !!inventory.pacmanExplicit?.some((p) => p.name === name),
				version: null,
			};
		case "mise": {
			const tool = globalMiseTool(inventory, name);
			return {
				installed:
					!!tool?.activeVersion &&
					tool.installedVersions.includes(tool.activeVersion),
				version: tool?.requestedVersion ?? null,
			};
		}
	}
}

export function cellFor(
	item: DesiredItem,
	machine: MachineRow,
	inventory: Inventory | null,
	pending: boolean,
): DesiredCell {
	if (pending)
		return { status: "pending", detail: "적용 중이거나 다시 수집하는 중" };
	const blocked = blockedReason(item.manager, machine, inventory);
	if (blocked) return { status: "blocked", detail: blocked };
	if (!inventory)
		return { status: "unknown", detail: "수집된 인벤토리가 없습니다" };
	const { installed, version } = installedState(
		item.manager,
		item.name,
		inventory,
	);
	if (item.ensure === "absent")
		return installed
			? { status: "drifted", detail: "설치되어 있음" }
			: { status: "in_sync", detail: null };
	if (item.manager === "mise") {
		if (version === null)
			return { status: "drifted", detail: "전역 설정에 없음" };
		if (version !== item.version)
			return { status: "drifted", detail: `전역 요청 ${version}` };
		if (!installed)
			return { status: "drifted", detail: `${version} 요청됐지만 설치 안 됨` };
		return { status: "in_sync", detail: null };
	}
	return installed
		? { status: "in_sync", detail: null }
		: { status: "drifted", detail: "설치 안 됨" };
}

/** Why a machine can never apply a file at `path`, or null when it can. */
export function fileBlockedReason(
	machine: MachineRow,
	inventory: Inventory | null,
	path: string,
): string | null {
	if (machine.platform === "windows")
		return "Windows는 설정 배포 대상이 아닙니다";
	const tracked = inventory?.files?.find((f) => f.path === path);
	if (tracked?.kind === "symlink" || tracked?.kind === "broken")
		return `symlink(→ ${tracked.linkTarget}) — 링크 대상 저장소가 관리하는 경로`;
	if (tracked?.kind === "dir") return "디렉터리";
	return null;
}

export function fileCellFor(
	file: DesiredFile,
	machine: MachineRow,
	inventory: Inventory | null,
	pending: boolean,
): DesiredCell {
	if (pending)
		return { status: "pending", detail: "적용 중이거나 다시 수집하는 중" };
	if (machine.platform === "windows")
		return { status: "blocked", detail: "Windows는 설정 배포 대상이 아닙니다" };
	if (!inventory)
		return { status: "unknown", detail: "수집된 인벤토리가 없습니다" };
	const tracked = inventory.files?.find((f) => f.path === file.path);
	if (!tracked)
		return { status: "unknown", detail: "아직 이 경로를 수집하지 않았습니다" };
	const blocked = fileBlockedReason(machine, inventory, file.path);
	if (blocked) return { status: "blocked", detail: blocked };
	if (file.ensure === "absent")
		return tracked.kind === "missing"
			? { status: "in_sync", detail: null }
			: { status: "drifted", detail: "파일이 있음" };
	if (!file.current) return { status: "blocked", detail: "내용 버전이 없음" };
	if (tracked.kind === "missing") return { status: "drifted", detail: "없음" };
	if (tracked.sha256 !== file.current.sha256)
		return { status: "drifted", detail: "내용 다름" };
	// Inventories collected before P4 carry no mode; only compare when both are known.
	if (tracked.mode && tracked.mode !== file.mode)
		return { status: "drifted", detail: `권한 ${tracked.mode} → ${file.mode}` };
	return { status: "in_sync", detail: null };
}

const quote = (value: string) => `'${value.replaceAll("'", `'\\''`)}'`;

/** The shell commands for one group of steps sharing a manager and an action. */
function commandsFor(
	manager: DesiredManager,
	action: PackageStep["action"],
	steps: PackageStep[],
): string[] {
	const names = steps.map((s) => quote(s.name)).join(" ");
	switch (manager) {
		case "brew_formula":
		case "brew_cask": {
			const kind = manager === "brew_cask" ? "--cask" : "--formula";
			return [
				`brew ${action === "install" ? "install" : "uninstall"} ${kind} ${names}`,
			];
		}
		case "pacman":
			// --needed skips packages already installed as dependencies; -D marks them explicit
			// so `pacman -Qe` reports them as present afterwards.
			return action === "install"
				? [
						`sudo -n pacman -S --needed --noconfirm ${names}`,
						`sudo -n pacman -D --asexplicit ${names}`,
					]
				: [`sudo -n pacman -Rns --noconfirm ${names}`];
		case "mise":
			return action === "install"
				? [
						`mise use -g -y ${steps.map((s) => quote(`${s.name}@${s.version}`)).join(" ")}`,
					]
				: [`mise unuse -g -y ${names}`];
	}
}

const MANAGER_ORDER: DesiredManager[] = [
	"brew_formula",
	"brew_cask",
	"pacman",
	"mise",
];

/**
 * File helpers, relative to $HOME. Writes go to a temp file in the target directory and are
 * renamed over the target (atomic), after copying any existing file to `<path>.neul-bak-<ts>`.
 * Deletes rename the file to the backup name. Symlinks are refused: their targets belong to
 * another source of truth (e.g. a stow-managed dotfiles repo).
 */
const FILE_HELPERS = `neul_ts=$(date -u +%Y%m%dT%H%M%SZ)
neul_write() { t="$HOME/$1"; d=$(dirname "$t")
  if [ -L "$t" ]; then echo "neul: $1 is a symlink; not touched" >&2; return 1; fi
  [ -d "$d" ] || { mkdir -p "$d" && chmod 700 "$d"; } || return 1
  tmp=$(mktemp "$d/.neul-tmp.XXXXXX") || return 1
  if base64 -d > "$tmp" && chmod "$2" "$tmp"; then :; else rm -f "$tmp"; return 1; fi
  if [ -e "$t" ]; then cp -p "$t" "$t.neul-bak-$neul_ts" || { rm -f "$tmp"; return 1; }; fi
  mv -f "$tmp" "$t"; }
neul_delete() { t="$HOME/$1"
  if [ -L "$t" ]; then echo "neul: $1 is a symlink; not touched" >&2; return 1; fi
  [ -e "$t" ] || return 0
  mv -f "$t" "$t.neul-bak-$neul_ts"; }`;

/**
 * POSIX script for one machine: file deletes, package removals, package installs, then file
 * writes; every step runs even if an earlier one fails, and the exit code is non-zero if any
 * failed. `display` omits file contents (it is what the user confirms and the run records);
 * `script` carries them base64-encoded from `contents` (fileId -> plaintext).
 */
export function planScript(
	steps: PlanStep[],
	contents: Map<number, string>,
): { display: string; script: string } {
	const { display, script } = renderPlan(steps, contents);
	return { display, script: script as string };
}

/** The confirmable text of a plan, without file contents. */
export function planDisplay(steps: PlanStep[]): string {
	return renderPlan(steps, null).display;
}

function renderPlan(
	steps: PlanStep[],
	contents: Map<number, string> | null,
): { display: string; script: string | null } {
	const display: string[] = [];
	const script: string[] = [];
	const both = (line: string) => {
		display.push(line);
		script.push(line);
	};
	const packages = steps.filter((s): s is PackageStep => s.kind === "package");
	const files = steps.filter((s): s is FileStep => s.kind === "file");
	if (packages.some((s) => s.manager.startsWith("brew_")))
		both("export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ENV_HINTS=1");
	if (files.length > 0) both(FILE_HELPERS);
	both("neul_rc=0");
	for (const step of files.filter((s) => s.action === "delete")) {
		both(`printf '%s\\n' ${quote(`$ delete ${step.path}`)}`);
		both(`neul_delete ${quote(step.path)} || neul_rc=1`);
	}
	for (const action of ["remove", "install"] as const) {
		for (const manager of MANAGER_ORDER) {
			const group = packages.filter(
				(s) => s.manager === manager && s.action === action,
			);
			if (group.length === 0) continue;
			for (const command of commandsFor(manager, action, group)) {
				both(`printf '%s\\n' ${quote(`$ ${command}`)}`);
				both(`${command} || neul_rc=1`);
			}
		}
	}
	for (const step of files.filter((s) => s.action === "write")) {
		both(`printf '%s\\n' ${quote(`$ write ${step.path} (${step.mode})`)}`);
		both(
			`neul_write ${quote(step.path)} ${step.mode} <<'NEUL_B64' || neul_rc=1`,
		);
		display.push(
			`# content omitted · sha256 ${step.sha256?.slice(0, 12)} · ${step.size} bytes`,
		);
		if (contents) {
			const content = contents.get(step.fileId);
			if (content === undefined)
				throw new Error(`no content for file ${step.fileId}`);
			script.push(Buffer.from(content, "utf8").toString("base64"));
		}
		both("NEUL_B64");
	}
	both("exit $neul_rc");
	return {
		display: display.join("\n"),
		script: contents ? script.join("\n") : null,
	};
}

export class FileVersionChanged extends Error {
	constructor() {
		super("파일 버전이 바뀌었습니다");
	}
}

/** Desired-state views, plans, and the pending state between an apply and its re-collection. */
export class Desired {
	private readonly recollecting = new Set<number>();

	constructor(
		private readonly store: Store,
		private readonly hub: Hub,
		private readonly executor: Executor,
		private readonly secrets: Secrets,
	) {}

	private pending(): Set<number> {
		return new Set([...this.recollecting, ...this.executor.applyingMachines()]);
	}

	private inventories(): Map<number, Inventory | null> {
		return new Map(
			this.store
				.listMachines()
				.map((m) => [
					m.id,
					this.store.latestInventory(m.id)?.inventory ?? null,
				]),
		);
	}

	view(): DesiredView {
		const machines = this.store.listMachines();
		const inventories = this.inventories();
		const pending = this.pending();
		const byId = new Map(machines.map((m) => [m.id, m]));
		return {
			machines: machines.map((m) => ({
				id: m.id,
				name: m.name,
				status: this.hub.summary(m).status,
			})),
			items: this.store.listDesired().map((item) => {
				const cells: Record<number, DesiredCell> = {};
				for (const id of item.machineIds) {
					const machine = byId.get(id);
					if (machine)
						cells[id] = cellFor(
							item,
							machine,
							inventories.get(id) ?? null,
							pending.has(id),
						);
				}
				return { ...item, cells };
			}),
			files: this.store.listDesiredFiles().map((file): DesiredFileRow => {
				const cells: Record<number, DesiredCell> = {};
				for (const id of file.machineIds) {
					const machine = byId.get(id);
					if (machine)
						cells[id] = fileCellFor(
							file,
							machine,
							inventories.get(id) ?? null,
							pending.has(id),
						);
				}
				return { ...file, cells };
			}),
		};
	}

	/** Every machine's eligibility and current state for a path that may be adopted. */
	fileCandidates(path: string): FileCandidate[] {
		const inventories = this.inventories();
		return this.store.listMachines().map((machine) => {
			const inventory = inventories.get(machine.id) ?? null;
			const tracked = inventory?.files?.find((f) => f.path === path);
			return {
				machineId: machine.id,
				machineName: machine.name,
				blocked:
					fileBlockedReason(machine, inventory, path) ??
					(inventory ? null : "아직 수집되지 않았습니다"),
				kind: tracked?.kind ?? null,
				sha256: tracked?.sha256 ?? null,
				mode: tracked?.mode ?? null,
				linkTarget: tracked?.linkTarget ?? null,
			};
		});
	}

	/** Every machine's eligibility and current state for an item that may be adopted. */
	candidates(manager: DesiredManager, name: string): DesiredCandidate[] {
		const inventories = this.inventories();
		return this.store.listMachines().map((machine) => {
			const inventory = inventories.get(machine.id) ?? null;
			const state = inventory
				? installedState(manager, name, inventory)
				: { installed: false, version: null };
			return {
				machineId: machine.id,
				machineName: machine.name,
				blocked:
					blockedReason(manager, machine, inventory) ??
					(inventory ? null : "아직 수집되지 않았습니다"),
				installed: state.installed,
				version: state.version,
			};
		});
	}

	/** Machines with drifted items, and the script that would fix each. */
	plans(): MachinePlan[] {
		const view = this.view();
		const names = new Map(view.machines.map((m) => [m.id, m.name]));
		const steps = new Map<number, PlanStep[]>();
		const add = (id: string, step: PlanStep) => {
			const list = steps.get(Number(id)) ?? [];
			list.push(step);
			steps.set(Number(id), list);
		};
		for (const item of view.items) {
			for (const [id, cell] of Object.entries(item.cells)) {
				if (cell.status !== "drifted") continue;
				add(id, {
					kind: "package",
					itemId: item.id,
					manager: item.manager,
					name: item.name,
					version: item.version,
					action: item.ensure === "present" ? "install" : "remove",
				});
			}
		}
		for (const file of view.files) {
			for (const [id, cell] of Object.entries(file.cells)) {
				if (cell.status !== "drifted") continue;
				const write = file.ensure === "present";
				add(id, {
					kind: "file",
					fileId: file.id,
					path: file.path,
					action: write ? "write" : "delete",
					mode: file.mode,
					sha256: write ? (file.current?.sha256 ?? null) : null,
					size: write ? (file.current?.size ?? null) : null,
					secret: file.secret,
				});
			}
		}
		return [...steps].map(([machineId, list]) => ({
			machineId,
			machineName: names.get(machineId) ?? String(machineId),
			steps: list,
			command: planDisplay(list),
		}));
	}

	/**
	 * The script that carries out a confirmed plan, with the current content of every written
	 * file (secrets decrypted here). Throws FileVersionChanged when a file's current version no
	 * longer matches the version the plan was shown with.
	 */
	async scriptFor(plan: MachinePlan): Promise<string> {
		const contents = new Map<number, string>();
		for (const step of plan.steps) {
			if (step.kind !== "file" || step.action !== "write") continue;
			const current = this.store.getDesiredFile(step.fileId)?.current;
			const version =
				current &&
				this.store.getDesiredFileVersionContent(step.fileId, current.versionId);
			if (!version) throw new FileVersionChanged();
			const content =
				version.content ??
				(version.contentAge === null
					? null
					: await this.secrets.decrypt(version.contentAge));
			if (content === null || sha256Hex(content) !== step.sha256)
				throw new FileVersionChanged();
			contents.set(step.fileId, content);
		}
		return planScript(plan.steps, contents).script;
	}

	/** Whether a collected file is exactly what a desired file on that machine asks for. */
	matchesDesiredFile(machineId: number, file: TrackedFile): boolean {
		const desired = this.store
			.listDesiredFiles()
			.find((f) => f.path === file.path && f.machineIds.includes(machineId));
		if (!desired) return false;
		return desired.ensure === "absent"
			? file.kind === "missing"
			: file.kind === "file" && file.sha256 === desired.current?.sha256;
	}

	/** After an apply target finishes, re-collect so the state reflects the change. */
	afterApply(machine: MachineRow): void {
		this.recollecting.add(machine.id);
		void this.hub
			.collect(machine)
			.catch((e) => console.error(`re-collect of ${machine.name} failed:`, e))
			.finally(() => this.recollecting.delete(machine.id));
	}
}
