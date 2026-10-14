import { describe, expect, test } from "bun:test";
import { DEFAULT_SETTINGS, type MachineRow, Store } from "../src/server/db.ts";
import {
	changedFiles,
	evaluateFacts,
	unreachableCondition,
} from "../src/server/monitor.ts";
import type {
	AlertRules,
	Facts,
	RunSummary,
	TrackedFile,
} from "../src/shared/types.ts";

const RULES: AlertRules = {
	unreachableMin: 30,
	diskPercent: 90,
	updatesCount: 3,
	trackedFiles: true,
	repos: ["repos/a"],
};

function facts(patch: Partial<Facts>): Facts {
	return {
		hostname: null,
		os: null,
		osName: null,
		kernel: null,
		arch: null,
		model: null,
		user: null,
		cpuCount: null,
		uptimeS: null,
		load: null,
		memTotalB: null,
		memAvailableB: null,
		disk: null,
		battery: null,
		updates: { brew: null, pacman: null },
		repos: null,
		...patch,
	};
}

function machine(store: Store): MachineRow {
	return store.addMachine({
		name: "a",
		alias: "a",
		transport: "ssh",
		platform: "posix",
	});
}

const disk = (usedB: number) => ({
	path: "/",
	totalB: 100,
	usedB,
	availableB: 100 - usedB,
});

describe("Store.syncAlerts", () => {
	test("opens once, updates the message, resolves, and reopens as a new alert", () => {
		const store = new Store(":memory:");
		const m = machine(store);
		const full = { rule: "disk", subject: "/", message: "91%" } as const;

		expect(store.syncAlerts(m.id, ["disk"], [full], "t1")).toHaveLength(1);
		expect(
			store.syncAlerts(m.id, ["disk"], [{ ...full, message: "95%" }], "t2"),
		).toEqual([]);
		expect(store.listAlerts(false).map((a) => a.message)).toEqual(["95%"]);

		// A rule that could not be evaluated keeps its alert open.
		store.syncAlerts(m.id, ["updates"], [], "t3");
		expect(store.listAlerts(false)).toHaveLength(1);

		store.syncAlerts(m.id, ["disk"], [], "t4");
		expect(store.listAlerts(false)).toEqual([]);
		const [reopened] = store.syncAlerts(m.id, ["disk"], [full], "t5");
		expect(store.listAlerts(true).map((a) => [a.id, a.resolvedAt])).toEqual([
			[reopened?.id, null],
			[expect.any(Number), "t4"],
		]);
	});

	test("settings never expose the stored VAPID keys", () => {
		const store = new Store(":memory:");
		store.putVapidKeys({ publicKey: "pub", privateKey: "secret" });
		store.putSettings(DEFAULT_SETTINGS);
		expect(JSON.stringify(store.getSettings())).not.toContain("secret");
	});
});

describe("evaluateFacts", () => {
	test("disk fires at the threshold, not below", () => {
		expect(evaluateFacts(RULES, facts({ disk: disk(89) })).active).toEqual([]);
		expect(
			evaluateFacts(RULES, facts({ disk: disk(90) })).active.map((c) => c.rule),
		).toEqual(["disk"]);
	});

	test("unknown update counts leave the rule unevaluated; zero known updates resolve it", () => {
		expect(evaluateFacts(RULES, facts({})).evaluated).not.toContain("updates");
		const none = evaluateFacts(
			RULES,
			facts({ updates: { brew: [], pacman: null } }),
		);
		expect(none.evaluated).toContain("updates");
		expect(none.active).toEqual([]);
		expect(
			evaluateFacts(
				RULES,
				facts({ updates: { brew: ["git"], pacman: ["a", "b"] } }),
			).active.map((c) => c.message),
		).toEqual(["업데이트 대기 3개 (brew 1, pacman 2)"]);
	});

	test("a rule that is off is evaluated with nothing active", () => {
		const off = evaluateFacts(
			{ ...RULES, diskPercent: null },
			facts({ disk: disk(99) }),
		);
		expect(off.evaluated).toContain("disk");
		expect(off.active).toEqual([]);
	});

	test("repos alert only for watched repos with uncommitted or unpushed work", () => {
		const result = evaluateFacts(
			RULES,
			facts({
				repos: [
					{ path: "repos/a", state: "ok", dirty: 0, ahead: 1 },
					{ path: "repos/b", state: "ok", dirty: 5, ahead: null },
				],
			}),
		);
		expect(result.active.map((c) => [c.subject, c.message])).toEqual([
			["repos/a", "repos/a: push 안 한 커밋 1개"],
		]);
	});
});

describe("unreachableCondition", () => {
	const store = new Store(":memory:");
	const m = { ...machine(store), created_at: "2026-10-14T00:00:00.000Z" };
	const failed: RunSummary = {
		id: 1,
		machineId: m.id,
		startedAt: "2026-10-14T00:00:00.000Z",
		finishedAt: "2026-10-14T00:00:00.000Z",
		durationMs: 0,
		status: "unreachable",
		error: "timeout",
		inventoryChanged: false,
	};
	const lastSeen = "2026-10-14T00:00:00.000Z";
	const at = (min: number) => Date.parse(lastSeen) + min * 60_000;

	test("fires once the last success is unreachableMin old", () => {
		expect(unreachableCondition(RULES, m, failed, lastSeen, at(29))).toBeNull();
		expect(
			unreachableCondition(RULES, m, failed, lastSeen, at(30))?.message,
		).toBe("30분 넘게 수집 실패: timeout");
	});

	test("a successful last run or a muted machine never fires", () => {
		expect(
			unreachableCondition(
				RULES,
				m,
				{ ...failed, status: "ok" },
				lastSeen,
				at(60),
			),
		).toBeNull();
		expect(
			unreachableCondition(
				RULES,
				{ ...m, notify_unreachable: 0 },
				failed,
				lastSeen,
				at(60),
			),
		).toBeNull();
	});
});

describe("changedFiles", () => {
	const file = (path: string, sha256: string | null): TrackedFile => ({
		path,
		kind: sha256 ? "file" : "missing",
		sha256,
		size: null,
		linkTarget: null,
	});

	test("reports paths present in both versions whose content or kind changed", () => {
		expect(
			changedFiles(
				[file(".zshrc", "a"), file(".gitconfig", "b"), file(".npmrc", null)],
				[
					file(".zshrc", "a2"),
					file(".gitconfig", "b"),
					file(".npmrc", "c"),
					file(".new", "d"),
				],
			).map((f) => f.path),
		).toEqual([".zshrc", ".npmrc"]);
	});
});
