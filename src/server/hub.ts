import { parseSections, toFacts, toInventory } from "../collector/parse.ts";
import { processStatus, runCollector } from "../collector/run.ts";
import type {
	Facts,
	Inventory,
	InventoryCounts,
	MachineDetail,
	MachineStatus,
	MachineSummary,
	RunSummary,
} from "../shared/types.ts";
import type { MachineRow, Store } from "./db.ts";

const STATUS_BY_RUN: Record<RunSummary["status"], MachineStatus> = {
	ok: "online",
	unreachable: "unreachable",
	error: "error",
};

function counts(inv: Inventory): InventoryCounts {
	return {
		brewFormulae: inv.brewFormulae?.length ?? null,
		brewLeaves: inv.brewFormulae?.filter((p) => p.onRequest).length ?? null,
		brewCasks: inv.brewCasks?.length ?? null,
		pacmanExplicit: inv.pacmanExplicit?.length ?? null,
		pacmanTotal: inv.pacmanTotal,
		miseTools: inv.mise?.filter((t) => t.activeVersion !== null).length ?? null,
		files: inv.files?.filter((f) => f.kind !== "missing").length ?? null,
	};
}

/** Called after every recorded collection with the inventory before and after it. */
export type RunListener = (
	machine: MachineRow,
	run: RunSummary,
	facts: Facts | null,
	previous: Inventory | null,
	current: Inventory | null,
) => void;

export class Hub {
	private readonly inFlight = new Map<number, Promise<RunSummary>>();

	constructor(
		readonly store: Store,
		private readonly onRun: RunListener,
	) {}

	isCollecting(machineId: number): boolean {
		return this.inFlight.has(machineId);
	}

	/** Collect one machine; concurrent callers share the in-flight run. */
	collect(machine: MachineRow): Promise<RunSummary> {
		const running = this.inFlight.get(machine.id);
		if (running) return running;
		const promise = this.collectOnce(machine).finally(() =>
			this.inFlight.delete(machine.id),
		);
		this.inFlight.set(machine.id, promise);
		return promise;
	}

	private async collectOnce(machine: MachineRow): Promise<RunSummary> {
		const startedAt = new Date().toISOString();
		let status: RunSummary["status"] = "ok";
		let error: string | null = null;
		let facts: Facts | null = null;
		let inventory: Inventory | null = null;
		try {
			const settings = this.store.getSettings();
			const result = await runCollector(
				machine,
				settings.trackedPaths,
				settings.alertRules.repos,
			);
			const failure = processStatus(machine, result);
			if (failure) {
				({ status, error } = failure);
			} else {
				const parsed = parseSections(result.stdout);
				if (!parsed.complete)
					throw new Error("수집 출력이 중간에 끊겼습니다 (@@ end 없음)");
				facts = toFacts(parsed.sections);
				inventory = toInventory(parsed.sections);
			}
		} catch (e) {
			status = "error";
			error = e instanceof Error ? e.message : String(e);
		}
		const previous = this.store.latestInventory(machine.id)?.inventory ?? null;
		const run = this.store.recordRun({
			machineId: machine.id,
			startedAt,
			finishedAt: new Date().toISOString(),
			status,
			error,
			facts,
			inventory,
		});
		try {
			this.onRun(machine, run, facts, previous, inventory);
		} catch (e) {
			console.error(`alert evaluation for ${machine.name} failed:`, e);
		}
		return run;
	}

	collectAll(): Promise<RunSummary[]> {
		return Promise.all(
			this.store
				.listMachines()
				.filter((m) => m.enabled)
				.map((m) => this.collect(m)),
		);
	}

	summary(machine: MachineRow): MachineSummary {
		const lastRun = this.store.lastRun(machine.id);
		const lastOk = this.store.lastOkRun(machine.id);
		const latest = this.store.latestInventory(machine.id);
		return {
			id: machine.id,
			name: machine.name,
			alias: machine.alias,
			transport: machine.transport,
			platform: machine.platform,
			enabled: machine.enabled === 1,
			notifyUnreachable: machine.notify_unreachable === 1,
			status: lastRun ? STATUS_BY_RUN[lastRun.status] : "unknown",
			collecting: this.isCollecting(machine.id),
			lastRun,
			lastSeenAt: lastOk?.finishedAt ?? null,
			facts: lastOk?.facts ?? null,
			counts: latest ? counts(latest.inventory) : null,
		};
	}

	detail(machine: MachineRow): MachineDetail {
		const latest = this.store.latestInventory(machine.id);
		return {
			...this.summary(machine),
			inventory: latest?.inventory ?? null,
			inventorySince: latest?.since ?? null,
			recentRuns: this.store.recentRuns(machine.id, 50),
		};
	}
}
