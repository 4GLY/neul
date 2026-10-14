import {
	ALERT_RULE_LABELS,
	type Alert,
	type AlertRule,
	type AlertRules,
	type Facts,
	type Inventory,
	type RunSummary,
	type TrackedFile,
} from "../shared/types.ts";
import type { AlertCondition, MachineRow, Store } from "./db.ts";
import type { Pusher } from "./push.ts";

const ALL_RULES: AlertRule[] = [
	"unreachable",
	"disk",
	"updates",
	"repo",
	"files",
];

export interface Evaluation {
	evaluated: AlertRule[]; // rules whose open alerts may be resolved by this evaluation
	active: AlertCondition[];
}

/**
 * Conditions that hold in one successful collection's facts. A rule that is off counts as
 * evaluated with nothing active, so turning it off resolves its open alerts.
 */
export function evaluateFacts(rules: AlertRules, facts: Facts): Evaluation {
	const evaluated: AlertRule[] = [];
	const active: AlertCondition[] = [];

	if (rules.diskPercent === null || facts.disk) evaluated.push("disk");
	if (rules.diskPercent !== null && facts.disk?.totalB) {
		const percent = (facts.disk.usedB / facts.disk.totalB) * 100;
		if (percent >= rules.diskPercent)
			active.push({
				rule: "disk",
				subject: facts.disk.path,
				message: `${facts.disk.path} 사용률 ${Math.floor(percent)}% (기준 ${rules.diskPercent}%)`,
			});
	}

	const { brew, pacman } = facts.updates;
	if (rules.updatesCount === null || brew || pacman) evaluated.push("updates");
	if (rules.updatesCount !== null && (brew || pacman)) {
		const count = (brew?.length ?? 0) + (pacman?.length ?? 0);
		if (count >= rules.updatesCount) {
			const parts = [
				brew ? `brew ${brew.length}` : null,
				pacman ? `pacman ${pacman.length}` : null,
			].filter(Boolean);
			active.push({
				rule: "updates",
				subject: "",
				message: `업데이트 대기 ${count}개 (${parts.join(", ")})`,
			});
		}
	}

	evaluated.push("repo");
	for (const repo of facts.repos ?? []) {
		if (repo.state !== "ok" || !rules.repos.includes(repo.path)) continue;
		const parts = [
			repo.dirty ? `커밋 안 한 변경 ${repo.dirty}개` : null,
			repo.ahead ? `push 안 한 커밋 ${repo.ahead}개` : null,
		].filter(Boolean);
		if (parts.length)
			active.push({
				rule: "repo",
				subject: repo.path,
				message: `${repo.path}: ${parts.join(", ")}`,
			});
	}
	return { evaluated, active };
}

/** No successful collection for `unreachableMin` minutes, measured from the last success. */
export function unreachableCondition(
	rules: AlertRules,
	machine: MachineRow,
	lastRun: RunSummary | null,
	lastSeenAt: string | null,
	now: number,
): AlertCondition | null {
	if (rules.unreachableMin === null || !machine.notify_unreachable) return null;
	if (!lastRun || lastRun.status === "ok") return null;
	const since = Date.parse(lastSeenAt ?? machine.created_at);
	if (now - since < rules.unreachableMin * 60_000) return null;
	return {
		rule: "unreachable",
		subject: "",
		message: `${rules.unreachableMin}분 넘게 수집 실패${lastRun.error ? `: ${lastRun.error}` : ""}`,
	};
}

/** Files present in both versions whose kind, content, or link target changed (current side). */
export function changedFiles(
	previous: TrackedFile[] | null,
	current: TrackedFile[] | null,
): TrackedFile[] {
	if (!previous || !current) return [];
	const before = new Map(previous.map((f) => [f.path, f]));
	return current.filter((f) => {
		const old = before.get(f.path);
		return (
			old &&
			(old.kind !== f.kind ||
				old.sha256 !== f.sha256 ||
				old.linkTarget !== f.linkTarget)
		);
	});
}

/** Whether a changed file is exactly what desired state asked for, so it is not an incident. */
export type ExpectedFile = (machineId: number, file: TrackedFile) => boolean;

/** Opens and resolves alerts from collection results and pushes newly opened ones (design.md §8 P2). */
export class Monitor {
	constructor(
		private readonly store: Store,
		private readonly pusher: Pusher,
		private readonly expected: ExpectedFile,
	) {}

	afterRun(
		machine: MachineRow,
		run: RunSummary,
		facts: Facts | null,
		previous: Inventory | null,
		current: Inventory | null,
	): void {
		if (!machine.enabled) return;
		const rules = this.store.getSettings().alertRules;
		const { evaluated, active } =
			run.status === "ok" && facts
				? evaluateFacts(rules, facts)
				: { evaluated: [], active: [] };
		const unreachable = unreachableCondition(
			rules,
			machine,
			run,
			this.store.lastOkRun(machine.id)?.finishedAt ?? null,
			Date.parse(run.finishedAt),
		);
		const opened = this.store.syncAlerts(
			machine.id,
			[...evaluated, "unreachable"],
			unreachable ? [...active, unreachable] : active,
			run.finishedAt,
		);
		if (rules.trackedFiles && run.inventoryChanged) {
			const paths = changedFiles(
				previous?.files ?? null,
				current?.files ?? null,
			)
				.filter((f) => !this.expected(machine.id, f))
				.map((f) => f.path);
			if (paths.length)
				opened.push(
					this.store.recordAlertEvent(
						machine.id,
						{ rule: "files", subject: "", message: paths.join(", ") },
						run.finishedAt,
					),
				);
		}
		this.notify(opened);
	}

	/** Periodic check: unreachable time keeps growing between collections; disabled machines are muted. */
	tick(now: number): void {
		const rules = this.store.getSettings().alertRules;
		const at = new Date(now).toISOString();
		const opened: Alert[] = [];
		for (const machine of this.store.listMachines()) {
			if (!machine.enabled) {
				this.store.syncAlerts(machine.id, ALL_RULES, [], at);
				continue;
			}
			const condition = unreachableCondition(
				rules,
				machine,
				this.store.lastRun(machine.id),
				this.store.lastOkRun(machine.id)?.finishedAt ?? null,
				now,
			);
			opened.push(
				...this.store.syncAlerts(
					machine.id,
					["unreachable"],
					condition ? [condition] : [],
					at,
				),
			);
		}
		this.notify(opened);
	}

	private notify(alerts: Alert[]): void {
		for (const alert of alerts) {
			this.pusher
				.send({
					title: `${alert.machineName} · ${ALERT_RULE_LABELS[alert.rule]}`,
					body: alert.message,
					url: "/#/alerts",
					tag: `${alert.machineId}:${alert.rule}:${alert.subject}`,
				})
				.then(({ sent }) => {
					if (sent)
						this.store.markAlertNotified(alert.id, new Date().toISOString());
				})
				.catch((e) => console.error("alert push failed:", e));
		}
	}
}
