import { describe, expect, test } from "bun:test";
import { parseSections, toFacts, toInventory } from "../src/collector/parse.ts";

const OUTPUT = [
	"@@ meta",
	"schema=1",
	"hostname=box",
	"os=Darwin",
	"os_name=macOS 26.1",
	"cpu_count=10",
	"uptime_s=3600",
	"load=1.5 2.25 3",
	"mem_total_b=17179869184",
	"model=",
	"disk_path=/System/Volumes/Data",
	"disk_total_b=1000",
	"disk_used_b=400",
	"disk_available_b=600",
	"battery=80 AC attached",
	"@@ brew_formulae",
	"git 2.51.0",
	"python@3.13 3.13.15 3.13.16",
	"@@ brew_leaves",
	"git",
	"@@ brew_casks",
	"ghostty 1.2.0",
	"@@ mise_json",
	JSON.stringify({
		node: [
			{ version: "22.1.0", installed: true, active: false },
			{
				version: "24.17.0",
				requested_version: "24",
				installed: true,
				active: true,
				source: { path: "~/.config/mise/config.toml" },
			},
		],
		actionlint: [{ version: "1.7.7", installed: true, active: false }],
	}),
	"@@ files",
	"file\tabc123\t42\t\t.ssh/config",
	"symlink\tdef456\t10\trepos/dotfiles/.zshrc\t.zshrc",
	"missing\t\t\t\t.config/*.toml",
	"file\t0f0f\t3\t\tdir with\ttab/x",
	"bogus\tline",
	"@@ end",
].join("\n");

describe("parseSections", () => {
	test("detects a complete run and splits sections", () => {
		const parsed = parseSections(OUTPUT);
		expect(parsed.complete).toBe(true);
		expect(parsed.sections.get("brew_leaves")).toEqual(["git"]);
	});

	test("output without the terminator is incomplete", () => {
		expect(parseSections("@@ meta\nhostname=box\n").complete).toBe(false);
	});

	test("tolerates CRLF line endings from Windows", () => {
		const parsed = parseSections("@@ meta\r\nhostname=win\r\n@@ end\r\n");
		expect(parsed.complete).toBe(true);
		expect(toFacts(parsed.sections).hostname).toBe("win");
	});
});

describe("toFacts", () => {
	const facts = toFacts(parseSections(OUTPUT).sections);

	test("parses numbers, load, disk and battery", () => {
		expect(facts.cpuCount).toBe(10);
		expect(facts.load).toEqual([1.5, 2.25, 3]);
		expect(facts.disk).toEqual({
			path: "/System/Volumes/Data",
			totalB: 1000,
			usedB: 400,
			availableB: 600,
		});
		expect(facts.battery).toEqual({ percent: 80, state: "AC attached" });
	});

	test("empty values become null rather than empty strings", () => {
		expect(facts.model).toBeNull();
		expect(facts.memAvailableB).toBeNull();
	});
});

describe("toInventory", () => {
	const inv = toInventory(parseSections(OUTPUT).sections);

	test("brew formulae keep every installed version and mark leaves", () => {
		expect(inv.brewFormulae).toEqual([
			{ name: "git", versions: ["2.51.0"], onRequest: true },
			{
				name: "python@3.13",
				versions: ["3.13.15", "3.13.16"],
				onRequest: false,
			},
		]);
	});

	test("mise reports the active version and its source; inactive tools have none", () => {
		expect(inv.mise).toEqual([
			{
				name: "actionlint",
				activeVersion: null,
				requestedVersion: null,
				installedVersions: ["1.7.7"],
				source: null,
			},
			{
				name: "node",
				activeVersion: "24.17.0",
				requestedVersion: "24",
				installedVersions: ["22.1.0", "24.17.0"],
				source: "~/.config/mise/config.toml",
			},
		]);
	});

	test("files keep tabs inside paths and drop unknown kinds", () => {
		expect(inv.files?.map((f) => [f.kind, f.path])).toEqual([
			["missing", ".config/*.toml"],
			["file", ".ssh/config"],
			["symlink", ".zshrc"],
			["file", "dir with\ttab/x"],
		]);
		expect(inv.files?.find((f) => f.path === ".zshrc")?.linkTarget).toBe(
			"repos/dotfiles/.zshrc",
		);
	});

	test("absent tools are null, not empty", () => {
		expect(inv.pacmanExplicit).toBeNull();
		expect(inv.pacmanTotal).toBeNull();
	});

	test("malformed mise JSON fails the run instead of reporting no tools", () => {
		const parsed = parseSections("@@ mise_json\n{not json\n@@ end");
		expect(() => toInventory(parsed.sections)).toThrow("mise");
	});
});
