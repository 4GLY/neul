import type {
	BrewPackage,
	Facts,
	FileKind,
	Inventory,
	MiseTool,
	PacmanPackage,
	RepoStatus,
	TrackedFile,
} from "../shared/types.ts";

export interface ParsedOutput {
	sections: Map<string, string[]>;
	complete: boolean; // saw the `@@ end` terminator
}

/** Split collector stdout into `@@ <section>` blocks (design.md §4.1). */
export function parseSections(stdout: string): ParsedOutput {
	const sections = new Map<string, string[]>();
	let current: string[] | null = null;
	let complete = false;
	for (const raw of stdout.split("\n")) {
		const line = raw.replace(/\r$/, "");
		if (line.startsWith("@@ ")) {
			const name = line.slice(3).trim();
			if (name === "end") {
				complete = true;
				current = null;
				continue;
			}
			current = [];
			sections.set(name, current);
			continue;
		}
		if (current && line !== "") current.push(line);
	}
	return { sections, complete };
}

function keyValues(lines: string[] | undefined): Map<string, string> {
	const out = new Map<string, string>();
	for (const line of lines ?? []) {
		const eq = line.indexOf("=");
		if (eq <= 0) continue;
		const value = line.slice(eq + 1).trim();
		if (value !== "") out.set(line.slice(0, eq), value);
	}
	return out;
}

function num(value: string | undefined): number | null {
	if (value === undefined) return null;
	const n = Number(value);
	return Number.isFinite(n) ? n : null;
}

export function toFacts(sections: Map<string, string[]>): Facts {
	const m = keyValues(sections.get("meta"));
	const load = m.get("load")?.split(/\s+/).map(Number);
	const diskTotal = num(m.get("disk_total_b"));
	const battery = m.get("battery")?.match(/^(\d+)\s*(.*)$/);
	return {
		hostname: m.get("hostname") ?? null,
		os: m.get("os") ?? null,
		osName: m.get("os_name") ?? null,
		kernel: m.get("kernel") ?? null,
		arch: m.get("arch") ?? null,
		model: m.get("model") ?? null,
		user: m.get("user") ?? null,
		cpuCount: num(m.get("cpu_count")),
		uptimeS: num(m.get("uptime_s")),
		load:
			load && load.length === 3 && load.every(Number.isFinite)
				? [load[0] as number, load[1] as number, load[2] as number]
				: null,
		memTotalB: num(m.get("mem_total_b")),
		memAvailableB: num(m.get("mem_available_b")),
		disk:
			diskTotal === null
				? null
				: {
						path: m.get("disk_path") ?? "/",
						totalB: diskTotal,
						usedB: num(m.get("disk_used_b")) ?? 0,
						availableB: num(m.get("disk_available_b")) ?? 0,
					},
		battery: battery
			? { percent: Number(battery[1]), state: (battery[2] ?? "").trim() }
			: null,
		updates: {
			brew: sections.get("brew_outdated") ?? null,
			pacman: sections.get("pacman_updates") ?? null,
		},
		repos: repoStatuses(sections.get("repos")),
	};
}

function repoStatuses(lines: string[] | undefined): RepoStatus[] | null {
	if (!lines) return null;
	return lines.flatMap((line): RepoStatus[] => {
		const [state, dirty, ahead, ...path] = line.split("\t");
		if ((state !== "ok" && state !== "missing") || path.length === 0) return [];
		return [
			{
				path: path.join("\t"),
				state,
				dirty: dirty ? num(dirty) : null,
				ahead: ahead ? num(ahead) : null,
			},
		];
	});
}

function byName<T extends { name: string }>(a: T, b: T): number {
	return a.name < b.name ? -1 : a.name > b.name ? 1 : 0;
}

function brewList(
	lines: string[] | undefined,
	onRequest: (name: string) => boolean,
): BrewPackage[] | null {
	if (lines === undefined) return null;
	return lines
		.map((line) => line.trim().split(/\s+/))
		.filter((parts) => parts[0])
		.map(([name, ...versions]) => ({
			name: name as string,
			versions,
			onRequest: onRequest(name as string),
		}))
		.sort(byName);
}

function pacmanList(lines: string[] | undefined): PacmanPackage[] | null {
	if (lines === undefined) return null;
	return lines
		.map((line) => line.trim().split(/\s+/))
		.filter((parts) => parts[0])
		.map(([name, version]) => ({
			name: name as string,
			version: version ?? "",
		}))
		.sort(byName);
}

interface MiseEntry {
	version?: string;
	requested_version?: string;
	installed?: boolean;
	active?: boolean;
	source?: { path?: string };
}

function miseTools(lines: string[] | undefined): MiseTool[] | null {
	if (lines === undefined) return null;
	let data: Record<string, MiseEntry[]>;
	try {
		data = JSON.parse(lines.join("\n") || "{}");
	} catch {
		throw new Error("mise ls --json 출력을 해석할 수 없습니다");
	}
	return Object.entries(data)
		.map(([name, entries]) => {
			const active = entries.find((e) => e.active);
			return {
				name,
				activeVersion: active?.version ?? null,
				requestedVersion: active?.requested_version ?? null,
				installedVersions: entries
					.filter((e) => e.installed && e.version)
					.map((e) => e.version as string),
				source: active?.source?.path ?? null,
			};
		})
		.sort(byName);
}

const FILE_KINDS: Record<FileKind, true> = {
	file: true,
	symlink: true,
	dir: true,
	broken: true,
	missing: true,
};

function trackedFiles(lines: string[] | undefined): TrackedFile[] | null {
	if (lines === undefined) return null;
	const out: TrackedFile[] = [];
	for (const line of lines) {
		const [kind, sha, size, target, ...rest] = line.split("\t");
		const path = rest.join("\t");
		if (!kind || !(kind in FILE_KINDS) || path === "") continue;
		out.push({
			path,
			kind: kind as FileKind,
			sha256: sha || null,
			size: size ? Number(size) : null,
			linkTarget: target || null,
		});
	}
	return out.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
}

export function toInventory(sections: Map<string, string[]>): Inventory {
	const leaves = new Set(sections.get("brew_leaves") ?? []);
	const pacmanTotal = sections.get("pacman_count")?.[0];
	return {
		brewFormulae: brewList(sections.get("brew_formulae"), (n) => leaves.has(n)),
		brewCasks: brewList(sections.get("brew_casks"), () => true),
		pacmanExplicit: pacmanList(sections.get("pacman_explicit")),
		pacmanTotal: pacmanTotal === undefined ? null : num(pacmanTotal),
		mise: miseTools(sections.get("mise_json")),
		files: trackedFiles(sections.get("files")),
	};
}
