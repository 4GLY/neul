import { describe, expect, test } from "bun:test";
import {
	EXEC_RUNS_KEPT,
	type ExecTargetResult,
	type MachineRow,
	Store,
} from "../src/server/db.ts";
import { OutputBuffer } from "../src/server/exec.ts";

function setup(): { store: Store; a: MachineRow; b: MachineRow } {
	const store = new Store(":memory:");
	const add = (name: string) =>
		store.addMachine({
			name,
			alias: name,
			transport: "ssh",
			platform: "posix",
		});
	return { store, a: add("a"), b: add("b") };
}

function start(store: Store, machines: MachineRow[], command = "true") {
	const { run, targetIds } = store.createExecRun({
		command,
		timeoutS: 60,
		createdBy: "local",
		createdAt: "2026-10-14T00:00:00.000Z",
		machines,
	});
	return { id: run.id, targetIds: targetIds as [number, ...number[]] };
}

function result(
	status: ExecTargetResult["status"],
	at: string,
): ExecTargetResult {
	return {
		status,
		exitCode: status === "ok" ? 0 : 1,
		finishedAt: at,
		output: [{ stream: "stdout", data: `${status}\n` }],
		truncated: false,
	};
}

describe("Store exec runs", () => {
	test("a run finishes only when its last target does, and keeps history after machine deletion", () => {
		const { store, a, b } = setup();
		const {
			id,
			targetIds: [first, second],
		} = start(store, [a, b]);

		expect(
			store.finishExecTarget(
				id,
				first,
				result("ok", "2026-10-14T00:00:01.000Z"),
			),
		).toBeNull();
		expect(store.getExecRun(id)?.finishedAt).toBeNull();
		expect(
			store.finishExecTarget(
				id,
				second ?? -1,
				result("failed", "2026-10-14T00:00:02.000Z"),
			),
		).toBe("2026-10-14T00:00:02.000Z");

		store.deleteMachine(a.id);
		const after = store.getExecRun(id);
		expect(after?.finishedAt).toBe("2026-10-14T00:00:02.000Z");
		expect(
			after?.targets.map((t) => [
				t.machineId,
				t.machineName,
				t.status,
				t.output,
			]),
		).toEqual([
			[null, "a", "ok", [{ stream: "stdout", data: "ok\n" }]],
			[b.id, "b", "failed", [{ stream: "stdout", data: "failed\n" }]],
		]);
	});

	test("interruptExecRuns closes only targets still running", () => {
		const { store, a, b } = setup();
		const { id, targetIds } = start(store, [a, b], "sleep 9");
		store.finishExecTarget(
			id,
			targetIds[0],
			result("ok", "2026-10-14T00:00:01.000Z"),
		);

		expect(store.interruptExecRuns("2026-10-14T00:05:00.000Z")).toBe(1);
		const after = store.getExecRun(id);
		expect(after?.finishedAt).toBe("2026-10-14T00:05:00.000Z");
		expect(after?.targets.map((t) => [t.status, t.finishedAt])).toEqual([
			["ok", "2026-10-14T00:00:01.000Z"],
			["interrupted", "2026-10-14T00:05:00.000Z"],
		]);
		expect(store.interruptExecRuns("2026-10-14T00:06:00.000Z")).toBe(0);
	});

	test(`keeps only the newest ${EXEC_RUNS_KEPT} runs`, () => {
		const { store, a } = setup();
		const ids = Array.from(
			{ length: EXEC_RUNS_KEPT + 1 },
			(_, i) => start(store, [a], `echo ${i}`).id,
		);
		expect(store.getExecRun(ids[0] ?? -1)).toBeNull();
		expect(store.getExecRun(ids[1] ?? -1)).not.toBeNull();
		expect(
			store.db
				.query(
					"SELECT (SELECT count(*) FROM exec_runs) AS runs, (SELECT count(*) FROM exec_targets) AS targets",
				)
				.get(),
		).toEqual({ runs: EXEC_RUNS_KEPT, targets: EXEC_RUNS_KEPT });
	});
});

describe("OutputBuffer", () => {
	const bytes = (text: string) => new TextEncoder().encode(text);

	test("keeps exactly `limit` bytes across both streams and drops the rest", () => {
		const out = new OutputBuffer(6);
		expect(out.push("stdout", bytes("abc"))).toBe("abc");
		expect(out.push("stderr", bytes("de"))).toBe("de");
		expect(out.push("stdout", bytes("f"))).toBe("f");
		expect(out.truncated).toBe(false);
		expect(out.push("stdout", bytes("g"))).toBe("");
		expect(out.truncated).toBe(true);
		expect(out.push("stderr", bytes("h"))).toBe("");
		expect(out.chunks).toEqual([
			{ stream: "stdout", data: "abc" },
			{ stream: "stderr", data: "de" },
			{ stream: "stdout", data: "f" },
		]);
	});

	test("decodes a multi-byte character split across reads and merges one stream", () => {
		const out = new OutputBuffer();
		const encoded = bytes("한글");
		expect(out.push("stdout", encoded.subarray(0, 2))).toBe("");
		expect(out.push("stdout", encoded.subarray(2))).toBe("한글");
		expect(out.push("stdout", bytes("!"))).toBe("!");
		expect(out.finish()).toEqual([]);
		expect(out.chunks).toEqual([{ stream: "stdout", data: "한글!" }]);
	});
});
