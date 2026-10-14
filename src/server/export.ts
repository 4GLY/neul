import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import type { ExportResult } from "../shared/types.ts";
import type { Store } from "./db.ts";

async function git(dir: string, args: string[]): Promise<number> {
	const proc = Bun.spawn(["git", ...args], {
		cwd: dir,
		stdout: "ignore",
		stderr: "pipe",
	});
	const [stderr, code] = await Promise.all([
		new Response(proc.stderr).text(),
		proc.exited,
	]);
	// `diff --quiet` signals "has changes" with 1; every other non-zero code is a failure.
	if (code !== 0 && !(args[0] === "diff" && code === 1))
		throw new Error(`git ${args[0]} failed: ${stderr.trim() || code}`);
	return code;
}

async function gitOutput(dir: string, args: string[]): Promise<string> {
	const proc = Bun.spawn(["git", ...args], { cwd: dir });
	const [stdout, code] = await Promise.all([
		new Response(proc.stdout).text(),
		proc.exited,
	]);
	if (code !== 0) throw new Error(`git ${args[0]} failed: ${code}`);
	return stdout.trim();
}

const json = (value: unknown) => `${JSON.stringify(value, null, 2)}\n`;

/**
 * Write the whole desired state into a local git repository and commit it when it changed.
 * Secret contents are exported as the stored age ciphertext (`<path>.age`); the hub identity is
 * never exported. Nothing is pushed.
 */
export async function exportDesired(
	store: Store,
	dir: string,
): Promise<ExportResult> {
	mkdirSync(dir, { recursive: true });
	if (!existsSync(join(dir, ".git"))) await git(dir, ["init", "-q"]);
	const names = new Map(store.listMachines().map((m) => [m.id, m.name]));
	const machines = (ids: number[]) =>
		ids.map((id) => names.get(id) ?? String(id)).sort();

	const packages = store
		.listDesired()
		.map((item) => ({
			manager: item.manager,
			name: item.name,
			version: item.version,
			ensure: item.ensure,
			machines: machines(item.machineIds),
		}))
		.sort((a, b) =>
			a.manager === b.manager
				? a.name.localeCompare(b.name)
				: a.manager.localeCompare(b.manager),
		);
	writeFileSync(join(dir, "packages.json"), json(packages));

	const files = store.listDesiredFiles();
	writeFileSync(
		join(dir, "files.json"),
		json(
			files.map((f) => ({
				path: f.path,
				ensure: f.ensure,
				mode: f.mode,
				secret: f.secret,
				machines: machines(f.machineIds),
				sha256: f.current?.sha256 ?? null,
				versionId: f.current?.versionId ?? null,
			})),
		),
	);

	const root = join(dir, "files");
	rmSync(root, { recursive: true, force: true });
	let written = 0;
	for (const file of files) {
		if (file.ensure !== "present" || !file.current) continue;
		const version = store.getDesiredFileVersionContent(
			file.id,
			file.current.versionId,
		);
		const body = file.secret ? version?.contentAge : version?.content;
		if (body == null) continue;
		const target = join(root, file.secret ? `${file.path}.age` : file.path);
		mkdirSync(dirname(target), { recursive: true });
		writeFileSync(target, body);
		written++;
	}

	await git(dir, ["add", "-A"]);
	if ((await git(dir, ["diff", "--cached", "--quiet"])) === 0)
		return { dir, commit: null, files: written };
	await git(dir, [
		"-c",
		"user.name=neul",
		"-c",
		"user.email=neul@localhost",
		"commit",
		"-q",
		"-m",
		`neul export ${new Date().toISOString()}`,
	]);
	return {
		dir,
		commit: await gitOutput(dir, ["rev-parse", "--short", "HEAD"]),
		files: written,
	};
}
