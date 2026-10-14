import { describe, expect, test } from "bun:test";
import { type MachineRow, Store } from "../src/server/db.ts";
import { cellFor, planCommand } from "../src/server/desired.ts";
import type {
	DesiredItem,
	Inventory,
	MiseTool,
	PlanStep,
} from "../src/shared/types.ts";

const GLOBAL = "/home/u/.config/mise/config.toml";

function inventory(patch: Partial<Inventory>): Inventory {
	return {
		brewFormulae: null,
		brewCasks: null,
		pacmanExplicit: null,
		pacmanTotal: null,
		pacmanSudo: null,
		mise: null,
		files: null,
		...patch,
	};
}

function tool(patch: Partial<MiseTool>): MiseTool {
	return {
		name: "jq",
		activeVersion: "1.7.1",
		requestedVersion: "1.7.1",
		installedVersions: ["1.7.1"],
		source: GLOBAL,
		...patch,
	};
}

const machine = (patch: Partial<MachineRow> = {}): MachineRow => ({
	id: 1,
	name: "m",
	alias: "m",
	transport: "ssh",
	platform: "posix",
	enabled: 1,
	notify_unreachable: 1,
	created_at: "2026-10-14T00:00:00.000Z",
	...patch,
});

const item = (patch: Partial<DesiredItem>): DesiredItem => ({
	id: 1,
	manager: "mise",
	name: "jq",
	version: "1.7.1",
	ensure: "present",
	machineIds: [1],
	createdAt: "2026-10-14T00:00:00.000Z",
	...patch,
});

describe("cellFor", () => {
	const status = (
		i: DesiredItem,
		inv: Inventory | null,
		m = machine(),
		pending = false,
	) => cellFor(i, m, inv, pending).status;

	test("mise is in sync only when the global config requests that version and it is installed", () => {
		const mise = (t: MiseTool) => inventory({ mise: [t] });
		expect(status(item({}), mise(tool({})))).toBe("in_sync");
		expect(
			status(item({}), mise(tool({ source: "/home/u/proj/mise.toml" }))),
		).toBe("drifted");
		expect(
			status(
				item({}),
				mise(
					tool({
						requestedVersion: "latest",
						activeVersion: "1.8.2",
						installedVersions: ["1.8.2"],
					}),
				),
			),
		).toBe("drifted");
		expect(status(item({}), mise(tool({ installedVersions: [] })))).toBe(
			"drifted",
		);
		expect(status(item({ ensure: "absent" }), mise(tool({})))).toBe("drifted");
		expect(status(item({ ensure: "absent" }), inventory({ mise: [] }))).toBe(
			"in_sync",
		);
	});

	test("pacman counts only explicitly installed packages and needs non-interactive sudo", () => {
		const cowsay = item({ manager: "pacman", name: "cowsay", version: null });
		const pacman = (sudo: boolean) =>
			inventory({
				pacmanExplicit: [{ name: "cowsay", version: "3.8.4-1" }],
				pacmanSudo: sudo,
			});
		expect(status(cowsay, pacman(true))).toBe("in_sync");
		expect(status(cowsay, pacman(false))).toBe("blocked");
		expect(
			status(cowsay, inventory({ pacmanExplicit: [], pacmanSudo: true })),
		).toBe("drifted");
	});

	test("pending wins over everything; Windows is blocked; no inventory is unknown", () => {
		const brew = item({ manager: "brew_formula", name: "git", version: null });
		expect(status(brew, null, machine(), true)).toBe("pending");
		expect(status(brew, null, machine({ platform: "windows" }))).toBe(
			"blocked",
		);
		expect(status(brew, null)).toBe("unknown");
		expect(status(brew, inventory({}))).toBe("blocked"); // brew not installed
	});
});

describe("planCommand", () => {
	const steps: PlanStep[] = [
		{
			itemId: 1,
			manager: "mise",
			name: "jq",
			version: "latest",
			action: "install",
		},
		{
			itemId: 2,
			manager: "brew_formula",
			name: "sl",
			version: null,
			action: "remove",
		},
		{
			itemId: 3,
			manager: "pacman",
			name: "cowsay",
			version: null,
			action: "install",
		},
	];

	test("runs removals first, keeps going after a failure, and exits non-zero", async () => {
		// Stub the package managers: log each call; brew fails.
		const stubs = [
			'brew() { echo "brew $*" >> "$LOG"; return 1; }',
			'sudo() { echo "sudo $*" >> "$LOG"; }',
			'mise() { echo "mise $*" >> "$LOG"; }',
		].join("\n");
		const log = `${process.env.TMPDIR ?? "/tmp"}/neul-plan-${process.pid}.log`;
		const proc = Bun.spawn(["sh", "-c", `${stubs}\n${planCommand(steps)}`], {
			env: { ...process.env, LOG: log },
			stdout: "ignore",
		});
		expect(await proc.exited).toBe(1);
		const calls = (await Bun.file(log).text()).trim().split("\n");
		await Bun.file(log).delete();
		expect(calls).toEqual([
			"brew uninstall --formula sl",
			"sudo -n pacman -S --needed --noconfirm cowsay",
			"sudo -n pacman -D --asexplicit cowsay",
			"mise use -g -y jq@latest",
		]);
	});
});

describe("Store desired items", () => {
	test("deleting a machine drops it from targets; names are unique per manager", () => {
		const store = new Store(":memory:");
		const add = (name: string) =>
			store.addMachine({
				name,
				alias: name,
				transport: "ssh",
				platform: "posix",
			});
		const a = add("a");
		const b = add("b");
		const created = store.addDesired({
			manager: "brew_formula",
			name: "git",
			version: null,
			ensure: "present",
			machineIds: [a.id, b.id],
		});
		store.deleteMachine(a.id);
		expect(store.getDesired(created.id)?.machineIds).toEqual([b.id]);
		expect(() =>
			store.addDesired({
				manager: "brew_formula",
				name: "git",
				version: null,
				ensure: "absent",
				machineIds: [b.id],
			}),
		).toThrow();
		expect(
			store.addDesired({
				manager: "pacman",
				name: "git",
				version: null,
				ensure: "present",
				machineIds: [b.id],
			}).name,
		).toBe("git");
	});
});
