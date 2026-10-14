import type {
	DesiredCandidate,
	DesiredCell,
	DesiredItem,
	DesiredManager,
	DesiredView,
	Inventory,
	MachinePlan,
	MiseTool,
	PlanStep,
} from "../shared/types.ts";
import type { MachineRow, Store } from "./db.ts";
import type { Executor } from "./exec.ts";
import type { Hub } from "./hub.ts";

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

const quote = (value: string) => `'${value.replaceAll("'", `'\\''`)}'`;

/** The shell commands for one group of steps sharing a manager and an action. */
function commandsFor(
	manager: DesiredManager,
	action: PlanStep["action"],
	steps: PlanStep[],
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
 * POSIX script for one machine: removals first, then installs; every command runs even if an
 * earlier one fails, and the exit code is non-zero if any failed.
 */
export function planCommand(steps: PlanStep[]): string {
	const lines: string[] = [];
	if (steps.some((s) => s.manager.startsWith("brew_")))
		lines.push("export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ENV_HINTS=1");
	lines.push("neul_rc=0");
	for (const action of ["remove", "install"] as const) {
		for (const manager of MANAGER_ORDER) {
			const group = steps.filter(
				(s) => s.manager === manager && s.action === action,
			);
			if (group.length === 0) continue;
			for (const command of commandsFor(manager, action, group)) {
				lines.push(`printf '%s\\n' ${quote(`$ ${command}`)}`);
				lines.push(`${command} || neul_rc=1`);
			}
		}
	}
	lines.push("exit $neul_rc");
	return lines.join("\n");
}

/** Desired-state views, plans, and the pending state between an apply and its re-collection. */
export class Desired {
	private readonly recollecting = new Set<number>();

	constructor(
		private readonly store: Store,
		private readonly hub: Hub,
		private readonly executor: Executor,
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
		};
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
		for (const item of view.items) {
			for (const [id, cell] of Object.entries(item.cells)) {
				if (cell.status !== "drifted") continue;
				const list = steps.get(Number(id)) ?? [];
				list.push({
					itemId: item.id,
					manager: item.manager,
					name: item.name,
					version: item.version,
					action: item.ensure === "present" ? "install" : "remove",
				});
				steps.set(Number(id), list);
			}
		}
		return [...steps].map(([machineId, list]) => ({
			machineId,
			machineName: names.get(machineId) ?? String(machineId),
			steps: list,
			command: planCommand(list),
		}));
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
