import {
	type ChildProcessWithoutNullStreams,
	execFileSync,
	spawn,
} from "node:child_process";
import {
	mkdirSync,
	mkdtempSync,
	rmSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { expect } from "@playwright/test";

export const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
export const repoRoot = resolve(webRoot, "..");
export const evidenceDir = join(repoRoot, "evidence");
export const qaDir = join(repoRoot, "docs", "qa");

export type ServerFixture = {
	readonly baseURL: string;
	readonly dbPath: string;
	readonly tempDir: string;
	readonly setupToken: string;
	readonly process: ChildProcessWithoutNullStreams;
};

export async function startServer(label: string): Promise<ServerFixture> {
	mkdirSync(evidenceDir, { recursive: true });
	mkdirSync(qaDir, { recursive: true });
	execFileSync("pnpm", ["build"], { cwd: webRoot, stdio: "pipe" });

	const tempDir = mkdtempSync(join(tmpdir(), `neul-${label}-`));
	const homeDir = join(tempDir, "home");
	mkdirSync(homeDir, { recursive: true });
	const dbPath = join(tempDir, "neul.sqlite");
	const serverPath = join(tempDir, "neul-server");
	execFileSync("go", ["build", "-o", serverPath, "./cmd/neul-server"], {
		cwd: repoRoot,
		env: processEnvWithoutGoroot(),
		stdio: "pipe",
	});
	const port = label === "empty" ? 18081 : 18082;
	const process = spawn(serverPath, [], {
		cwd: repoRoot,
		env: {
			...processEnvWithoutGoroot(),
			NEUL_ADDR: `127.0.0.1:${port}`,
			NEUL_DB: dbPath,
			NEUL_HOME_DIR: homeDir,
			NEUL_STATIC_DIR: join(webRoot, "dist"),
		},
	});
	const output: string[] = [];
	process.stdout.on("data", (chunk) => output.push(chunk.toString()));
	process.stderr.on("data", (chunk) => output.push(chunk.toString()));

	try {
		const setupToken = await waitForSetupToken(output);
		const baseURL = `http://127.0.0.1:${port}`;
		await waitForHealth(baseURL);
		return { baseURL, dbPath, tempDir, setupToken, process };
	} catch (error) {
		process.kill("SIGINT");
		rmSync(tempDir, { force: true, recursive: true });
		throw error;
	}
}

export async function stopServer(
	fixture: ServerFixture,
	label: string,
): Promise<void> {
	fixture.process.kill("SIGINT");
	await new Promise<void>((resolveStop) => {
		const timeout = setTimeout(resolveStop, 3000);
		fixture.process.once("exit", () => {
			clearTimeout(timeout);
			resolveStop();
		});
	});
	rmSync(fixture.tempDir, { force: true, recursive: true });
	writeFileSync(
		join(evidenceDir, `${label}-cleanup.txt`),
		[
			`db_removed=${!exists(fixture.dbPath)}`,
			`temp_removed=${!exists(fixture.tempDir)}`,
		].join("\n"),
	);
}

stopServer.cleanDist = (): void => {
	rmSync(join(webRoot, "dist"), { force: true, recursive: true });
};

export async function ownerSessionCookie(baseURL: string, setupToken: string) {
	const response = await fetch(`${baseURL}/api/session/local`, {
		body: JSON.stringify({ setupToken }),
		headers: { "Content-Type": "application/json" },
		method: "POST",
	});
	expect(response.status).toBe(204);
	const cookie = response.headers.get("set-cookie");
	if (cookie === null) {
		throw new Error("session cookie was not set");
	}
	const value = /neul_session=([^;]+)/.exec(cookie)?.[1];
	if (value === undefined) {
		throw new Error("neul_session cookie was not found");
	}
	return { domain: "127.0.0.1", name: "neul_session", path: "/", value };
}

function processEnvWithoutGoroot(): NodeJS.ProcessEnv {
	const next = { ...process.env };
	delete next.GOROOT;
	return next;
}

async function waitForSetupToken(output: readonly string[]): Promise<string> {
	for (let attempt = 0; attempt < 100; attempt += 1) {
		const token = /neul setup token: (setup_[^\s]+)/.exec(output.join(""))?.[1];
		if (token !== undefined) {
			return token;
		}
		await delay(100);
	}
	throw new Error(`setup token was not printed: ${output.join("")}`);
}

async function waitForHealth(baseURL: string): Promise<void> {
	for (let attempt = 0; attempt < 100; attempt += 1) {
		try {
			const response = await fetch(`${baseURL}/api/healthz`);
			if (response.ok) {
				return;
			}
		} catch {}
		await delay(100);
	}
	throw new Error(`server did not become healthy: ${baseURL}`);
}

function exists(path: string): boolean {
	try {
		statSync(path);
		return true;
	} catch {
		return false;
	}
}

function delay(ms: number): Promise<void> {
	return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}
