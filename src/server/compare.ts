import type {
	CompareRow,
	CompareSection,
	CompareView,
	Inventory,
	MachineStatus,
	TrackedFile,
} from "../shared/types.ts";

export interface CompareInput {
	id: number;
	name: string;
	status: MachineStatus;
	inventory: Inventory | null;
}

function fileCell(f: TrackedFile): string {
	if ((f.kind === "file" || f.kind === "symlink") && f.sha256)
		return f.sha256.slice(0, 12);
	return f.kind;
}

/**
 * Build one matrix section. `pick` returns name → value for a machine, or null when the
 * machine does not report this category (it then gets no column in the section).
 */
function section(
	id: CompareSection["id"],
	title: string,
	machines: CompareInput[],
	pick: (inv: Inventory) => Map<string, string> | null,
): CompareSection | null {
	const columns: { id: number; values: Map<string, string> }[] = [];
	for (const m of machines) {
		const values = m.inventory ? pick(m.inventory) : null;
		if (values) columns.push({ id: m.id, values });
	}
	if (columns.length === 0) return null;
	const keys = new Set<string>();
	for (const c of columns) for (const k of c.values.keys()) keys.add(k);
	const rows: CompareRow[] = [...keys].sort().map((key) => {
		const cells: Record<number, string | null> = {};
		for (const c of columns) cells[c.id] = c.values.get(key) ?? null;
		const values = Object.values(cells);
		return { key, cells, uniform: values.every((v) => v === values[0]) };
	});
	return { id, title, machineIds: columns.map((c) => c.id), rows };
}

export function buildCompare(machines: CompareInput[]): CompareView {
	const sections = [
		section("brewFormulae", "Homebrew formulae", machines, (inv) =>
			inv.brewFormulae
				? new Map(inv.brewFormulae.map((p) => [p.name, p.versions.join(" ")]))
				: null,
		),
		section("brewCasks", "Homebrew casks", machines, (inv) =>
			inv.brewCasks
				? new Map(inv.brewCasks.map((p) => [p.name, p.versions.join(" ")]))
				: null,
		),
		section("mise", "mise (활성 버전)", machines, (inv) =>
			inv.mise
				? new Map(
						inv.mise.flatMap((t) =>
							t.activeVersion ? [[t.name, t.activeVersion] as const] : [],
						),
					)
				: null,
		),
		section("pacman", "pacman (직접 설치)", machines, (inv) =>
			inv.pacmanExplicit
				? new Map(inv.pacmanExplicit.map((p) => [p.name, p.version]))
				: null,
		),
		section("files", "추적 파일 (sha256 앞 12자)", machines, (inv) =>
			inv.files ? new Map(inv.files.map((f) => [f.path, fileCell(f)])) : null,
		),
	].filter((s): s is CompareSection => s !== null);
	return {
		machines: machines.map(({ id, name, status }) => ({ id, name, status })),
		sections,
	};
}
