import { describe, expect, test } from "bun:test";
import { buildCompare } from "../src/server/compare.ts";
import { Store } from "../src/server/db.ts";
import type { Inventory } from "../src/shared/types.ts";

function inventory(brew: Record<string, string>): Inventory {
	return {
		brewFormulae: Object.entries(brew).map(([name, v]) => ({
			name,
			versions: [v],
			onRequest: true,
		})),
		brewCasks: null,
		pacmanExplicit: null,
		pacmanTotal: null,
		mise: null,
		files: null,
	};
}

function run(
	store: Store,
	machineId: number,
	inv: Inventory | null,
	minute: number,
) {
	const at = new Date(Date.UTC(2026, 9, 14, 0, minute)).toISOString();
	return store.recordRun({
		machineId,
		startedAt: at,
		finishedAt: at,
		status: inv ? "ok" : "unreachable",
		error: inv ? null : "timeout",
		facts: null,
		inventory: inv,
	});
}

describe("Store.recordRun", () => {
	test("dedupes identical inventories and flags only real changes, including reverts", () => {
		const store = new Store(":memory:");
		const m = store.addMachine({
			name: "a",
			alias: "a",
			transport: "ssh",
			platform: "posix",
		});
		const a = inventory({ git: "1" });
		const b = inventory({ git: "2" });

		expect(run(store, m.id, a, 0).inventoryChanged).toBe(false); // first inventory is a baseline
		expect(run(store, m.id, a, 1).inventoryChanged).toBe(false);
		expect(run(store, m.id, null, 2).inventoryChanged).toBe(false); // failed run keeps the last inventory
		expect(store.latestInventory(m.id)?.inventory).toEqual(a);
		expect(run(store, m.id, b, 3).inventoryChanged).toBe(true);
		expect(run(store, m.id, a, 4).inventoryChanged).toBe(true); // revert to an earlier version
		expect(store.latestInventory(m.id)?.inventory).toEqual(a);
		expect(
			store.db.query("SELECT count(*) AS n FROM inventories").get(),
		).toEqual({ n: 2 });
	});

	test("deleting a machine removes its runs and inventories", () => {
		const store = new Store(":memory:");
		const m = store.addMachine({
			name: "a",
			alias: "a",
			transport: "ssh",
			platform: "posix",
		});
		run(store, m.id, inventory({ git: "1" }), 0);
		store.deleteMachine(m.id);
		expect(
			store.db
				.query(
					"SELECT (SELECT count(*) FROM runs) + (SELECT count(*) FROM inventories) AS n",
				)
				.get(),
		).toEqual({
			n: 0,
		});
	});
});

describe("buildCompare", () => {
	test("rows are uniform only when every reporting machine has the same value", () => {
		const view = buildCompare([
			{
				id: 1,
				name: "mac1",
				status: "online",
				inventory: inventory({ git: "2", jq: "1" }),
			},
			{
				id: 2,
				name: "mac2",
				status: "online",
				inventory: inventory({ git: "2" }),
			},
			{
				id: 3,
				name: "linux",
				status: "online",
				inventory: { ...inventory({}), brewFormulae: null },
			},
			{ id: 4, name: "never-collected", status: "unknown", inventory: null },
		]);
		const brew = view.sections.find((s) => s.id === "brewFormulae");
		expect(brew?.machineIds).toEqual([1, 2]); // machines without brew get no column
		expect(brew?.rows).toEqual([
			{ key: "git", cells: { 1: "2", 2: "2" }, uniform: true },
			{ key: "jq", cells: { 1: "1", 2: null }, uniform: false },
		]);
		expect(view.sections.map((s) => s.id)).toEqual(["brewFormulae"]);
		expect(view.machines).toHaveLength(4);
	});
});
