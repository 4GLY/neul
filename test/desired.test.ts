import { describe, expect, test } from "bun:test";
import {
	lstatSync,
	mkdirSync,
	mkdtempSync,
	readdirSync,
	readFileSync,
	readlinkSync,
	statSync,
	symlinkSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type MachineRow, Store } from "../src/server/db.ts";
import {
	cellFor,
	fileCellFor,
	planScript,
	sha256Hex,
} from "../src/server/desired.ts";
import type {
	DesiredFile,
	DesiredItem,
	Inventory,
	MiseTool,
	PlanStep,
	TrackedFile,
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

describe("fileCellFor", () => {
	const file = (patch: Partial<DesiredFile> = {}): DesiredFile => ({
		id: 1,
		path: ".zshrc",
		ensure: "present",
		mode: "644",
		secret: false,
		machineIds: [1],
		createdAt: "2026-10-14T00:00:00.000Z",
		current: {
			versionId: 1,
			sha256: "aaa",
			size: 3,
			createdAt: "2026-10-14T00:00:00.000Z",
		},
		...patch,
	});
	const tracked = (patch: Partial<TrackedFile>): Inventory =>
		inventory({
			files: [
				{
					path: ".zshrc",
					kind: "file",
					sha256: "aaa",
					size: 3,
					mode: "644",
					linkTarget: null,
					...patch,
				},
			],
		});

	test("never applies through a symlink", () => {
		const cell = fileCellFor(
			file(),
			machine(),
			tracked({ kind: "symlink", linkTarget: "repos/dotfiles/.zshrc" }),
			false,
		);
		expect(cell.status).toBe("blocked");
		expect(cell.detail).toContain("repos/dotfiles/.zshrc");
	});

	test("absent is in sync only when the file is missing", () => {
		const absent = file({ ensure: "absent", current: null });
		const missing = tracked({ kind: "missing", sha256: null, mode: null });
		expect(fileCellFor(absent, machine(), missing, false).status).toBe(
			"in_sync",
		);
		expect(fileCellFor(absent, machine(), tracked({}), false).status).toBe(
			"drifted",
		);
	});

	test("content and mode differences drift; unknown mode is not compared", () => {
		expect(
			fileCellFor(file(), machine(), tracked({ sha256: "bbb" }), false),
		).toEqual({ status: "drifted", detail: "내용 다름" });
		expect(
			fileCellFor(file(), machine(), tracked({ mode: "600" }), false),
		).toEqual({ status: "drifted", detail: "권한 600 → 644" });
		expect(
			fileCellFor(file(), machine(), tracked({ mode: null }), false).status,
		).toBe("in_sync");
	});
});

describe("planScript", () => {
	const packages: PlanStep[] = [
		{
			kind: "package",
			itemId: 1,
			manager: "mise",
			name: "jq",
			version: "latest",
			action: "install",
		},
		{
			kind: "package",
			itemId: 2,
			manager: "brew_formula",
			name: "sl",
			version: null,
			action: "remove",
		},
		{
			kind: "package",
			itemId: 3,
			manager: "pacman",
			name: "cowsay",
			version: null,
			action: "install",
		},
	];

	async function run(script: string, home: string, prelude = "") {
		const proc = Bun.spawn(["sh", "-c", `${prelude}\n${script}`], {
			env: { ...process.env, HOME: home },
			stdout: "ignore",
			stderr: "pipe",
		});
		return proc.exited;
	}

	test("runs removals first, keeps going after a failure, and exits non-zero", async () => {
		const home = mkdtempSync(join(tmpdir(), "neul-plan-"));
		// Stub the package managers: log each call; brew fails.
		const stubs = [
			'brew() { echo "brew $*" >> "$HOME/log"; return 1; }',
			'sudo() { echo "sudo $*" >> "$HOME/log"; }',
			'mise() { echo "mise $*" >> "$HOME/log"; }',
		].join("\n");
		const { display, script } = planScript(packages, new Map());
		expect(display).toBe(script);
		expect(await run(script, home, stubs)).toBe(1);
		expect(readFileSync(join(home, "log"), "utf8").trim().split("\n")).toEqual([
			"brew uninstall --formula sl",
			"sudo -n pacman -S --needed --noconfirm cowsay",
			"sudo -n pacman -D --asexplicit cowsay",
			"mise use -g -y jq@latest",
		]);
	});

	const write = (
		fileId: number,
		path: string,
		content: string,
		patch: Partial<PlanStep> = {},
	): PlanStep =>
		({
			kind: "file",
			fileId,
			path,
			action: "write",
			mode: "600",
			sha256: sha256Hex(content),
			size: Buffer.byteLength(content),
			secret: false,
			...patch,
		}) as PlanStep;

	test("writes atomically with the mode, backing up what was there", async () => {
		const home = mkdtempSync(join(tmpdir(), "neul-plan-"));
		writeFileSync(join(home, ".existing"), "old\n");
		const contents = new Map([
			[1, "new ✓\n"],
			[2, "key\n"],
		]);
		const { script } = planScript(
			[write(1, ".existing", "new ✓\n"), write(2, ".ssh/dir/key", "key\n")],
			contents,
		);
		expect(await run(script, home)).toBe(0);
		expect(readFileSync(join(home, ".existing"), "utf8")).toBe("new ✓\n");
		expect(statSync(join(home, ".existing")).mode & 0o777).toBe(0o600);
		const backups = readdirSync(home).filter((n) =>
			n.startsWith(".existing.neul-bak-"),
		);
		expect(backups).toHaveLength(1);
		expect(readFileSync(join(home, backups[0] as string), "utf8")).toBe(
			"old\n",
		);
		expect(readFileSync(join(home, ".ssh/dir/key"), "utf8")).toBe("key\n");
		expect(readdirSync(join(home, ".ssh/dir"))).toEqual(["key"]);
	});

	test("refuses to write through a symlink", async () => {
		const home = mkdtempSync(join(tmpdir(), "neul-plan-"));
		mkdirSync(join(home, "repo"));
		writeFileSync(join(home, "repo/.zshrc"), "stow\n");
		symlinkSync("repo/.zshrc", join(home, ".zshrc"));
		const { script } = planScript(
			[write(1, ".zshrc", "x\n")],
			new Map([[1, "x\n"]]),
		);
		expect(await run(script, home)).toBe(1);
		expect(lstatSync(join(home, ".zshrc")).isSymbolicLink()).toBe(true);
		expect(readlinkSync(join(home, ".zshrc"))).toBe("repo/.zshrc");
		expect(readFileSync(join(home, "repo/.zshrc"), "utf8")).toBe("stow\n");
	});

	test("delete moves the file to a backup", async () => {
		const home = mkdtempSync(join(tmpdir(), "neul-plan-"));
		writeFileSync(join(home, ".gone"), "bye\n");
		const { script } = planScript(
			[write(1, ".gone", "", { action: "delete", sha256: null, size: null })],
			new Map(),
		);
		expect(await run(script, home)).toBe(0);
		const names = readdirSync(home);
		expect(names).not.toContain(".gone");
		const backup = names.find((n) => n.startsWith(".gone.neul-bak-"));
		expect(readFileSync(join(home, backup as string), "utf8")).toBe("bye\n");
	});

	test("only the script carries file contents", () => {
		const secret = "not-a-real-key-7f3a\n";
		const encoded = Buffer.from(secret).toString("base64");
		const { display, script } = planScript(
			[write(1, ".ssh/id", secret, { secret: true })],
			new Map([[1, secret]]),
		);
		expect(display).not.toContain("not-a-real-key");
		expect(display).not.toContain(encoded);
		expect(script).toContain(encoded);
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
